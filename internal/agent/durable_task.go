package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/evolve"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/verify"
)

const durableContinuePrompt = `Internal task-loop instruction: the original request already authorizes the work. Do not ask whether to continue. Take the next obvious safe action with tools. Stop only for permission, a genuine user choice, repeated verification failure, an unavailable resource, or exhausted budget.`

const durableRepairMarker = "Internal verification-repair instruction:"

// errTaskMutationSkipped is an internal signal for callers that decide a
// task update is no longer applicable. It is deliberately not reported as a
// persistence failure to the user.
var errTaskMutationSkipped = errors.New("durable task mutation skipped")

var errTaskOwnershipChanged = errors.New("durable task ownership changed")

// Progress/evidence callbacks own a task/intent/turn tuple, not whichever
// contract happens to occupy the same session file after a CAS conflict.
type taskMutationOwner struct {
	taskID, sessionID string
	intent, turn      uint64
	lastSequence      uint64
	lastIntent        uint64
	lastState         taskstate.TurnState
}

func taskOwner(task *taskstate.Task) taskMutationOwner {
	owner := taskMutationOwner{taskID: task.ID, sessionID: task.SessionID, intent: task.IntentRevision, turn: task.TurnRevision}
	if last := task.LastTurn(); last != nil {
		owner.lastSequence = last.Sequence
		owner.lastIntent = last.IntentRevision
		owner.lastState = last.State
	}
	return owner
}

// maxTaskMutationAttempts bounds recovery from a compare-and-swap conflict.
// A task mutation is replayed only against a freshly loaded durable snapshot;
// it never overwrites a newer cross-process update or retries an arbitrary
// persistence failure indefinitely.
const maxTaskMutationAttempts = 3

type taskPersistenceError struct {
	err error
}

func (e *taskPersistenceError) Error() string {
	if e == nil || e.err == nil {
		return "durable task state was not saved"
	}
	return e.err.Error()
}

func (e *taskPersistenceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func durableRepairPrompt(evidence string, repeated bool) string {
	evidence = strings.TrimSpace(evidence)
	if len(evidence) > 4000 {
		evidence = evidence[:4000] + "…"
	}
	prompt := durableRepairMarker + ` verification failed. Inspect this evidence, repair the smallest responsible area, and run verify again. Do not ask whether to continue.`
	if repeated {
		prompt += ` The same verification failure repeated: do not repeat the previous edit or command. Reread the current target, state a different hypothesis, and choose a materially different safe repair; if no distinct route exists, report blocked.`
	}
	if hint := durableRecoveryHint(evidence); hint != "" {
		prompt += "\nRecovery hint: " + hint
	}
	return prompt + "\n\n" + evidence
}

// repeatedVerificationFailure is a small escape hatch for autonomous repair
// loops. Once the exact normalized failure repeats, the next repair prompt
// must demand a different hypothesis instead of silently retrying the same
// route. It is advisory only; the verifier and task failure budget remain
// authoritative.
func (a *Agent) repeatedVerificationFailure() bool {
	task := a.TaskSnapshot()
	if task == nil || len(task.Verification) < 2 {
		return false
	}
	last := task.Verification[len(task.Verification)-1]
	previous := task.Verification[len(task.Verification)-2]
	if last.Retired || previous.Retired || last.Passed || previous.Passed {
		return false
	}
	lastFingerprint := verificationFailureFingerprint(last.Summary)
	return lastFingerprint != "" && lastFingerprint == verificationFailureFingerprint(previous.Summary)
}

func verificationFailureFingerprint(evidence string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(evidence))), " ")
}

func durableRecoveryHint(evidence string) string {
	low := strings.ToLower(evidence)
	switch {
	case strings.Contains(low, "old_string found") && strings.Contains(low, "times"):
		return "the edit matched multiple regions; reread the file and choose a unique exact replacement."
	case strings.Contains(low, "old_string not found"), strings.Contains(low, "no such file"), strings.Contains(low, "file does not exist"):
		return "the file or context is stale; reread the relevant path before recomputing the edit."
	case strings.Contains(low, "truncated"), strings.Contains(low, "output limit"):
		return "the evidence is truncated; narrow the command or inspect the relevant lines before deciding."
	case strings.Contains(low, "command not found"), strings.Contains(low, "executable file not found"):
		return "the runner is unavailable; inspect the repo map and package-manager manifests for the supported command."
	default:
		return ""
	}
}

func stripDurableInternal(msgs []llm.Message) []llm.Message {
	out := msgs[:0]
	for _, msg := range msgs {
		if msg.Role == "system" && (msg.Content == durableContinuePrompt || strings.HasPrefix(msg.Content, durableRepairMarker)) {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func (a *Agent) continueAfterVerificationFailure(text string, round int, evidence verificationEvidence, ev EventHandler, maxToolRounds int) bool {
	if evidence.output != "" {
		a.noteTaskVerification(evidence, ev)
	}
	if round+1 >= maxToolRounds || strings.Contains(strings.ToLower(text), "blocked:") {
		return false
	}
	a.taskMu.RLock()
	if a.task == nil || len(a.task.Verification) == 0 || a.task.Verification[len(a.task.Verification)-1].Retired {
		a.taskMu.RUnlock()
		return false
	}
	status := verificationStatus(a.task.Verification[len(a.task.Verification)-1].Summary)
	a.taskMu.RUnlock()
	if status == "INCONCLUSIVE" || status == "SKIPPED" {
		a.mutateTask(ev, func(task *taskstate.Task) error {
			task.Block("verification " + strings.ToLower(status))
			return nil
		})
		return false
	}
	if status != "FAIL" {
		return false
	}
	a.taskMu.RLock()
	tooMany := a.task != nil && a.task.ConsecutiveVerificationFailures() >= taskstate.DefaultPolicy().MaxVerificationFailures
	a.taskMu.RUnlock()
	if tooMany {
		a.mutateTask(ev, func(task *taskstate.Task) error {
			task.Block("verification repeatedly failed")
			return nil
		})
		return false
	}
	return a.mutateTask(ev, func(task *taskstate.Task) error {
		task.NoteAttempt()
		return task.SetStatus(taskstate.StatusWorking)
	})
}

// SetTaskSession switches durable task state with the chat session. Task state
// stays outside chat history, so compaction cannot erase execution progress.
func (a *Agent) SetTaskSession(sessionID string) error {
	workspaceRoot := a.ConfigSnapshot().Workspace
	binding := a.nativeTaskBinding()
	releaseRun, err := a.acquireProjectRunLockForWorkspace(workspaceRoot, binding)
	if err != nil {
		return fmt.Errorf("project run is unavailable: %w", err)
	}
	defer releaseRun()
	a.undoMu.Lock()
	defer a.undoMu.Unlock()
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.TaskStore != binding.store || a.taskStoreGeneration != binding.storeGeneration {
		return errTaskOwnershipChanged
	}
	requireReattachment := a.undoReattachRequired
	targetSessionID := strings.TrimSpace(sessionID)
	if requireReattachment && targetSessionID != "" && strings.TrimSpace(workspaceRoot) == "" {
		// A store rebind revokes cached undo authority. Without the workspace,
		// the durable journal cannot be validated, so refuse before changing
		// session state or recovering (and possibly rewriting) a task record.
		return errors.New("cannot reattach task-store authority without a configured workspace")
	}
	a.TaskSession = targetSessionID
	a.taskSessionGeneration++
	a.latestUndo = nil
	a.undoLoadErr = nil
	a.task = nil
	a.taskLoadErr = nil
	if a.TaskSession == "" {
		a.undoReattachRequired = false
		return nil
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		a.undoLoadErr = errors.New("undo recovery is unavailable without a configured workspace")
		if a.TaskStore != nil {
			task, err := a.TaskStore.OwnershipSnapshot(a.TaskSession)
			if errors.Is(err, taskstate.ErrNotFound) {
				task = nil
			} else if err != nil {
				a.taskLoadErr = err
				return fmt.Errorf("inspect durable task for session attachment: %w", err)
			}
			if task != nil {
				if err := prepareTaskForAttachment(workspaceRoot, a.TaskStore, task); err != nil {
					a.taskLoadErr = err
					return err
				}
				a.task = task
			}
		}
		if requireReattachment {
			a.undoReattachRequired = false
		}
		return nil
	}
	finishAttachment := func() error {
		if a.undoLoadErr != nil && !errors.Is(a.undoLoadErr, errLegacyUndoJournal) {
			if requireReattachment {
				return fmt.Errorf("validate undo recovery during session reattachment: %w", a.undoLoadErr)
			}
			return fmt.Errorf("validate undo recovery: %w", a.undoLoadErr)
		}
		if requireReattachment {
			a.undoReattachRequired = false
		}
		return nil
	}
	loadUndo := func(task *taskstate.Task) {
		undo, loadErr := loadValidatedDurableUndo(workspaceRoot, a.TaskSession, a.taskSessionGeneration, task, undoTaskStoreAuthority{store: a.TaskStore, epoch: a.taskStoreGeneration})
		if loadErr != nil {
			a.undoLoadErr = loadErr
			return
		}
		a.latestUndo = undo
	}
	if a.TaskStore == nil {
		loadUndo(nil)
		return finishAttachment()
	}
	task, err := a.TaskStore.OwnershipSnapshot(a.TaskSession)
	if errors.Is(err, taskstate.ErrNotFound) {
		loadUndo(nil)
		if a.undoLoadErr != nil && !errors.Is(a.undoLoadErr, errLegacyUndoJournal) {
			a.taskLoadErr = a.undoLoadErr
		}
		return finishAttachment()
	}
	if err != nil {
		a.taskLoadErr = err
		return fmt.Errorf("inspect durable task for session attachment: %w", err)
	}
	// Validate durable undo against the read-only owner snapshot before Load,
	// legacy normalization, proof revalidation, or interrupted-turn recovery
	// can write a replacement task record.
	loadUndo(task)
	if a.undoLoadErr != nil && !errors.Is(a.undoLoadErr, errLegacyUndoJournal) {
		a.taskLoadErr = a.undoLoadErr
		return finishAttachment()
	}
	if err := prepareTaskForAttachment(workspaceRoot, a.TaskStore, task); err != nil {
		a.taskLoadErr = err
		return err
	}
	a.task = task
	return finishAttachment()
}

// prepareTaskForAttachment restores persisted task trust before the snapshot
// becomes agent state. An empty workspace fails proof revalidation closed.
func prepareTaskForAttachment(workspaceRoot string, store *taskstate.Store, task *taskstate.Task) error {
	if task == nil {
		return nil
	}
	changed := task.NormalizeLegacyCompletion()
	revalidated, err := revalidatePersistedTask(workspaceRoot, task)
	if err != nil {
		return err
	}
	changed = changed || revalidated
	// A task with an active turn was left behind by a process that did not
	// reach its close point. Record the stale attempt before publishing it.
	if task.RecoverActiveTurn() {
		changed = true
	}
	if !changed {
		return nil
	}
	if store == nil {
		return errors.New("persist recovered durable task: task store is unavailable")
	}
	if err := store.Save(task); err != nil {
		return fmt.Errorf("persist recovered durable task: %w", err)
	}
	return nil
}

func (a *Agent) taskSessionSnapshot() (string, uint64) {
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()
	return a.TaskSession, a.taskSessionGeneration
}

// TaskSnapshot returns an isolated copy safe for UI and persistence callers.
func (a *Agent) TaskSnapshot() *taskstate.Task {
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()
	return cloneTask(a.task)
}

func (a *Agent) beginDurableTask(prompt string, ev EventHandler) (bool, error) {
	return a.beginDurableTaskWithFallback(prompt, ev, false)
}

// beginDurableTaskWithFallback normally keeps task inference conservative, but
// a real blocked or successful side effect must still have a durable owner
// even when the original user wording looked informational. The fallback is
// only used after execution reaches that boundary.
func (a *Agent) beginDurableTaskWithFallback(prompt string, ev EventHandler, fallback bool) (bool, error) {
	return a.beginDurableTaskInState(prompt, ev, fallback, a.RuntimeSnapshot())
}

// Admission uses the turn's original runtime identity. A later UI update must
// not silently become the workspace goal authorized for retirement.
func (a *Agent) beginDurableTaskInState(prompt string, ev EventHandler, fallback bool, runtime RuntimeState) (bool, error) {
	replacement, replacing := taskstate.ExplicitReplacement(prompt)
	admissionPrompt := prompt
	if replacing {
		admissionPrompt = replacement
	}
	a.taskMu.Lock()
	if a.taskRunBinding != nil {
		binding := *a.taskRunBinding
		if a.TaskStore != binding.store || a.TaskSession != binding.sessionID || a.taskSessionGeneration != binding.generation || a.taskStoreGeneration != binding.storeGeneration || !binding.owns(a.task) {
			a.taskMu.Unlock()
			return true, errTaskOwnershipChanged
		}
	}
	if a.TaskStore == nil || a.TaskSession == "" {
		a.taskMu.Unlock()
		if replacing {
			err := errors.New("outcome replacement requires durable task storage")
			a.reportTaskUpdateError(ev, err)
			return true, err
		}
		return false, nil
	}
	if a.taskLoadErr != nil {
		err := a.taskLoadErr
		a.taskMu.Unlock()
		err = fmt.Errorf("load durable task state: %w", err)
		a.reportTaskPersistenceError(ev, err)
		return true, err
	}
	var candidate *taskstate.Task
	newTask := a.task == nil || (!replacing && a.task.Status == taskstate.StatusDone && !a.task.NeedsVerification())
	if newTask {
		task, ok, err := taskstate.NewFromPrompt(a.TaskSession, admissionPrompt)
		if err != nil {
			a.taskMu.Unlock()
			a.reportTaskUpdateError(ev, err)
			return true, err
		}
		if !ok {
			if replacing {
				a.taskMu.Unlock()
				err := errors.New("replacement outcome must be a nonempty task-like request")
				a.reportTaskUpdateError(ev, err)
				return true, err
			}
			if !fallback {
				a.taskMu.Unlock()
				return false, nil
			}
			goal := strings.TrimSpace(admissionPrompt)
			if goal == "" {
				goal = "complete the requested action"
			}
			task, err = taskstate.New(a.TaskSession, goal, []string{"resolve the permission requirement"})
			if err != nil {
				a.taskMu.Unlock()
				a.reportTaskUpdateError(ev, err)
				return true, err
			}
		}
		candidate = task
		if a.task != nil {
			if a.task.IntentRevision == ^uint64(0) || a.task.TurnRevision == ^uint64(0) {
				a.taskMu.Unlock()
				err := errors.New("successor task ownership revision exhausted")
				a.reportTaskUpdateError(ev, err)
				return true, err
			}
			// A successor replaces the session's previous task, not an absent
			// revision-zero record. Keep its new identity and empty proof while
			// comparing against the completed owner's persisted generation.
			candidate.Revision = a.task.Revision
			// Older completion/undo consumers also bind to turn sequences.
			// Do not reuse those counters when the task identity changes.
			candidate.IntentRevision = a.task.IntentRevision + 1
			candidate.TurnRevision = a.task.TurnRevision
		}
	} else {
		candidate = cloneTask(a.task)
	}
	owner := taskOwner(candidate)
	expectedID, expectedIntent, expectedTurn := candidate.ID, candidate.IntentRevision, candidate.TurnRevision
	prepare := func(candidate *taskstate.Task) error {
		// Keep the original durable outcome and definition of done stable while
		// recording a changed interpretation of a later user request. This makes
		// steering visible to routing and recovery without letting one short
		// follow-up silently erase the larger outcome.
		if replacing {
			if candidate.ID != expectedID || candidate.IntentRevision != expectedIntent || candidate.TurnRevision != expectedTurn {
				return errors.New("outcome changed before replacement could be saved")
			}
			if err := candidate.ReplaceOutcome(replacement); err != nil {
				return err
			}
			if runtime.Goal != "" {
				candidate.ReplacedWorkspaceGoal = &taskstate.GoalRetirement{Text: runtime.Goal, Revision: runtime.GoalRevision}
			}
		} else if inferred := taskstate.Infer(prompt); inferred.TaskLike && inferred.Intent != nil && !strings.EqualFold(strings.TrimSpace(inferred.Goal), strings.TrimSpace(candidate.Goal)) {
			intent := *inferred.Intent
			intent.Outcome = candidate.Goal
			candidate.SetIntent(&intent)
		}
		candidate.InitializeChangeSequence()
		if candidate.Status == taskstate.StatusDone && candidate.NeedsVerification() {
			if err := candidate.SetStatus(taskstate.StatusVerifying); err != nil {
				return err
			}
		}
		if candidate.Status == taskstate.StatusBlocked {
			if err := candidate.SetStatus(taskstate.StatusWorking); err != nil {
				return err
			}
		}
		if candidate.Status == taskstate.StatusPlanning {
			if err := candidate.SetStatus(taskstate.StatusWorking); err != nil {
				return err
			}
		}
		candidate.NoteAttempt()
		return nil
	}
	if err := prepare(candidate); err != nil {
		a.taskMu.Unlock()
		a.reportTaskUpdateError(ev, err)
		return true, err
	}
	var snapshot *taskstate.Task
	var err error
	if newTask {
		// Never rebase a fresh/successor task into whatever contract now
		// occupies the session. A conflict requires a fresh admission.
		snapshot, err = a.persistTaskCandidateLocked(candidate)
	} else {
		snapshot, err = a.persistTaskCandidateWithRetryLocked(candidate, prepare, func(current *taskstate.Task) bool {
			if replacing {
				return taskOwner(current) == owner
			}
			// Admission precedes this run's own turn. Cooperative processes can
			// finish earlier turns while this request waits for the run lock.
			// Merge that closed-turn progress, never a changed outcome or a
			// different active turn. In-flight callbacks use the stricter tuple.
			if current.ID != owner.taskID || current.SessionID != owner.sessionID || current.IntentRevision != owner.intent {
				return false
			}
			if current.TurnRevision != owner.turn {
				last := current.LastTurn()
				return last != nil && last.State != taskstate.TurnActive
			}
			return true
		})
	}
	if err != nil {
		a.taskMu.Unlock()
		a.reportTaskPersistenceError(ev, err)
		return true, err
	}
	a.taskMu.Unlock()
	emitTaskState(ev, snapshot)
	return false, nil
}

// ensureDurableTaskForBlock gives a denied side effect a durable blocked
// projection without making informational prompts create tasks up front.
func (a *Agent) ensureDurableTaskForBlock(prompt, reason string, ev EventHandler) {
	if a.TaskSnapshot() == nil {
		if failed, _ := a.beginDurableTaskWithFallback(prompt, ev, true); failed {
			return
		}
	}
	a.blockDurableTask(reason, ev)
}

func (a *Agent) taskPromptSuffix() string {
	context := renderDurableTaskContext(a.TaskSnapshot())
	return context
}

func (a *Agent) noteTaskChanged(path, prompt string, ev EventHandler) {
	// A prompt can be intentionally informational while the model still
	// reaches a real, approved file mutation. Preserve the same durable
	// changed-file projection used by task-like prompts once that boundary is
	// crossed, without creating tasks for read-only turns.
	if a.TaskSnapshot() == nil {
		if failed, _ := a.beginDurableTaskWithFallback(prompt, ev, true); failed {
			return
		}
	}
	a.mutateTask(ev, func(task *taskstate.Task) error {
		task.RecordChanged(path)
		for current := task.Current(); current != nil && !strings.Contains(strings.ToLower(current.Description), "verif"); current = task.Current() {
			task.Advance()
		}
		return nil
	})
}

// noteTaskPermission records the effective result of a permission prompt in
// the same durable evidence ledger as the mutation it governs. It is called
// after the corresponding tool result is applied, so an approval for a write
// is bound to the resulting ChangeSeq rather than becoming stale immediately
// when the file is changed. Automatic Fast-mode and persisted always-allow
// decisions never reach this helper because the gate reports Prompted=false.
func (a *Agent) noteTaskPermission(req perm.Request, decision perm.Decision, ev EventHandler) {
	status := "DENIED"
	if decision == perm.Allow {
		status = "APPROVED"
	}
	summary := strings.TrimSpace(req.Summary)
	if summary == "" {
		summary = "permission decision for " + strings.TrimSpace(req.Tool)
	}
	a.mutateTask(ev, func(task *taskstate.Task) error {
		task.RecordApprovalEvidence(status, summary, "permission prompt")
		return nil
	})
}

func (a *Agent) continueAfterDeferral(text string, round int, ev EventHandler, maxToolRounds int) bool {
	low := strings.ToLower(strings.TrimSpace(text))
	if low == "" || round+1 >= maxToolRounds {
		return false
	}
	deferred := strings.Contains(low, "would you like me to") ||
		strings.Contains(low, "do you want me to") ||
		strings.Contains(low, "want me to") ||
		strings.Contains(low, "shall i") ||
		strings.Contains(low, "if you'd like, i can") ||
		strings.Contains(low, "if you want, i can")
	if !deferred {
		return false
	}
	return a.mutateTask(ev, func(task *taskstate.Task) error {
		decision := taskstate.ShouldContinue(task, taskstate.Signals{SafeNextAction: true})
		if !decision.Continue {
			return errTaskMutationSkipped
		}
		task.NoteAttempt()
		return nil
	})
}

func (a *Agent) noteTaskVerification(evidence verificationEvidence, ev EventHandler) {
	if strings.TrimSpace(evidence.output) == "" {
		return
	}
	stored := evidence.output
	passed := verificationStatus(stored) == "PASS"
	if passed && !verificationObservationUsable(evidence) {
		stored = inconclusiveVerification(evidence.observationReason)
		passed = false
	}
	a.mutateTask(ev, func(task *taskstate.Task) error {
		criteria := task.RequiredCriterionIndices()
		if len(criteria) == 0 {
			task.AddVerificationWithObservation("verify", passed, stored, evidence.observation)
			return nil
		}
		// One successful, workspace-bound verifier run is the trusted producer
		// for the bounded definition-of-done criteria. Bind it explicitly so the
		// durable completion predicate cannot be satisfied by aggregate narration
		// or an unscoped passing status alone.
		task.AddVerificationForCriteria(criteria, "verify", passed, stored, evidence.observation)
		return nil
	})
	a.rememberVerification(stored)
}

// rememberVerification turns current evidence into a bounded causal memory
// record. Memory is advisory context only; the live verifier and permission
// gate remain authoritative.
func (a *Agent) rememberVerification(output string) {
	status := verificationStatus(output)
	if status != "PASS" && status != "FAIL" && status != "INCONCLUSIVE" && status != "SKIPPED" {
		return
	}
	state := a.RuntimeSnapshot()
	workspace := strings.TrimSpace(state.CFG.Workspace)
	if workspace == "" {
		return
	}
	if strings.TrimSpace(state.Memory.Workspace) == "" || state.Memory.Workspace != workspace {
		return
	}
	hint := state.Goal
	var targets []string
	if task := a.TaskSnapshot(); task != nil {
		if strings.TrimSpace(task.Goal) != "" {
			hint = task.Goal
		}
		targets = append(targets, task.ChangedFiles...)
	}
	memory, err := evolve.Update(workspace, func(memory evolve.Store) (evolve.Store, error) {
		if status == "PASS" {
			return evolve.RecordVerificationRoute(memory, hint, targets, output), nil
		}
		return evolve.RecordFailure(memory, hint, output), nil
	})
	if err == nil {
		a.SetMemory(memory)
	}
}

// requireTaskVerification records that an explicit completion marker still
// needs fresh evidence. The negative sequence is also meaningful for tasks
// with no file mutations, where the normal mutation-based verification gate
// would otherwise consider an unverified task complete.
func (a *Agent) requireTaskVerification(ev EventHandler) {
	a.mutateTask(ev, func(task *taskstate.Task) error {
		task.VerifiedChangeSeq = -1
		if task.Status == taskstate.StatusPlanning {
			if err := task.SetStatus(taskstate.StatusWorking); err != nil {
				return err
			}
		}
		if task.Status == taskstate.StatusVerifying {
			return nil
		}
		return task.SetStatus(taskstate.StatusVerifying)
	})
}

func verificationStatus(output string) string {
	return string(verify.StatusFromEvidence(output))
}

func revalidatePersistedTask(root string, task *taskstate.Task) (bool, error) {
	return revalidateTaskProof(context.Background(), root, task)
}

// revalidateTaskProof operates only on an isolated candidate. Finalization
// must commit invalidation and the terminal state together; a failed save
// cannot leave a later finish able to reuse stale in-memory trust.
func revalidateTaskProof(ctx context.Context, root string, task *taskstate.Task) (bool, error) {
	if task == nil || len(task.Verification) == 0 {
		return false, nil
	}
	latest := task.Verification[len(task.Verification)-1]
	if latest.Retired || !latest.Passed {
		return false, nil
	}
	reason := "persisted verification has no complete PASS status"
	if strings.TrimSpace(root) == "" {
		reason = "configured workspace is unavailable for persisted proof revalidation"
	} else if verificationStatus(latest.Summary) == "PASS" {
		evidence := verificationEvidence{
			output:            latest.Summary,
			targets:           observationPaths(latest.Observation),
			observation:       cloneWorkspaceObservation(latest.Observation),
			observationUsable: latest.Observation != nil,
		}
		observation, fresh, checkReason := recheckVerificationEvidenceObservation(ctx, root, evidence)
		if fresh {
			// Persisted verification records intentionally lose their runtime trust
			// bit when serialized. A fresh comparison against the live workspace is
			// the only boundary that may restore it during agent resume.
			return task.ReestablishWorkspaceVerification(observation), nil
		}
		if checkReason != "" {
			reason = "persisted workspace evidence is stale: " + checkReason
		}
	}
	if !task.InvalidateLatestVerification(reason) {
		return false, nil
	}
	if task.Status == taskstate.StatusDone {
		if err := task.SetStatus(taskstate.StatusVerifying); err != nil {
			return false, err
		}
	} else if task.Status == taskstate.StatusPlanning {
		if err := task.SetStatus(taskstate.StatusWorking); err != nil {
			return false, err
		}
		if err := task.SetStatus(taskstate.StatusVerifying); err != nil {
			return false, err
		}
	} else if task.Status != taskstate.StatusVerifying && task.Status != taskstate.StatusBlocked {
		if err := task.SetStatus(taskstate.StatusVerifying); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (a *Agent) revalidateVerificationBeforeCompletion(ctx context.Context, root string, evidence verificationEvidence, ev EventHandler) (string, bool) {
	fresh, reason := recheckVerificationEvidence(ctx, root, evidence)
	if fresh {
		return evidence.output, true
	}
	if reason == "" {
		reason = "workspace evidence is not fresh"
	}
	inconclusive := inconclusiveVerification("completion evidence is stale: " + reason)
	return inconclusive, false
}

func (a *Agent) blockDurableTask(reason string, ev EventHandler) {
	a.mutateTask(ev, func(task *taskstate.Task) error {
		task.Block(reason)
		return nil
	})
}

func applyDurableTaskFinish(task *taskstate.Task, text, blocker string) error {
	if blocker != "" {
		task.Block(blocker)
	} else if task.Status == taskstate.StatusBlocked {
		// Preserve the specific blocker recorded earlier in the turn.
	} else if task.ConsecutiveVerificationFailures() > 0 {
		status := verificationStatus(task.Verification[len(task.Verification)-1].Summary)
		if status == "INCONCLUSIVE" || status == "SKIPPED" {
			if err := task.SetStatus(taskstate.StatusVerifying); err != nil {
				return err
			}
		} else if task.ConsecutiveVerificationFailures() >= taskstate.DefaultPolicy().MaxVerificationFailures {
			task.Block("verification repeatedly failed")
		} else if err := task.SetStatus(taskstate.StatusWorking); err != nil {
			return err
		}
	} else {
		low := strings.ToLower(text)
		if strings.Contains(low, "blocked:") || strings.Contains(low, "permission needed") {
			task.Block("agent reported a blocker")
		} else if task.NeedsVerification() {
			if err := task.SetStatus(taskstate.StatusVerifying); err != nil {
				return err
			}
		} else {
			for task.Advance() {
			}
			if err := task.SetStatus(taskstate.StatusDone); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Agent) finishDurableTask(text, blocker string, ev EventHandler) error {
	snapshot, err := a.mutateTaskResult(func(task *taskstate.Task) error {
		return applyDurableTaskFinish(task, text, blocker)
	})
	if err != nil {
		if errors.Is(err, errTaskMutationSkipped) {
			return nil
		}
		var persistenceErr *taskPersistenceError
		if errors.As(err, &persistenceErr) {
			err = fmt.Errorf("durable task state was not saved: %w", err)
			if ev != nil {
				ev.OnError(err)
			}
			return err
		}
		// A logical completion refusal, such as missing current proof, is an
		// expected non-terminal state. Preserve the existing behavior of
		// reporting it to the event surface without turning the whole turn into
		// a persistence failure.
		a.reportTaskUpdateError(ev, err)
		return nil
	}
	if snapshot != nil {
		emitTaskState(ev, snapshot)
	}
	return nil
}

func (a *Agent) setTaskStatus(status taskstate.Status, ev EventHandler) {
	a.mutateTask(ev, func(task *taskstate.Task) error {
		if task.Status == status || task.Status == taskstate.StatusDone || task.Status == taskstate.StatusBlocked {
			return errTaskMutationSkipped
		}
		return task.SetStatus(status)
	})
}

// mutateTask applies a durable task update to an isolated candidate, persists
// it, and only then publishes the candidate as the new in-memory state. A
// failed Save therefore cannot produce a task snapshot that looks persisted;
// the last successfully persisted state remains available for resume.
func (a *Agent) mutateTask(ev EventHandler, mutate func(*taskstate.Task) error) bool {
	snapshot, err := a.mutateTaskResultBound(mutate, true)
	if err != nil {
		if errors.Is(err, errTaskMutationSkipped) {
			return false
		}
		var persistenceErr *taskPersistenceError
		if errors.As(err, &persistenceErr) {
			a.reportTaskPersistenceError(ev, err)
		} else {
			a.reportTaskUpdateError(ev, err)
		}
		return false
	}
	if snapshot == nil {
		return false
	}
	emitTaskState(ev, snapshot)
	return true
}

// mutateTaskResult applies a durable task update through the same isolated
// save-before-publish/CAS commit point as ordinary turn mutations. A conflict
// reloads the newest task and replays the pure candidate mutation a bounded
// number of times, preserving progress written by another process without
// allowing a stale candidate to overwrite it. A nil snapshot means that no
// durable task is attached to this agent; callers that need to surface
// persistence failures can inspect the returned error.
func (a *Agent) mutateTaskResult(mutate func(*taskstate.Task) error) (*taskstate.Task, error) {
	// Terminal completion and undo have their own explicit ownership/proof
	// predicates. Keep those consumers' fresh-snapshot refusal semantics.
	return a.mutateTaskResultBound(mutate, false)
}

func (a *Agent) mutateTaskResultBound(mutate func(*taskstate.Task) error, bindOwnership bool) (*taskstate.Task, error) {
	if mutate == nil {
		return nil, nil
	}
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.taskRunBinding != nil {
		if err := a.checkTaskBindingLocked(*a.taskRunBinding); err != nil {
			return nil, &taskPersistenceError{err: err}
		}
	}
	if a.task == nil || a.TaskStore == nil {
		return nil, nil
	}
	var owns func(*taskstate.Task) bool
	if a.taskRunBinding != nil {
		binding := *a.taskRunBinding
		owns = binding.owns
	} else if bindOwnership {
		captured := taskOwner(a.task)
		owns = func(current *taskstate.Task) bool { return taskOwner(current) == captured }
	}
	candidate := cloneTask(a.task)
	if err := mutate(candidate); err != nil {
		return nil, err
	}
	snapshot, err := a.persistTaskCandidateWithRetryLocked(candidate, mutate, owns)
	if err != nil {
		return nil, &taskPersistenceError{err: err}
	}
	return snapshot, nil
}

// persistTaskCandidateWithRetryLocked commits a prepared candidate without
// overwriting a newer cross-process task revision. On a CAS conflict it loads
// the current durable task and replays the candidate mutation before trying
// again, provided the captured intermediate owner still matches. The caller
// must hold taskMu; mutate must only change its argument.
func (a *Agent) persistTaskCandidateWithRetryLocked(candidate *taskstate.Task, mutate func(*taskstate.Task) error, owns func(*taskstate.Task) bool) (*taskstate.Task, error) {
	if candidate == nil || mutate == nil {
		return nil, errors.New("durable task mutation is not configured")
	}
	for attempt := 0; attempt < maxTaskMutationAttempts; attempt++ {
		snapshot, err := a.persistTaskCandidateLocked(candidate)
		if err == nil {
			return snapshot, nil
		}
		if !errors.Is(err, taskstate.ErrRevisionConflict) || attempt == maxTaskMutationAttempts-1 {
			return nil, err
		}
		current, loadErr := a.TaskStore.Load(candidate.SessionID)
		if loadErr != nil {
			return nil, errors.Join(err, fmt.Errorf("reload durable task after revision conflict: %w", loadErr))
		}
		if owns != nil && !owns(current) {
			// Do not adopt the replacement into memory. Later callbacks from
			// this old turn must continue to fail rather than acquire its owner.
			return nil, errors.Join(err, errTaskOwnershipChanged)
		}
		a.task = current
		candidate = cloneTask(current)
		if err := mutate(candidate); err != nil {
			return nil, err
		}
	}
	return nil, taskstate.ErrRevisionConflict
}

// rebaseTaskFromStore refreshes the in-memory task from the latest durable
// generation after a CAS conflict. The next mutation can then preserve fields
// written by another process, including attempts, history, and evidence.
func (a *Agent) rebaseTaskFromStore() error {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.TaskStore == nil {
		return nil
	}
	sessionID := strings.TrimSpace(a.TaskSession)
	if sessionID == "" && a.task != nil {
		sessionID = a.task.SessionID
	}
	if sessionID == "" {
		return nil
	}
	current, err := a.TaskStore.Load(sessionID)
	if err != nil {
		return err
	}
	a.task = current
	a.taskLoadErr = nil
	return nil
}

// persistTaskCandidateLocked is the single commit point for durable task
// mutations. The caller must hold taskMu; no in-memory state or event snapshot
// is changed until Save succeeds.
func (a *Agent) persistTaskCandidateLocked(candidate *taskstate.Task) (*taskstate.Task, error) {
	if candidate == nil || a.TaskStore == nil {
		return nil, errors.New("durable task store is not configured")
	}
	if err := a.TaskStore.Save(candidate); err != nil {
		return nil, err
	}
	a.task = candidate
	if a.taskRunBinding != nil {
		binding := a.taskRunBinding.withAdmittedTask(candidate)
		a.taskRunBinding = &binding
	}
	return cloneTask(candidate), nil
}

func (a *Agent) reportTaskPersistenceError(ev EventHandler, err error) {
	if ev == nil || err == nil {
		return
	}
	ev.OnError(fmt.Errorf("durable task state was not saved: %w", err))
}

func (a *Agent) reportTaskUpdateError(ev EventHandler, err error) {
	if ev == nil || err == nil {
		return
	}
	ev.OnError(fmt.Errorf("durable task state update failed: %w", err))
}

func cloneTask(task *taskstate.Task) *taskstate.Task {
	if task == nil {
		return nil
	}
	cp := *task
	cp.Steps = append([]taskstate.Step(nil), task.Steps...)
	cp.DefinitionOfDone = append([]taskstate.Criterion(nil), task.DefinitionOfDone...)
	if task.Intent != nil {
		intent := *task.Intent
		cp.Intent = &intent
	}
	if task.ReplacedWorkspaceGoal != nil {
		retired := *task.ReplacedWorkspaceGoal
		cp.ReplacedWorkspaceGoal = &retired
	}
	cp.ChangedFiles = append([]string(nil), task.ChangedFiles...)
	cp.Verification = append([]taskstate.Verification(nil), task.Verification...)
	for i := range cp.Verification {
		cp.Verification[i].Observation = cloneWorkspaceObservation(task.Verification[i].Observation)
	}
	cp.Constraints = append([]string(nil), task.Constraints...)
	cp.Risks = append([]string(nil), task.Risks...)
	cp.Uncertainty = append([]string(nil), task.Uncertainty...)
	cp.Evidence = append([]taskstate.Evidence(nil), task.Evidence...)
	cp.Turns = append([]taskstate.TurnRecord(nil), task.Turns...)
	for i := range cp.Turns {
		cp.Turns[i].ChangedFiles = append([]string(nil), task.Turns[i].ChangedFiles...)
		if task.Turns[i].FinishedAt != nil {
			finished := *task.Turns[i].FinishedAt
			cp.Turns[i].FinishedAt = &finished
		}
	}
	return &cp
}

func emitTaskState(ev EventHandler, snapshot *taskstate.Task) {
	if handler, ok := ev.(TaskStateHandler); ok {
		handler.OnTaskState(cloneTask(snapshot))
	}
}
