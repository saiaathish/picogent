package agent

import (
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/taskstate"
)

const durableContinuePrompt = `Internal task-loop instruction: the original request already authorizes the work. Do not ask whether to continue. Take the next obvious safe action with tools. Stop only for permission, a genuine user choice, repeated verification failure, an unavailable resource, or exhausted budget.`

const durableRepairMarker = "Internal verification-repair instruction:"

func durableRepairPrompt(evidence string) string {
	evidence = strings.TrimSpace(evidence)
	if len(evidence) > 4000 {
		evidence = evidence[:4000] + "…"
	}
	return durableRepairMarker + ` verification failed. Inspect this evidence, repair the smallest responsible area, and run verify again. Do not ask whether to continue.` + "\n\n" + evidence
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

func (a *Agent) continueAfterVerificationFailure(text string, round int, verified string) bool {
	if round+1 >= a.CFG.MaxToolRounds || strings.Contains(strings.ToLower(text), "blocked:") {
		return false
	}
	if verified != "" {
		a.noteTaskVerification(verified)
	}
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.task == nil || len(a.task.Verification) == 0 {
		return false
	}
	status := verificationStatus(a.task.Verification[len(a.task.Verification)-1].Summary)
	if status == "INCONCLUSIVE" || status == "SKIPPED" {
		a.task.Block("verification " + strings.ToLower(status))
		_ = a.TaskStore.Save(a.task)
		return false
	}
	if status != "FAIL" {
		return false
	}
	if a.task.ConsecutiveVerificationFailures() >= taskstate.DefaultPolicy().MaxVerificationFailures {
		a.task.Block("verification repeatedly failed")
		_ = a.TaskStore.Save(a.task)
		return false
	}
	a.task.NoteAttempt()
	_ = a.task.SetStatus(taskstate.StatusWorking)
	_ = a.TaskStore.Save(a.task)
	return true
}

// SetTaskSession switches durable task state with the chat session. Task state
// stays outside chat history, so compaction cannot erase execution progress.
func (a *Agent) SetTaskSession(sessionID string) {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	a.TaskSession = strings.TrimSpace(sessionID)
	a.task = nil
	if a.TaskStore == nil || a.TaskSession == "" {
		return
	}
	if task, err := a.TaskStore.Load(a.TaskSession); err == nil {
		a.task = task
	}
}

// TaskSnapshot returns an isolated copy safe for UI and persistence callers.
func (a *Agent) TaskSnapshot() *taskstate.Task {
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()
	if a.task == nil {
		return nil
	}
	cp := *a.task
	cp.Steps = append([]taskstate.Step(nil), a.task.Steps...)
	cp.ChangedFiles = append([]string(nil), a.task.ChangedFiles...)
	cp.Verification = append([]taskstate.Verification(nil), a.task.Verification...)
	return &cp
}

func (a *Agent) beginDurableTask(prompt string) {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.TaskStore == nil || a.TaskSession == "" {
		return
	}
	if a.task == nil || a.task.Status == taskstate.StatusDone || a.task.Status == taskstate.StatusBlocked {
		task, ok, err := taskstate.NewFromPrompt(a.TaskSession, prompt)
		if err != nil || !ok {
			return
		}
		a.task = task
	}
	if a.task.Status == taskstate.StatusPlanning {
		_ = a.task.SetStatus(taskstate.StatusWorking)
	}
	a.task.NoteAttempt()
	_ = a.TaskStore.Save(a.task)
}

func (a *Agent) taskPromptSuffix() string {
	t := a.TaskSnapshot()
	if t == nil || t.Status == taskstate.StatusDone {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nDurable task (compact execution state; keep working without asking for routine approval):\nGoal: ")
	b.WriteString(t.Goal)
	b.WriteString("\nStatus: ")
	b.WriteString(string(t.Status))
	for i, step := range t.Steps {
		mark := "[ ]"
		if step.Done {
			mark = "[x]"
		} else if i == t.CurrentStep {
			mark = "[>]"
		}
		fmt.Fprintf(&b, "\n%s %s", mark, step.Description)
	}
	b.WriteString("\nContinue while the goal is unresolved and a safe permitted action remains.")
	return b.String()
}

func (a *Agent) noteTaskChanged(path string) {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.task == nil {
		return
	}
	a.task.AddChangedFiles(path)
	for current := a.task.Current(); current != nil && !strings.Contains(strings.ToLower(current.Description), "verif"); current = a.task.Current() {
		a.task.Advance()
	}
	_ = a.TaskStore.Save(a.task)
}

func (a *Agent) continueAfterDeferral(text string, round int) bool {
	low := strings.ToLower(strings.TrimSpace(text))
	if low == "" || round+1 >= a.CFG.MaxToolRounds {
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
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.task == nil {
		return false
	}
	decision := taskstate.ShouldContinue(a.task, taskstate.Signals{SafeNextAction: true})
	if !decision.Continue {
		return false
	}
	a.task.NoteAttempt()
	_ = a.TaskStore.Save(a.task)
	return true
}

func (a *Agent) noteTaskVerification(output string) {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.task == nil || strings.TrimSpace(output) == "" {
		return
	}
	passed := verificationStatus(output) == "PASS"
	a.task.AddVerification("verify", passed, output)
	_ = a.TaskStore.Save(a.task)
}

func verificationStatus(output string) string {
	upper := strings.ToUpper(strings.TrimSpace(output))
	for _, status := range []string{"INCONCLUSIVE", "SKIPPED", "FAIL", "PASS"} {
		if strings.HasPrefix(upper, "VERIFY "+status) {
			return status
		}
	}
	return "INCONCLUSIVE"
}

func (a *Agent) blockDurableTask(reason string) {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.task == nil {
		return
	}
	a.task.Block(reason)
	_ = a.TaskStore.Save(a.task)
}

func (a *Agent) finishDurableTask(changed []string, text, blocker string) {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.task == nil {
		return
	}
	a.task.AddChangedFiles(changed...)
	if blocker != "" {
		a.task.Block(blocker)
		_ = a.TaskStore.Save(a.task)
		return
	}
	if a.task.Status == taskstate.StatusBlocked {
		_ = a.TaskStore.Save(a.task)
		return
	}
	if a.task.ConsecutiveVerificationFailures() > 0 {
		if a.task.ConsecutiveVerificationFailures() >= taskstate.DefaultPolicy().MaxVerificationFailures {
			a.task.Block("verification repeatedly failed")
		} else {
			_ = a.task.SetStatus(taskstate.StatusWorking)
		}
		_ = a.TaskStore.Save(a.task)
		return
	}
	low := strings.ToLower(text)
	if strings.Contains(low, "blocked:") || strings.Contains(low, "permission needed") {
		a.task.Block("agent reported a blocker")
		_ = a.TaskStore.Save(a.task)
		return
	}
	for a.task.Advance() {
	}
	_ = a.task.SetStatus(taskstate.StatusDone)
	_ = a.TaskStore.Save(a.task)
}
