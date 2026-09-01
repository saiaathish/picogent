package gui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/workspace"
)

func TestGUIQueuedTurnRebuildAndReloadKeepCompletionProofBound(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspaceRoot := t.TempDir()
	const sessionID = "gui-proof-continuity"
	const queuedFileContent = "package feature\n\n// changed by the queued turn\n"

	if err := os.WriteFile(filepath.Join(workspaceRoot, "feature.go"), []byte("package feature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	observation, err := workspace.Capture(context.Background(), workspaceRoot, []string{"feature.go"})
	if err != nil {
		t.Fatal(err)
	}

	store := taskstate.NewStore(t.TempDir())
	task, err := taskstate.New(sessionID, "ship a verified change", []string{"implement", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []taskstate.Criterion{
		{Description: "implementation is complete", Required: true},
		{Description: "verification is complete", Required: true},
	}
	if !task.SetIntent(&taskstate.IntentContract{
		Outcome:    task.Goal,
		Class:      "implementation",
		Action:     "deliver",
		NeedsTests: true,
	}) {
		t.Fatal("initial intent was not recorded")
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	task.RecordChanged("feature.go")
	task.AddVerificationForCriteria([]int{0, 1}, "verify", true, "verify PASS\ninitial proof", &observation)
	if err := task.SetStatus(taskstate.StatusDone); err != nil {
		t.Fatalf("seed task could not become complete: %v (%#v)", err, task.CompletionCheck())
	}
	if !task.CompletionReady() {
		t.Fatalf("seed task proof is not current: %#v", task.CompletionCheck())
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	if err := (&session.Session{ID: sessionID, Title: "Proof continuity", Workspace: workspaceRoot}).Save(); err != nil {
		t.Fatal(err)
	}

	writeArgs, err := json.Marshal(map[string]string{
		"path":    "feature.go",
		"content": queuedFileContent,
	})
	if err != nil {
		t.Fatal(err)
	}
	scripted := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "The current state is unchanged."}},
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write", Name: "write_file", Arguments: string(writeArgs)}}}},
		{Message: llm.Message{Role: "assistant", Content: "The follow-up changed the file."}},
	}}
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspaceRoot
	gate := perm.New(config.ModeFast, workspaceRoot, nil)
	gate.AddAlwaysAllowed("write_file")
	old := agent.New(cfg, scripted, tools.NewRegistry(tools.Context{Workspace: workspaceRoot}), gate)
	defer old.Close()
	old.SetTaskStore(store)
	if err := old.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}

	events := make(chan event, 128)
	firstReady := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondAdmission := make(chan *taskstate.Task, 1)
	releaseSecond := make(chan struct{})
	var firstOnce sync.Once
	var secondOnce sync.Once
	var seenMu sync.Mutex
	var seenAgents []*agent.Agent
	var s *server
	s = &server{
		cfg:       cfg,
		ag:        old,
		sessionID: sessionID,
		permCh:    make(chan perm.Decision, 1),
		subs:      []chan event{events},
		beforeAgentRun: func() {
			s.mu.Lock()
			current := s.ag
			if current != nil {
				current.SetClient(scripted)
				current.Tools.UpdateContext(func(c *tools.Context) {
					// Keep this lifecycle contract deterministic: the queued write
					// must be observed before any automatic verifier can rebind proof.
					c.Verify = nil
					c.VerifyTargets = nil
				})
			}
			s.mu.Unlock()
			seenMu.Lock()
			seenAgents = append(seenAgents, current)
			seenMu.Unlock()

			first := false
			firstOnce.Do(func() {
				first = true
				close(firstReady)
			})
			if first {
				<-releaseFirst
				return
			}
			secondOnce.Do(func() {
				secondAdmission <- current.TaskSnapshot()
				<-releaseSecond
			})
		},
	}

	s.startAgentTurn("Begin the verified change", nil)
	select {
	case <-firstReady:
	case <-time.After(10 * time.Second):
		t.Fatal("initial GUI turn did not reach the pre-run barrier")
	}
	if !s.queueSteer("Apply the queued change", nil, "Apply the queued change") {
		t.Fatal("queued GUI follow-up was rejected")
	}

	if err := s.rebuildAgent(); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	rebuilt := s.ag
	s.mu.Unlock()
	if rebuilt == nil || rebuilt == old {
		t.Fatal("GUI rebuild did not publish a replacement agent")
	}
	defer rebuilt.Close()
	rebuilt.SetClient(scripted)
	rebuilt.Gate.SetAlwaysAllowed([]string{"write_file"})
	rebuilt.Tools.UpdateContext(func(c *tools.Context) {
		// Keep this lifecycle contract deterministic: the queued write must be
		// observed before any automatic verifier can rebind proof.
		c.Verify = nil
		c.VerifyTargets = nil
	})
	if rebuilt.TaskSession != sessionID {
		t.Fatalf("rebuilt task session = %q, want %q", rebuilt.TaskSession, sessionID)
	}
	if got := rebuilt.TaskSnapshot(); got == nil || got.ID != task.ID || !got.CompletionReady() {
		t.Fatalf("rebuilt task proof = %#v, want current proof for task %q", got, task.ID)
	}

	close(releaseFirst)
	var admittedProof *taskstate.Task
	select {
	case admittedProof = <-secondAdmission:
	case <-time.After(15 * time.Second):
		t.Fatal("queued GUI turn did not reach rebuilt-agent admission")
	}
	if admittedProof == nil || admittedProof.ID != task.ID || !admittedProof.CompletionReady() || admittedProof.ChangeSeq != task.ChangeSeq {
		t.Fatalf("queued admission proof = %#v, want the current pre-write proof", admittedProof)
	}
	close(releaseSecond)

	waitForGUIProofContinuityIdle(t, s)
	seenMu.Lock()
	gotAgents := append([]*agent.Agent(nil), seenAgents...)
	seenMu.Unlock()
	if len(gotAgents) != 2 || gotAgents[0] != old || gotAgents[1] == old {
		t.Fatalf("turn runtimes = count=%d %#v, want initial %p followed by a rebuilt runtime", len(gotAgents), gotAgents, old)
	}
	finalAgent := gotAgents[1]
	defer finalAgent.Close()

	s.mu.Lock()
	final := s.ag.TaskSnapshot()
	s.mu.Unlock()
	if final == nil || final.ID != task.ID || final.SessionID != sessionID {
		t.Fatalf("final GUI task = %#v, want task %q in session %q", final, task.ID, sessionID)
	}
	feature, err := os.ReadFile(filepath.Join(workspaceRoot, "feature.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(feature) != queuedFileContent {
		t.Fatalf("queued write content = %q, want %q", feature, queuedFileContent)
	}
	if final.ChangeSeq != task.ChangeSeq+1 {
		t.Fatalf("queued write change sequence = %d, want %d", final.ChangeSeq, task.ChangeSeq+1)
	}
	lastTurn := final.LastTurn()
	if lastTurn == nil || lastTurn.MutationCount != 1 || len(lastTurn.ChangedFiles) != 1 || lastTurn.ChangedFiles[0] != "feature.go" {
		t.Fatalf("queued durable turn = %#v, want one mutation to feature.go", lastTurn)
	}
	if final.CompletionReady() {
		t.Fatalf("queued write reused older proof: %#v", final.CompletionCheck())
	}
	if !final.NeedsVerification() {
		t.Fatalf("queued write did not leave the task awaiting fresh verification: %#v", final)
	}

	var lastProgress event
	progressCount := 0
	for {
		select {
		case got := <-events:
			if got.Type == "task_progress" {
				lastProgress = got
				progressCount++
			}
		default:
			goto drained
		}
	}
drained:
	if progressCount == 0 || lastProgress.Task == nil || lastProgress.Completion == nil || lastProgress.Completion.Ready {
		t.Fatalf("GUI task projection after queued write = %#v, task=%#v, progress=%d", lastProgress.Completion, lastProgress.Task, progressCount)
	}

	saved, err := session.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	var users []string
	for _, message := range saved.Messages {
		if message.Role == "user" {
			users = append(users, message.Content)
		}
	}
	if len(users) != 2 || users[0] != "Begin the verified change" || users[1] != "Apply the queued change" {
		t.Fatalf("queued transcript users = %q, want FIFO admission", users)
	}

	fresh := agent.New(cfg, scripted, tools.NewRegistry(tools.Context{Workspace: workspaceRoot}), perm.New(config.ModeFast, workspaceRoot, nil))
	defer fresh.Close()
	fresh.SetTaskStore(store)
	if err := fresh.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	reloaded := fresh.TaskSnapshot()
	if reloaded == nil || reloaded.CompletionReady() {
		t.Fatalf("fresh same-session reload reused stale proof: %#v", reloaded)
	}
	if reloaded.ChangeSeq != final.ChangeSeq || reloaded.TurnRevision != final.TurnRevision || len(reloaded.Turns) != len(final.Turns) {
		t.Fatalf("reload lost durable chronology: got change=%d turn=%d turns=%d, want change=%d turn=%d turns=%d", reloaded.ChangeSeq, reloaded.TurnRevision, len(reloaded.Turns), final.ChangeSeq, final.TurnRevision, len(final.Turns))
	}

	currentObservation, err := workspace.Capture(context.Background(), workspaceRoot, []string{"feature.go"})
	if err != nil {
		t.Fatal(err)
	}
	reloaded.AddVerificationForCriteria([]int{0, 1}, "verify", true, "verify PASS\nqueued state rechecked", &currentObservation)
	if !reloaded.CompletionReady() {
		t.Fatalf("fresh criterion-bound proof did not rebind: %#v", reloaded.CompletionCheck())
	}
	if err := store.Save(reloaded); err != nil {
		t.Fatal(err)
	}
	if err := fresh.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	reloadEvents := make(chan event, 2)
	reloadedServer := &server{ag: fresh, sessionID: sessionID, subs: []chan event{reloadEvents}}
	reloadedServer.emitTaskSnapshot(sessionID)
	select {
	case got := <-reloadEvents:
		if got.Completion == nil || !got.Completion.Ready || got.Task == nil || got.Task.ID != task.ID {
			t.Fatalf("GUI projection after fresh proof = %#v, task=%#v", got.Completion, got.Task)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GUI did not project freshly rebound completion proof")
	}
}

func waitForGUIProofContinuityIdle(t *testing.T, s *server) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		s.mu.Lock()
		active, busy := s.activeTurns, s.busy
		s.mu.Unlock()
		s.steerMu.Lock()
		queued := len(s.steerQueue)
		s.steerMu.Unlock()
		if active == 0 && !busy && queued == 0 {
			s.waitForTurns()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("GUI did not become idle: active=%d busy=%v queued=%d", active, busy, queued)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
