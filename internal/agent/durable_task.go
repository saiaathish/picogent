package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/evolve"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/verify"
)

const durableContinuePrompt = `Internal task-loop instruction: the original request already authorizes the work. Do not ask whether to continue. Take the next obvious safe action with tools. Stop only for permission, a genuine user choice, repeated verification failure, an unavailable resource, or exhausted budget.`

const durableRepairMarker = "Internal verification-repair instruction:"

// errTaskMutationSkipped is an internal signal for callers that decide a
// task update is no longer applicable. It is deliberately not reported as a
// persistence failure to the user.
var errTaskMutationSkipped = errors.New("durable task mutation skipped")

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
	if last.Passed || previous.Passed {
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
	if a.task == nil || len(a.task.Verification) == 0 {
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
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	a.TaskSession = strings.TrimSpace(sessionID)
	a.task = nil
	a.taskLoadErr = nil
	if a.TaskStore == nil || a.TaskSession == "" {
		return nil
	}
	task, err := a.TaskStore.Load(a.TaskSession)
	if err == nil {
		invalidated, revalidateErr := revalidatePersistedTask(workspaceRoot, task)
		if revalidateErr != nil {
			a.taskLoadErr = revalidateErr
			return revalidateErr
		}
		if invalidated {
			if err := a.TaskStore.Save(task); err != nil {
				revalidateErr := fmt.Errorf("persist invalidated verification: %w", err)
				a.taskLoadErr = revalidateErr
				return revalidateErr
			}
		}
		a.task = task
		return nil
	}
	if errors.Is(err, taskstate.ErrNotFound) {
		return nil
	}
	a.taskLoadErr = err
	return err
}

// TaskSnapshot returns an isolated copy safe for UI and persistence callers.
func (a *Agent) TaskSnapshot() *taskstate.Task {
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()
	return cloneTask(a.task)
}

func (a *Agent) beginDurableTask(prompt string, ev EventHandler) bool {
	a.taskMu.Lock()
	if a.TaskStore == nil || a.TaskSession == "" {
		a.taskMu.Unlock()
		return false
	}
	if a.taskLoadErr != nil {
		err := a.taskLoadErr
		a.taskMu.Unlock()
		a.reportTaskPersistenceError(ev, fmt.Errorf("load durable task state: %w", err))
		return true
	}
	var candidate *taskstate.Task
	if a.task == nil || (a.task.Status == taskstate.StatusDone && !a.task.NeedsVerification()) {
		task, ok, err := taskstate.NewFromPrompt(a.TaskSession, prompt)
		if err != nil {
			a.taskMu.Unlock()
			a.reportTaskUpdateError(ev, err)
			return true
		}
		if !ok {
			a.taskMu.Unlock()
			return false
		}
		candidate = task
	} else {
		candidate = cloneTask(a.task)
	}
	candidate.InitializeChangeSequence()
	if candidate.Status == taskstate.StatusDone && candidate.NeedsVerification() {
		if err := candidate.SetStatus(taskstate.StatusVerifying); err != nil {
			a.taskMu.Unlock()
			a.reportTaskUpdateError(ev, err)
			return true
		}
	}
	if candidate.Status == taskstate.StatusBlocked {
		if err := candidate.SetStatus(taskstate.StatusWorking); err != nil {
			a.taskMu.Unlock()
			a.reportTaskUpdateError(ev, err)
			return true
		}
	}
	if candidate.Status == taskstate.StatusPlanning {
		if err := candidate.SetStatus(taskstate.StatusWorking); err != nil {
			a.taskMu.Unlock()
			a.reportTaskUpdateError(ev, err)
			return true
		}
	}
	candidate.NoteAttempt()
	snapshot, err := a.persistTaskCandidateLocked(candidate)
	if err != nil {
		a.taskMu.Unlock()
		a.reportTaskPersistenceError(ev, err)
		return true
	}
	a.taskMu.Unlock()
	emitTaskState(ev, snapshot)
	return false
}

func (a *Agent) taskPromptSuffix() string {
	context := renderDurableTaskContext(a.TaskSnapshot())
	return context
}

func (a *Agent) noteTaskChanged(path string, ev EventHandler) {
	a.mutateTask(ev, func(task *taskstate.Task) error {
		task.RecordChanged(path)
		for current := task.Current(); current != nil && !strings.Contains(strings.ToLower(current.Description), "verif"); current = task.Current() {
			task.Advance()
		}
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
	if task == nil || len(task.Verification) == 0 {
		return false, nil
	}
	latest := task.Verification[len(task.Verification)-1]
	if !latest.Passed {
		return false, nil
	}
	reason := "persisted verification has no complete PASS status"
	if verificationStatus(latest.Summary) == "PASS" {
		evidence := verificationEvidence{
			output:            latest.Summary,
			targets:           observationPaths(latest.Observation),
			observation:       cloneWorkspaceObservation(latest.Observation),
			observationUsable: latest.Observation != nil,
		}
		observation, fresh, checkReason := recheckVerificationEvidenceObservation(context.Background(), root, evidence)
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
	a.invalidateLatestTaskVerification(reason, ev)
	return inconclusive, false
}

func (a *Agent) invalidateLatestTaskVerification(reason string, ev EventHandler) bool {
	return a.mutateTask(ev, func(task *taskstate.Task) error {
		if !task.InvalidateLatestVerification(reason) {
			return errTaskMutationSkipped
		}
		if task.Status == taskstate.StatusDone {
			return task.SetStatus(taskstate.StatusVerifying)
		}
		return nil
	})
}

func (a *Agent) blockDurableTask(reason string, ev EventHandler) {
	a.mutateTask(ev, func(task *taskstate.Task) error {
		task.Block(reason)
		return nil
	})
}

func (a *Agent) finishDurableTask(text, blocker string, ev EventHandler) {
	a.mutateTask(ev, func(task *taskstate.Task) error {
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
	})
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
	if mutate == nil {
		return false
	}
	a.taskMu.Lock()
	if a.task == nil || a.TaskStore == nil {
		a.taskMu.Unlock()
		return false
	}
	candidate := cloneTask(a.task)
	if err := mutate(candidate); err != nil {
		a.taskMu.Unlock()
		if !errors.Is(err, errTaskMutationSkipped) {
			a.reportTaskUpdateError(ev, err)
		}
		return false
	}
	snapshot, err := a.persistTaskCandidateLocked(candidate)
	if err != nil {
		a.taskMu.Unlock()
		a.reportTaskPersistenceError(ev, err)
		return false
	}
	a.taskMu.Unlock()
	emitTaskState(ev, snapshot)
	return true
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
	cp.ChangedFiles = append([]string(nil), task.ChangedFiles...)
	cp.Verification = append([]taskstate.Verification(nil), task.Verification...)
	for i := range cp.Verification {
		cp.Verification[i].Observation = cloneWorkspaceObservation(task.Verification[i].Observation)
	}
	cp.Constraints = append([]string(nil), task.Constraints...)
	cp.Risks = append([]string(nil), task.Risks...)
	cp.Uncertainty = append([]string(nil), task.Uncertainty...)
	cp.Evidence = append([]taskstate.Evidence(nil), task.Evidence...)
	return &cp
}

func emitTaskState(ev EventHandler, snapshot *taskstate.Task) {
	if handler, ok := ev.(TaskStateHandler); ok {
		handler.OnTaskState(cloneTask(snapshot))
	}
}
