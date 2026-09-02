package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/testsupport"
)

func TestTUIProjectsLongHorizonOutcomeStates(t *testing.T) {
	cases, err := testsupport.NewCompletionProjectionCases()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			want := outcome.EvaluateCompletion(tc.Task)
			contract := outcome.Build(tc.Task, projecthealth.Report{Schema: projecthealth.Schema})
			last := tc.Task.LastTurn()
			if last == nil || last.Sequence == 0 {
				t.Fatal("projection fixture has no durable turn identity")
			}
			turnID := last.Sequence
			var emitted tea.Msg
			h := &handler{send: func(msg tea.Msg) { emitted = msg }, turnID: turnID, sessionID: tc.Task.SessionID}
			h.OnTaskState(tc.Task)

			progress, ok := emitted.(taskProgressMsg)
			if !ok {
				t.Fatalf("TUI task-progress message = %T, want taskProgressMsg", emitted)
			}
			if !reflect.DeepEqual(progress.completion, want) {
				t.Fatalf("TUI event proof = %#v, want durable proof %#v", progress.completion, want)
			}
			if progress.turnID != turnID || progress.sessionID != tc.Task.SessionID {
				t.Fatalf("TUI event identity = turn=%d session=%q, want turn=%d session=%q", progress.turnID, progress.sessionID, turnID, tc.Task.SessionID)
			}

			m := &model{
				sessionID: tc.Task.SessionID,
				turnID:    turnID,
				task:      &taskstate.Task{SessionID: tc.Task.SessionID},
				vp:        viewport.New(80, 20),
				lines:     []logLine{{Kind: "system", Text: "current"}},
			}
			_, _ = m.Update(progress)
			if m.task != tc.Task || !reflect.DeepEqual(m.completion, want) {
				t.Fatalf("TUI stored task/proof = %#v/%#v, want %#v/%#v", m.task, m.completion, tc.Task, want)
			}

			rendered := formatTaskProgressWithProof(m.task, m.completion)
			if rendered == "" {
				t.Fatal("TUI task progress was empty")
			}
			label := "proof pending"
			if tc.WantReady {
				label = "proof ready"
			}
			if !strings.Contains(rendered, label) {
				t.Fatalf("TUI task progress = %q, want %q", rendered, label)
			}
			if contract.Turn.TurnSequence != last.Sequence || contract.Turn.LastTurnState != string(last.State) || contract.Turn.LastRoute != last.Route {
				t.Fatalf("TUI turn projection = %#v, want durable turn %#v", contract.Turn, last)
			}
			wantRecovery := tc.State == testsupport.StateRecoveryPending
			if contract.Turn.NeedsRecovery() != wantRecovery {
				t.Fatalf("TUI recovery projection = %v, want %v: %#v", contract.Turn.NeedsRecovery(), wantRecovery, contract.Turn)
			}
			if tc.State == testsupport.StateCurrentProof && contract.Stop.Policy != outcome.StopRecheck {
				t.Fatalf("TUI current-proof stop policy = %q, want %q", contract.Stop.Policy, outcome.StopRecheck)
			}
		})
	}
}

func TestTUIReloadProjectionRequiresFreshWorkspaceEvidence(t *testing.T) {
	workspaceRoot := t.TempDir()
	fixture, err := testsupport.NewWorkspaceBoundCompletionFixture(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	if err := store.Save(fixture.Task); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Load(fixture.Task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	wantPending := outcome.EvaluateCompletion(reloaded)
	if wantPending.Ready {
		t.Fatalf("reload retained runtime proof: %#v", wantPending)
	}
	last := reloaded.LastTurn()
	if last == nil || last.Sequence == 0 {
		t.Fatal("reload fixture has no durable turn identity")
	}
	events := make(chan tea.Msg, 2)
	h := &handler{send: func(msg tea.Msg) { events <- msg }, turnID: last.Sequence, sessionID: reloaded.SessionID}
	m := &model{
		sessionID: reloaded.SessionID,
		turnID:    last.Sequence,
		task:      &taskstate.Task{SessionID: reloaded.SessionID},
		vp:        viewport.New(80, 20),
		lines:     []logLine{{Kind: "system", Text: "current"}},
	}

	h.OnTaskState(reloaded)
	pendingMsg, ok := (<-events).(taskProgressMsg)
	if !ok || pendingMsg.turnID != last.Sequence || pendingMsg.sessionID != reloaded.SessionID || pendingMsg.completion.Ready || !reflect.DeepEqual(pendingMsg.completion, wantPending) {
		t.Fatalf("TUI reloaded proof = %#v, want tagged fail-closed %#v", pendingMsg, wantPending)
	}
	_, _ = m.Update(pendingMsg)
	if m.task != reloaded || !reflect.DeepEqual(m.completion, wantPending) {
		t.Fatalf("TUI stored reloaded task/proof = %#v/%#v, want %#v/%#v", m.task, m.completion, reloaded, wantPending)
	}

	if !reloaded.ReestablishWorkspaceVerification(&fixture.Observation) {
		t.Fatal("fresh workspace observation did not rebind TUI proof")
	}
	wantReady := outcome.EvaluateCompletion(reloaded)
	h.OnTaskState(reloaded)
	readyMsg, ok := (<-events).(taskProgressMsg)
	if !ok || readyMsg.turnID != last.Sequence || readyMsg.sessionID != reloaded.SessionID || !readyMsg.completion.Ready || !reflect.DeepEqual(readyMsg.completion, wantReady) {
		t.Fatalf("TUI rebound proof = %#v, want tagged ready %#v", readyMsg, wantReady)
	}
	_, _ = m.Update(readyMsg)
	if !reflect.DeepEqual(m.completion, wantReady) {
		t.Fatalf("TUI stored rebound proof = %#v, want %#v", m.completion, wantReady)
	}
}
