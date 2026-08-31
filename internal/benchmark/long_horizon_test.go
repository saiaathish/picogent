package benchmark_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/evolve"
	"github.com/saiaathish/picogent/internal/learn"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/trace"
)

const (
	longSessionHelperEnv   = "PICOGENT_LONG_SESSION_HELPER"
	longSessionTurnsEnv    = "PICOGENT_LONG_SESSION_TURNS"
	longSessionRootEnv     = "PICOGENT_LONG_SESSION_ROOT"
	longSessionResultEnv   = "PICOGENT_LONG_SESSION_RESULT"
	longSessionMaxTurns    = 256
	longSessionTraceLimit  = 256 << 10
	longSessionLearnLimit  = 256 << 10
	longSessionEvolveLimit = 256 << 10
	longSessionTaskLimit   = 1 << 20
)

// TestLongSessionGrowth measures the same-process persistence envelope at
// several horizons. Each horizon runs in a fresh child so the parent can
// record that process's maximum RSS separately from the Go test runner.
//
// This is a deterministic local fixture: it exercises bounded trace, learned,
// evolved, session, and task records without claiming live-provider quality or
// GUI/browser memory behavior.
func TestLongSessionGrowth(t *testing.T) {
	if os.Getenv(longSessionHelperEnv) == "1" {
		runLongSessionHelper(t)
		return
	}

	const (
		shortHorizon = 1
		midHorizon   = 64
		fullHorizon  = longSessionMaxTurns
	)
	samples := make([]longSessionReport, 0, 3)
	for _, turns := range []int{shortHorizon, midHorizon, fullHorizon} {
		sample := runLongSessionChild(t, turns)
		if sample.Turns != turns {
			t.Fatalf("long-session child turns=%d, want %d", sample.Turns, turns)
		}
		if len(sample.Points) == 0 {
			t.Fatalf("long-session child at %d turns returned no checkpoints", turns)
		}
		if (runtime.GOOS == "darwin" || runtime.GOOS == "linux") && sample.ChildMaxRSSBytes <= 0 {
			t.Fatalf("long-session child at %d turns did not report max RSS", turns)
		}
		for _, point := range sample.Points {
			assertLongSessionBounds(t, sample.Turns, point)
		}
		samples = append(samples, sample)
	}

	for _, sample := range samples {
		final := sample.Points[len(sample.Points)-1]
		if final.TraceSeq != uint64(sample.Turns) {
			t.Fatalf("%d-turn trace sequence=%d, want %d", sample.Turns, final.TraceSeq, sample.Turns)
		}
		if final.TaskTurnRevision != uint64(sample.Turns) {
			t.Fatalf("%d-turn task revision=%d, want %d", sample.Turns, final.TaskTurnRevision, sample.Turns)
		}
		t.Logf("long-session turns=%d child-max-rss=%s checkpoints=%s", sample.Turns, formatBytes(sample.ChildMaxRSSBytes), formatLongSessionPoints(sample.Points))
	}
}

type longSessionReport struct {
	Turns            int                `json:"turns"`
	ChildMaxRSSBytes int64              `json:"child_max_rss_bytes"`
	Points           []longSessionPoint `json:"points"`
}

type longSessionPoint struct {
	Turn              int    `json:"turn"`
	TraceBytes        int64  `json:"trace_bytes"`
	LearnBytes        int64  `json:"learn_bytes"`
	EvolveBytes       int64  `json:"evolve_bytes"`
	SessionBytes      int64  `json:"session_bytes"`
	TaskBytes         int64  `json:"task_bytes"`
	TraceSeq          uint64 `json:"trace_seq"`
	SessionMessages   int    `json:"session_messages"`
	LearnFiles        int    `json:"learn_files"`
	LearnChanges      int    `json:"learn_changes"`
	LearnTools        int    `json:"learn_tools"`
	EvolveHabits      int    `json:"evolve_habits"`
	EvolvePlaybooks   int    `json:"evolve_playbooks"`
	EvolveFailures    int    `json:"evolve_failures"`
	EvolveRoutes      int    `json:"evolve_routes"`
	TaskTurnRevision  uint64 `json:"task_turn_revision"`
	TaskRetainedTurns int    `json:"task_retained_turns"`
}

func runLongSessionChild(t *testing.T, turns int) longSessionReport {
	t.Helper()
	root := t.TempDir()
	resultPath := filepath.Join(root, "result.json")
	cmd := exec.Command(os.Args[0], "-test.run", "^TestLongSessionGrowth$", "-test.count=1")
	cmd.Env = replaceEnv(os.Environ(), map[string]string{
		longSessionHelperEnv: "1",
		longSessionTurnsEnv:  strconv.Itoa(turns),
		longSessionRootEnv:   root,
		longSessionResultEnv: resultPath,
	})
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("long-session child at %d turns: %v\n%s", turns, err, output)
	}
	childRSS := childMaxRSSBytes(cmd.ProcessState)
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read long-session result at %d turns: %v", turns, err)
	}
	var report longSessionReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode long-session result at %d turns: %v", turns, err)
	}
	report.ChildMaxRSSBytes = childRSS
	return report
}

func runLongSessionHelper(t *testing.T) {
	turns, err := strconv.Atoi(os.Getenv(longSessionTurnsEnv))
	if err != nil || turns < 1 || turns > longSessionMaxTurns {
		t.Fatalf("invalid long-session turns %q", os.Getenv(longSessionTurnsEnv))
	}
	root := os.Getenv(longSessionRootEnv)
	resultPath := os.Getenv(longSessionResultEnv)
	if root == "" || resultPath == "" {
		t.Fatal("long-session child paths are required")
	}
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)

	workspaceID := projects.IDForPath(workspace)
	traceLog, err := trace.Open(workspace)
	if err != nil {
		t.Fatalf("open long-session trace: %v", err)
	}
	taskStore := taskstate.NewStore(filepath.Join(home, "tasks", workspaceID))
	const (
		sessionID = "long-session"
		taskID    = "long-session-task"
	)
	history := make([]llm.Message, 0, turns*3)
	checkpoints := map[int]bool{1: true, 64: true, longSessionMaxTurns: true}
	report := longSessionReport{Turns: turns}

	for turn := 1; turn <= turns; turn++ {
		if err := traceLog.Append("turn_end", "long_session", fmt.Sprintf("completed deterministic turn %03d %s", turn, strings.Repeat("x", 220)), trace.Bool(true), int64(turn)); err != nil {
			t.Fatalf("turn %d append trace: %v", turn, err)
		}

		learned, err := learn.Load(workspace)
		if err != nil {
			t.Fatalf("turn %d load learn state: %v", turn, err)
		}
		learned.RecordTurn()
		learned.RecordSearch()
		learned.RecordRead(fmt.Sprintf("internal/feature/file-%03d.go", turn))
		learned.RecordChange(fmt.Sprintf("internal/feature/file-%03d.go", turn), turn, 1)
		learned.RecordTool(fmt.Sprintf("tool-%03d", turn))
		learned.RecordTest(1, 0, 0, strings.Repeat("test evidence ", 300))
		if err := learn.Save(&learned); err != nil {
			t.Fatalf("turn %d save learn state: %v", turn, err)
		}

		_, err = evolve.Update(workspace, func(store evolve.Store) (evolve.Store, error) {
			store, _, _ = evolve.UpsertHabit(store, fmt.Sprintf("preserve deterministic turn %03d evidence", turn), "long-session")
			store, _, _ = evolve.UpsertPlaybook(store, fmt.Sprintf("turn route %03d", turn), strings.Repeat("bounded route evidence ", 20), fmt.Sprintf("class-%02d", turn), "long-session")
			store = evolve.RecordFailure(store, fmt.Sprintf("class-%02d", turn), "FAIL deterministic fixture evidence")
			if turn%3 == 0 {
				store = evolve.RecordVerificationRoute(store, fmt.Sprintf("class-%02d", turn), []string{fmt.Sprintf("internal/feature/file-%03d.go", turn)}, "PASS\ngo test ./...")
			}
			return store, nil
		})
		if err != nil {
			t.Fatalf("turn %d update evolve state: %v", turn, err)
		}

		history = append(history,
			llm.Message{Role: "user", Content: fmt.Sprintf("long-session request %03d %s", turn, strings.Repeat("request ", 32))},
			llm.Message{Role: "assistant", Content: fmt.Sprintf("completed long-session request %03d %s", turn, strings.Repeat("response ", 32))},
			llm.Message{Role: "tool", Name: "inspect", Content: strings.Repeat("tool result ", 32)},
		)
		if err := session.SaveMessages(workspace, sessionID, history); err != nil {
			t.Fatalf("turn %d save session: %v", turn, err)
		}

		task, err := taskStore.Load(taskID)
		if errors.Is(err, taskstate.ErrNotFound) {
			task, err = taskstate.New(taskID, "measure long-session persistence", []string{"run deterministic turns", "retain bounded state"})
		} else if err != nil {
			t.Fatalf("turn %d load task state: %v", turn, err)
		}
		if err != nil {
			t.Fatalf("turn %d create task state: %v", turn, err)
		}
		if task.Status == taskstate.StatusPlanning {
			if err := task.SetStatus(taskstate.StatusWorking); err != nil {
				t.Fatalf("turn %d start task: %v", turn, err)
			}
		}
		task.Attempts = turn
		if task.Attempts > 128 {
			task.Attempts = 128
		}
		sequence, ok := task.BeginTurn(taskstate.TurnRouteInspect)
		if !ok || !task.FinishTurn(sequence, taskstate.TurnRouteInspect, "bounded deterministic persistence", "UNVERIFIED", taskstate.StopNone, 1, 0) {
			t.Fatalf("turn %d close task turn", turn)
		}
		if err := taskStore.Save(task); err != nil {
			t.Fatalf("turn %d save task state: %v", turn, err)
		}

		if !checkpoints[turn] {
			continue
		}
		point := longSessionPoint{Turn: turn}
		point.TraceBytes = requiredFileSize(t, traceLog.Path())
		point.LearnBytes = requiredFileSize(t, filepath.Join(home, "learn", workspaceID+".json"))
		point.EvolveBytes = requiredFileSize(t, filepath.Join(home, "evolve", workspaceID+".json"))
		savedSession, err := session.Load(sessionID)
		if err != nil {
			t.Fatalf("turn %d reload session: %v", turn, err)
		}
		sessionPath, err := savedSession.Path()
		if err != nil {
			t.Fatalf("turn %d session path: %v", turn, err)
		}
		point.SessionBytes = requiredFileSize(t, sessionPath)
		point.SessionMessages = len(savedSession.Messages)

		savedLearn, err := learn.Load(workspace)
		if err != nil {
			t.Fatalf("turn %d reload learn state: %v", turn, err)
		}
		point.LearnFiles = len(savedLearn.FilesRead)
		point.LearnChanges = len(savedLearn.FilesChanged)
		point.LearnTools = len(savedLearn.ToolCounts)

		savedEvolve, err := evolve.Load(workspace)
		if err != nil {
			t.Fatalf("turn %d reload evolve state: %v", turn, err)
		}
		point.EvolveHabits = len(savedEvolve.Habits)
		point.EvolvePlaybooks = len(savedEvolve.Playbooks)
		point.EvolveFailures = len(savedEvolve.Failures)
		point.EvolveRoutes = len(savedEvolve.VerificationRoutes)

		latestTrace := traceLog.Tail(1)
		if len(latestTrace) != 1 {
			t.Fatalf("turn %d trace tail empty", turn)
		}
		point.TraceSeq = uint64(latestTrace[0].Seq)
		savedTask, err := taskStore.Load(taskID)
		if err != nil {
			t.Fatalf("turn %d reload task state: %v", turn, err)
		}
		taskPath, err := taskStore.Path(taskID)
		if err != nil {
			t.Fatalf("turn %d task path: %v", turn, err)
		}
		point.TaskBytes = requiredFileSize(t, taskPath)
		point.TaskTurnRevision = savedTask.TurnRevision
		point.TaskRetainedTurns = len(savedTask.Turns)
		report.Points = append(report.Points, point)
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, data, 0o600); err != nil {
		t.Fatalf("write long-session result: %v", err)
	}
}

func assertLongSessionBounds(t *testing.T, turns int, point longSessionPoint) {
	t.Helper()
	if point.Turn < 1 || point.Turn > turns {
		t.Fatalf("checkpoint turn=%d outside 1..%d", point.Turn, turns)
	}
	for name, size := range map[string]int64{
		"trace": point.TraceBytes, "learn": point.LearnBytes, "evolve": point.EvolveBytes,
		"session": point.SessionBytes, "task": point.TaskBytes,
	} {
		if size <= 0 {
			t.Fatalf("%d-turn %s store size=%d, want positive", point.Turn, name, size)
		}
		if size > limitFor(name) {
			t.Fatalf("%d-turn %s store size=%d exceeds limit=%d", point.Turn, name, size, limitFor(name))
		}
	}
	if point.SessionMessages > session.MaxSessionMessages {
		t.Fatalf("%d-turn session messages=%d exceeds %d", point.Turn, point.SessionMessages, session.MaxSessionMessages)
	}
	if point.LearnFiles > 256 || point.LearnChanges > 256 || point.LearnTools > 64 {
		t.Fatalf("%d-turn learn map sizes files=%d changes=%d tools=%d exceed bounds", point.Turn, point.LearnFiles, point.LearnChanges, point.LearnTools)
	}
	if point.EvolveHabits > 5 || point.EvolvePlaybooks > 4 || point.EvolveFailures > 6 || point.EvolveRoutes > 6 {
		t.Fatalf("%d-turn evolve sizes habits=%d playbooks=%d failures=%d routes=%d exceed bounds", point.Turn, point.EvolveHabits, point.EvolvePlaybooks, point.EvolveFailures, point.EvolveRoutes)
	}
	if point.TaskRetainedTurns > 16 {
		t.Fatalf("%d-turn retained task turns=%d exceed 16", point.Turn, point.TaskRetainedTurns)
	}
}

func limitFor(name string) int64 {
	switch name {
	case "trace":
		return longSessionTraceLimit
	case "learn":
		return longSessionLearnLimit
	case "evolve":
		return longSessionEvolveLimit
	case "session":
		return session.MaxSessionBytes
	case "task":
		return longSessionTaskLimit
	default:
		return 0
	}
}

func requiredFileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}

func replaceEnv(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		out = append(out, entry)
	}
	for key, value := range overrides {
		out = append(out, key+"="+value)
	}
	return out
}

func formatBytes(value int64) string {
	if value <= 0 {
		return "unavailable"
	}
	return fmt.Sprintf("%dB", value)
}

func formatLongSessionPoints(points []longSessionPoint) string {
	parts := make([]string, 0, len(points))
	for _, point := range points {
		parts = append(parts, fmt.Sprintf("%d:{trace=%d,learn=%d,evolve=%d,session=%d,task=%d,msg=%d,turns=%d}", point.Turn, point.TraceBytes, point.LearnBytes, point.EvolveBytes, point.SessionBytes, point.TaskBytes, point.SessionMessages, point.TaskRetainedTurns))
	}
	return strings.Join(parts, ";")
}
