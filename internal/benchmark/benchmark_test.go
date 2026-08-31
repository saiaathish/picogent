// Package benchmark contains deterministic, provider-independent v3 baselines.
//
// These benchmarks measure local work only. They deliberately do not claim to
// measure live model quality, network latency, or end-to-end GUI startup.
package benchmark_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/ctxmgr"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/repomap"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/verify"
)

func BenchmarkContextManage(b *testing.B) {
	cases := []struct {
		name string
		msgs []llm.Message
	}{
		{name: "working-set", msgs: benchmarkMessages(32, 360)},
		{name: "context-heavy", msgs: benchmarkMessages(160, 1800)},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			var last []llm.Message
			var stats ctxmgr.Stats
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var err error
				last, stats, err = ctxmgr.Manage(context.Background(), nil, "benchmark", tc.msgs, ctxmgr.DefaultBudget)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(len(last)), "messages/op")
			b.ReportMetric(float64(stats.Tokens), "tokens/op")
		})
	}
}

func BenchmarkRepoMapInspect(b *testing.B) {
	workspace := benchmarkRepo(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m, err := repomap.Inspect(context.Background(), workspace)
		if err != nil {
			b.Fatal(err)
		}
		if m.InventoryFiles == 0 {
			b.Fatal("benchmark fixture was not inventoried")
		}
	}
}

func BenchmarkRepoMapFormat(b *testing.B) {
	workspace := benchmarkRepo(b)
	m, err := repomap.Inspect(context.Background(), workspace)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatted := repomap.Format(m)
		if formatted == "" || len(formatted) > repomap.MaxOutputBytes {
			b.Fatalf("formatted map length = %d", len(formatted))
		}
	}
	b.SetBytes(int64(len(repomap.Format(m))))
}

func BenchmarkRepoMapCapture(b *testing.B) {
	workspace := benchmarkRepo(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snapshot, err := repomap.Capture(context.Background(), workspace)
		if err != nil {
			b.Fatal(err)
		}
		if snapshot.Root == "" || len(snapshot.ManifestPaths) == 0 {
			b.Fatal("capture fixture was not represented")
		}
	}
}

func BenchmarkRepoMapSnapshotFormat(b *testing.B) {
	workspace := benchmarkRepo(b)
	snapshot, err := repomap.Capture(context.Background(), workspace)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatted := repomap.FormatSnapshot(snapshot)
		if formatted == "" || len(formatted) > repomap.MaxOutputBytes {
			b.Fatalf("formatted snapshot length = %d", len(formatted))
		}
	}
	b.SetBytes(int64(len(repomap.FormatSnapshot(snapshot))))
}

func BenchmarkProjectHealth(b *testing.B) {
	workspace := benchmarkRepo(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		report, err := projecthealth.Assess(context.Background(), workspace)
		if err != nil {
			b.Fatal(err)
		}
		if report.Schema != projecthealth.Schema || len(projecthealth.Format(report)) > projecthealth.MaxOutputBytes {
			b.Fatal("project health report was invalid or unbounded")
		}
	}
}

func BenchmarkProjectHealthFormat(b *testing.B) {
	workspace := benchmarkRepo(b)
	report, err := projecthealth.Assess(context.Background(), workspace)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatted := projecthealth.Format(report)
		if formatted == "" || len(formatted) > projecthealth.MaxOutputBytes {
			b.Fatalf("formatted project health length = %d", len(formatted))
		}
	}
	b.SetBytes(int64(len(projecthealth.Format(report))))
}

func BenchmarkOutcomeFocus(b *testing.B) {
	task := &taskstate.Task{
		Status: taskstate.StatusWorking,
		Intent: &taskstate.IntentContract{NeedsTests: true},
	}
	report := projecthealth.Report{
		Schema: projecthealth.Schema,
		Status: projecthealth.StateAttention,
		Findings: []projecthealth.Finding{
			{ID: "build-unverified", Dimension: "build", Priority: 64},
			{ID: "tests-unverified", Dimension: "tests", Priority: 76},
			{ID: "uncommitted-work", Dimension: "release", Priority: 32},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decision := outcome.Select(task, report)
		if decision.FindingID != "tests-unverified" {
			b.Fatalf("focus = %+v", decision)
		}
		if outcome.Instruction(decision) == "" {
			b.Fatal("focus instruction is empty")
		}
	}
}

func BenchmarkSessionListMeta(b *testing.B) {
	workspace, _ := benchmarkSessions(b, 60)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metas, err := session.ListMeta(workspace)
		if err != nil {
			b.Fatal(err)
		}
		if len(metas) != 60 {
			b.Fatalf("session metadata count = %d, want 60", len(metas))
		}
	}
}

func BenchmarkSessionLoad(b *testing.B) {
	b.Run("canonical", func(b *testing.B) {
		_, target := benchmarkSessions(b, 60)
		benchmarkSessionLoad(b, target, 4)
	})
	b.Run("legacy-history", func(b *testing.B) {
		_, target := benchmarkLegacySession(b)
		benchmarkSessionLoad(b, target, -1)
	})
}

func benchmarkSessionLoad(b *testing.B, target string, wantMessages int) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, err := session.Load(target)
		if err != nil {
			b.Fatal(err)
		}
		if s.ID != target || (wantMessages >= 0 && len(s.Messages) != wantMessages) || (wantMessages < 0 && len(s.Messages) > session.MaxSessionMessages) {
			b.Fatalf("loaded session = %#v", s)
		}
	}
}

func BenchmarkVerificationPlan(b *testing.B) {
	workspace := benchmarkRepo(b)
	targets := []string{"internal/feature/feature.go", "internal/feature/feature_test.go"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plan := verify.DetectPlan(workspace, targets)
		if len(plan.Targeted) != 1 || len(plan.Broader) != 1 {
			b.Fatalf("verification plan = %#v", plan)
		}
	}
}

func BenchmarkVerificationEvidence(b *testing.B) {
	evidence := strings.Repeat("verify PASS\ngo test ./internal/feature\n", 40)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := verify.StatusFromEvidence(evidence); got != verify.StatusPass {
			b.Fatalf("status = %s", got)
		}
	}
}

func BenchmarkVerificationManifest(b *testing.B) {
	pipeline := verify.PipelineResult{
		Status:   verify.StatusPass,
		Duration: 2400 * time.Millisecond,
		Stages: []verify.StageResult{
			{Scope: verify.ScopeTargeted, Status: verify.StatusPass, Evidence: []verify.Result{{
				Scope: verify.ScopeTargeted, Runner: "go", Command: "go test ./internal/feature", Status: verify.StatusPass, Passed: 18,
				Duration: 700 * time.Millisecond,
			}}},
			{Scope: verify.ScopeBroader, Status: verify.StatusPass, Evidence: []verify.Result{{
				Scope: verify.ScopeBroader, Runner: "go", Command: "go test ./...", Status: verify.StatusPass, Passed: 30,
				Duration: 1700 * time.Millisecond,
			}}},
		},
	}
	provenance := verify.HeadEvidence{
		GitRoot:     "/workspace",
		SHA:         "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExpectedSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Match:       verify.ManifestPass,
		Tree:        "CLEAN",
	}
	var output bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output.Reset()
		if err := verify.WriteJSON(&output, verify.ManifestFromPipeline(pipeline, provenance)); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(output.Len()), "bytes/op")
}

func BenchmarkScriptedAgentEdit(b *testing.B) {
	workspace := b.TempDir()
	path := filepath.Join(workspace, "note.txt")
	args, err := json.Marshal(map[string]string{"path": "note.txt", "content": "after\n"})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	lastCalls := 0
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
			b.Fatal(err)
		}
		fake := &llm.Scripted{Responses: []llm.ChatResponse{
			toolResponse("write", "write_file", string(args)),
			{Message: llm.Message{Role: "assistant", Content: "done"}},
		}}
		cfg := config.Default()
		cfg.Workspace = workspace
		cfg.Mode = config.ModeFast
		cfg.Provider = config.ProviderOllama
		a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
		b.StartTimer()
		_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, benchmarkAllow{})
		b.StopTimer()
		if err != nil {
			b.Fatal(err)
		}
		if !result.UndoAvailable {
			b.Fatal("scripted edit did not produce an undo checkpoint")
		}
		lastCalls = len(fake.Calls)
		if _, err := a.UndoLastTurn(); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
	b.StopTimer()
	b.ReportMetric(float64(lastCalls), "model-calls/op")
}

type benchmarkAllow struct{ agent.NopHandler }

func (benchmarkAllow) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Allow, nil
}

func benchmarkMessages(turns, chars int) []llm.Message {
	content := strings.Repeat("context ", chars/8)
	msgs := make([]llm.Message, 0, turns*3+1)
	msgs = append(msgs, llm.Message{Role: "system", Content: "Picogent benchmark workspace"})
	for i := 0; i < turns; i++ {
		msgs = append(msgs,
			llm.Message{Role: "user", Content: fmt.Sprintf("request %d: %s", i, content)},
			llm.Message{Role: "assistant", Content: fmt.Sprintf("planning request %d", i)},
			llm.Message{Role: "tool", Name: "read_file", Content: content},
		)
	}
	return msgs
}

func benchmarkRepo(b *testing.B) string {
	b.Helper()
	workspace := b.TempDir()
	writeBenchmarkFile(b, filepath.Join(workspace, "go.mod"), "module example.com/benchmark\n\ngo 1.25\n")
	writeBenchmarkFile(b, filepath.Join(workspace, "README.md"), "# benchmark fixture\n")
	writeBenchmarkFile(b, filepath.Join(workspace, "internal", "feature", "feature.go"), "package feature\n\nfunc Value() int { return 1 }\n")
	writeBenchmarkFile(b, filepath.Join(workspace, "internal", "feature", "feature_test.go"), "package feature\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) { if Value() != 1 { t.Fatal(\"wrong\") } }\n")
	writeBenchmarkFile(b, filepath.Join(workspace, "services", "api", "package.json"), "{}\n")
	return workspace
}

func benchmarkSessions(b *testing.B, count int) (workspace, target string) {
	b.Helper()
	workspace = b.TempDir()
	b.Setenv("PICOGENT_HOME", b.TempDir())
	for i := 0; i < count; i++ {
		s := session.New(workspace)
		s.ID = fmt.Sprintf("benchmark-%03d", i)
		s.Title = fmt.Sprintf("Benchmark session %03d", i)
		s.Messages = []llm.Message{
			{Role: "user", Content: "continue the task"},
			{Role: "assistant", Content: "I inspected the workspace."},
			{Role: "user", Content: "resume from the checkpoint"},
			{Role: "assistant", Content: "the next step is verification"},
		}
		if err := s.Save(); err != nil {
			b.Fatal(err)
		}
	}
	return workspace, "benchmark-059"
}

func benchmarkLegacySession(b *testing.B) (workspace, target string) {
	b.Helper()
	workspace = b.TempDir()
	b.Setenv("PICOGENT_HOME", b.TempDir())
	s := session.New(workspace)
	s.ID = "benchmark-legacy"
	s.Title = "Benchmark legacy session"
	s.Messages = make([]llm.Message, 0, session.MaxSessionMessages+20)
	for i := 0; i < session.MaxSessionMessages+20; i++ {
		s.Messages = append(s.Messages,
			llm.Message{Role: "user", Content: fmt.Sprintf("legacy request %03d", i)},
			llm.Message{Role: "assistant", Content: fmt.Sprintf("legacy response %03d", i)},
		)
	}
	path, err := s.Path()
	if err != nil {
		b.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		b.Fatal(err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		b.Fatal(err)
	}
	return workspace, s.ID
}

func toolResponse(id, name, args string) llm.ChatResponse {
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: id, Name: name, Arguments: args}}}}
}

func writeBenchmarkFile(b *testing.B, path, content string) {
	b.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatal(err)
	}
}
