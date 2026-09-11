package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestMeasureToolBindsCurrentTrustedEvidence(t *testing.T) {
	root := t.TempDir()
	writeMeasurementFixture(t, root, `package measurementfixture

import "testing"

func BenchmarkExpected(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = i * 2
	}
}
`)

	a := measurementAgent(t, root)
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "improve performance by measuring the current behavior"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil {
		t.Fatal("measurement task was not persisted")
	}
	status, current, origin := result.Task.RequirementEvidenceState(taskstate.EvidenceKindMeasurement)
	if status != "PASS" || !current || origin != taskstate.EvidenceOriginMeasurementTool {
		t.Fatalf("measurement evidence = status=%q current=%v origin=%q task=%#v", status, current, origin, result.Task)
	}
	if len(result.Task.Evidence) == 0 || result.Task.Evidence[len(result.Task.Evidence)-1].Summary == "" {
		t.Fatalf("measurement summary was not retained: %#v", result.Task.Evidence)
	}
}

func TestMeasureToolKeepsUnsupportedMeasurementInconclusive(t *testing.T) {
	a := measurementAgent(t, t.TempDir())
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "improve performance by measuring the current behavior"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil {
		t.Fatal("measurement task was not persisted")
	}
	status, _, origin := result.Task.RequirementEvidenceState(taskstate.EvidenceKindMeasurement)
	if status != "INCONCLUSIVE" || origin != taskstate.EvidenceOriginMeasurementTool {
		t.Fatalf("unsupported measurement evidence = status=%q origin=%q task=%#v", status, origin, result.Task)
	}
	if result.Task.CompletionCheck().Ready {
		t.Fatal("unsupported measurement was accepted as completion proof")
	}
}

func TestMeasureToolKeepsFailedMeasurementNonPassing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/failed-measurement\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeMeasurementFixtureFile(t, root, "bench_test.go", "package failedmeasurement\n\nfunc BenchmarkBroken(\n")

	a := measurementAgent(t, root)
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "improve performance by measuring the current behavior"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil {
		t.Fatal("measurement task was not persisted")
	}
	status, _, origin := result.Task.RequirementEvidenceState(taskstate.EvidenceKindMeasurement)
	if status != "FAIL" || origin != taskstate.EvidenceOriginMeasurementTool {
		t.Fatalf("failed measurement evidence = status=%q origin=%q task=%#v", status, origin, result.Task)
	}
	if result.Task.CompletionCheck().Ready {
		t.Fatal("failed measurement was accepted as completion proof")
	}
}

func measurementAgent(t *testing.T, root string) *agent.Agent {
	t.Helper()
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "measure", Name: "measure", Arguments: `{}`}}}},
		{Message: llm.Message{Role: "assistant", Content: "Measured the current performance."}},
	}}
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = root
	cfg.Mode = config.ModeFast
	store := taskstate.NewStore(t.TempDir())
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: root}), perm.New(config.ModeFast, root, nil))
	a.TaskStore = store
	if err := a.SetTaskSession("measurement-evidence"); err != nil {
		t.Fatal(err)
	}
	return a
}

func writeMeasurementFixture(t *testing.T, root, benchmark string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/measurement\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeMeasurementFixtureFile(t, root, "bench_test.go", benchmark)
}

func writeMeasurementFixtureFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
