package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/ctxmgr"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/taskstate"
)

const (
	// longHorizonTurns intentionally exceeds every durable FIFO in the current
	// task/session model. The fixture is large enough to exercise retention,
	// restart, and context composition without making normal CI expensive.
	longHorizonTurns       = 96
	longHorizonMaxTurns    = 16
	longHorizonMaxEvidence = 16
)

type longHorizonFixture struct {
	workspace string
	session   *session.Session
	task      *taskstate.Task
	store     *taskstate.Store
	messages  []llm.Message
}

type longHorizonMetrics struct {
	maxSessionMessages int
	maxTaskTurns       int
	maxTaskEvidence    int
	maxContextChars    int
	maxContextTokens   int
	maxSessionBytes    int
	maxTaskBytes       int
	reloads            int
}

func TestLongHorizonResumeEnvelope(t *testing.T) {
	fixture := newLongHorizonFixture(t)
	metrics := advanceLongHorizon(t, fixture, longHorizonTurns)

	if len(fixture.messages) != longHorizonTurns*4 {
		t.Fatalf("raw logical history = %d messages, want %d", len(fixture.messages), longHorizonTurns*4)
	}
	if metrics.reloads != longHorizonTurns {
		t.Fatalf("store reloads = %d, want %d", metrics.reloads, longHorizonTurns)
	}
	if len(fixture.session.Messages) > session.MaxSessionMessages {
		t.Fatalf("retained session messages = %d, want <= %d", len(fixture.session.Messages), session.MaxSessionMessages)
	}
	if len(fixture.session.Messages) == 0 || fixture.session.Messages[len(fixture.session.Messages)-1].Content != fmt.Sprintf("recorded turn %d", longHorizonTurns-1) {
		t.Fatalf("retained session does not include latest turn: %#v", fixture.session.Messages[maxLongHorizon(0, len(fixture.session.Messages)-2):])
	}
	if len(fixture.task.Turns) > longHorizonMaxTurns {
		t.Fatalf("retained task turns = %d, want <= %d", len(fixture.task.Turns), longHorizonMaxTurns)
	}
	if len(fixture.task.Evidence) > longHorizonMaxEvidence {
		t.Fatalf("retained task evidence = %d, want <= %d", len(fixture.task.Evidence), longHorizonMaxEvidence)
	}
	if fixture.task.TurnRevision != longHorizonTurns {
		t.Fatalf("turn revision = %d, want %d", fixture.task.TurnRevision, longHorizonTurns)
	}
	if fixture.task.IntentRevision != 2 {
		t.Fatalf("steering revision = %d, want initial intent plus one revision", fixture.task.IntentRevision)
	}
	steeredTurns := 0
	for _, turn := range fixture.task.Turns {
		if turn.IntentRevision == fixture.task.IntentRevision {
			steeredTurns++
		}
	}
	if steeredTurns == 0 {
		t.Fatal("no retained turn records refer to the steered intent revision")
	}
	if last := fixture.task.LastTurn(); last == nil || last.State != taskstate.TurnCompleted {
		t.Fatalf("last durable turn = %#v, want completed", last)
	}
	if metrics.maxContextChars > maxDurableContextChars {
		t.Fatalf("durable context grew to %d chars, want <= %d", metrics.maxContextChars, maxDurableContextChars)
	}
	if metrics.maxContextTokens > ctxmgr.SoftTarget(ctxmgr.DefaultBudget)*3 {
		t.Fatalf("managed context grew to %d tokens, want <= %d", metrics.maxContextTokens, ctxmgr.SoftTarget(ctxmgr.DefaultBudget)*3)
	}
	if metrics.maxSessionBytes > session.MaxSessionBytes {
		t.Fatalf("session record grew to %d bytes, want <= %d", metrics.maxSessionBytes, session.MaxSessionBytes)
	}
	if err := fixture.task.Validate(); err != nil {
		t.Fatalf("reloaded task is invalid: %v", err)
	}

	sessionPath, err := fixture.session.Path()
	if err != nil {
		t.Fatal(err)
	}
	taskPath, err := fixture.store.Path(fixture.task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"session": sessionPath, "task": taskPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s record: %v", name, err)
		}
		if name == "session" && info.Size() > session.MaxSessionBytes {
			t.Fatalf("session file size = %d, want <= %d", info.Size(), session.MaxSessionBytes)
		}
	}
}

func BenchmarkLongHorizonResumeEnvelope(b *testing.B) {
	b.ReportAllocs()
	var metrics longHorizonMetrics
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		fixture := newLongHorizonFixture(b)
		b.StartTimer()
		metrics = advanceLongHorizon(b, fixture, longHorizonTurns)
		b.StopTimer()
	}
	b.ReportMetric(float64(metrics.maxSessionMessages), "retained-session-messages/op")
	b.ReportMetric(float64(metrics.maxTaskTurns), "retained-task-turns/op")
	b.ReportMetric(float64(metrics.maxTaskEvidence), "retained-task-evidence/op")
	b.ReportMetric(float64(metrics.maxContextChars), "durable-context-chars/op")
	b.ReportMetric(float64(metrics.maxContextTokens), "managed-context-tokens/op")
	b.ReportMetric(float64(metrics.maxSessionBytes), "session-bytes/op")
	b.ReportMetric(float64(metrics.maxTaskBytes), "task-bytes/op")
}

func maxLongHorizon(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func newLongHorizonFixture(tb testing.TB) *longHorizonFixture {
	tb.Helper()
	root := tb.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		tb.Fatalf("create workspace: %v", err)
	}
	tb.Setenv("PICOGENT_HOME", home)

	s := &session.Session{
		ID:        "long-horizon-session",
		Title:     "long horizon",
		Workspace: workspace,
	}
	if err := s.Save(); err != nil {
		tb.Fatalf("save initial session: %v", err)
	}
	task, err := taskstate.New(s.ID, "maintain a verified multi-turn outcome", []string{"inspect", "implement", "verify"})
	if err != nil {
		tb.Fatalf("create task: %v", err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		tb.Fatalf("start task: %v", err)
	}
	task.SetIntent(&taskstate.IntentContract{
		Outcome:    task.Goal,
		Class:      "implementation",
		Action:     "maintain",
		NeedsTests: true,
	})
	store := taskstate.WorkspaceStore(workspace)
	if err := store.Save(task); err != nil {
		tb.Fatalf("save initial task: %v", err)
	}
	return &longHorizonFixture{workspace: workspace, session: s, task: task, store: store}
}

func advanceLongHorizon(tb testing.TB, fixture *longHorizonFixture, turns int) longHorizonMetrics {
	tb.Helper()
	var metrics longHorizonMetrics
	for i := 0; i < turns; i++ {
		callID := fmt.Sprintf("read-%03d", i)
		fixture.messages = append(fixture.messages,
			llm.Message{Role: "user", Content: fmt.Sprintf("continue turn %d", i)},
			llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("inspect the bounded outcome for turn %d", i),
				ToolCalls: []llm.ToolCall{{
					ID:        callID,
					Name:      "read_file",
					Arguments: `{"path":"internal/feature.go"}`,
				}},
			},
			llm.Message{
				Role:       "tool",
				ToolCallID: callID,
				Name:       "read_file",
				Content:    strings.Repeat("bounded observation ", 24),
			},
			llm.Message{Role: "assistant", Content: fmt.Sprintf("recorded turn %d", i)},
		)

		if i == turns/2 {
			// Steering changes the interpretation, not the durable outcome. The
			// next reload must preserve both the revision and the original goal.
			fixture.task.SetIntent(&taskstate.IntentContract{
				Outcome:    fixture.task.Goal,
				Class:      "review",
				Action:     "reassess",
				NeedsTests: true,
			})
		}
		fixture.task.NoteAttempt()
		fixture.task.RecordChanged(fmt.Sprintf("internal/feature-%02d.go", i%4))
		sequence, ok := fixture.task.BeginTurn(taskstate.TurnRouteImplement)
		if !ok {
			tb.Fatalf("begin turn %d failed", i)
		}
		if !fixture.task.FinishTurn(sequence, taskstate.TurnRouteImplement, fmt.Sprintf("hypothesis %d", i), "UNVERIFIED", taskstate.StopNone, 2, 1) {
			tb.Fatalf("finish turn %d failed", i)
		}
		fixture.task.RecordTestsEvidence("PASS", fmt.Sprintf("turn %d test evidence", i), "long-horizon fixture")

		if err := session.SaveMessages(fixture.workspace, fixture.session.ID, fixture.messages); err != nil {
			tb.Fatalf("save session turn %d: %v", i, err)
		}
		if err := fixture.store.Save(fixture.task); err != nil {
			tb.Fatalf("save task turn %d: %v", i, err)
		}

		managed, stats, err := ctxmgr.Manage(context.Background(), nil, "gpt-5.6-terra", append([]llm.Message(nil), fixture.messages...), ctxmgr.DefaultBudget)
		if err != nil {
			tb.Fatalf("manage context turn %d: %v", i, err)
		}
		if got := len(renderDurableTaskContext(fixture.task)); got > metrics.maxContextChars {
			metrics.maxContextChars = got
		}
		if stats.Tokens > metrics.maxContextTokens {
			metrics.maxContextTokens = stats.Tokens
		}
		if len(managed) == 0 {
			tb.Fatalf("managed context turn %d was empty", i)
		}
		if len(fixture.session.Messages) > metrics.maxSessionMessages {
			metrics.maxSessionMessages = len(fixture.session.Messages)
		}
		if len(fixture.task.Turns) > metrics.maxTaskTurns {
			metrics.maxTaskTurns = len(fixture.task.Turns)
		}
		if len(fixture.task.Evidence) > metrics.maxTaskEvidence {
			metrics.maxTaskEvidence = len(fixture.task.Evidence)
		}
		if encoded, err := json.Marshal(fixture.session); err != nil {
			tb.Fatalf("encode session turn %d: %v", i, err)
		} else if len(encoded) > metrics.maxSessionBytes {
			metrics.maxSessionBytes = len(encoded)
		}
		if encoded, err := json.Marshal(fixture.task); err != nil {
			tb.Fatalf("encode task turn %d: %v", i, err)
		} else if len(encoded) > metrics.maxTaskBytes {
			metrics.maxTaskBytes = len(encoded)
		}

		loadedSession, err := session.Load(fixture.session.ID)
		if err != nil {
			tb.Fatalf("reload session turn %d: %v", i, err)
		}
		loadedTask, err := fixture.store.Load(fixture.task.SessionID)
		if err != nil {
			tb.Fatalf("reload task turn %d: %v", i, err)
		}
		fixture.session = loadedSession
		fixture.task = loadedTask
		metrics.reloads++
	}
	return metrics
}

type longHorizonHelperResult struct {
	TurnState       taskstate.TurnState `json:"turn_state"`
	SessionMessages int                 `json:"session_messages"`
	TaskRevision    uint64              `json:"task_revision"`
}

func TestLongHorizonResumeAfterProcessExit(t *testing.T) {
	if os.Getenv("PICOGENT_LONG_HORIZON_HELPER") == "1" {
		longHorizonResumeHelper(t)
		return
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)
	s := &session.Session{
		ID:        "process-restart-session",
		Title:     "restart proof",
		Workspace: workspace,
		Messages:  []llm.Message{{Role: "user", Content: "resume after process exit"}},
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	store := taskstate.WorkspaceStore(workspace)
	task, err := taskstate.New(s.ID, "resume the interrupted outcome", []string{"recover", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if _, ok := task.BeginTurn(taskstate.TurnRouteImplement); !ok {
		t.Fatal("active turn did not start")
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(root, "helper-result.json")
	cmd := exec.Command(os.Args[0], "-test.run", "^TestLongHorizonResumeAfterProcessExit$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"PICOGENT_LONG_HORIZON_HELPER=1",
		"PICOGENT_HOME="+home,
		"PICOGENT_LONG_HORIZON_WORKSPACE="+workspace,
		"PICOGENT_LONG_HORIZON_TASK_DIR="+filepath.Join(workspace, ".picogent", "tasks"),
		"PICOGENT_LONG_HORIZON_SESSION="+s.ID,
		"PICOGENT_LONG_HORIZON_RESULT="+resultPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fresh process resume failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result longHorizonHelperResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.TurnState != taskstate.TurnInterrupted || result.SessionMessages != 1 || result.TaskRevision != 2 {
		t.Fatalf("fresh process result = %#v, want interrupted/1/revision-2", result)
	}
	loaded, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if last := loaded.LastTurn(); last == nil || last.State != taskstate.TurnInterrupted || last.StopReason != taskstate.StopCanceled {
		t.Fatalf("persisted restart turn = %#v, want canceled interruption", last)
	}
}

func longHorizonResumeHelper(t *testing.T) {
	t.Helper()
	store := taskstate.NewStore(os.Getenv("PICOGENT_LONG_HORIZON_TASK_DIR"))
	task, err := store.Load(os.Getenv("PICOGENT_LONG_HORIZON_SESSION"))
	if err != nil {
		t.Fatal(err)
	}
	last := task.LastTurn()
	if last == nil || last.State != taskstate.TurnActive {
		t.Fatalf("fresh process loaded turn = %#v, want active", last)
	}
	if !task.InterruptTurn(last.Sequence, taskstate.TurnRouteRecover, "process restarted before provider completion", "UNVERIFIED", taskstate.StopCanceled, 0, 0) {
		t.Fatal("fresh process did not interrupt active turn")
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	s, err := session.Load(os.Getenv("PICOGENT_LONG_HORIZON_SESSION"))
	if err != nil {
		t.Fatal(err)
	}
	result := longHorizonHelperResult{TurnState: task.LastTurn().State, SessionMessages: len(s.Messages), TaskRevision: task.Revision}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("PICOGENT_LONG_HORIZON_RESULT"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
