package agent

import (
	"context"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

const benchmarkAdmissionHealthReport = `{"schema":"picogent.project-health.v1","status":"ATTENTION","shape":{},"provenance":{"head_known":true,"dirty_known":true},"dimensions":[]}`

// BenchmarkProjectHealthAdmissionRouting measures the real admission seam for
// an eligible broad request and a tiny request. The synthetic report keeps the
// benchmark focused on routing and bounded contract construction rather than
// repository size, Git availability, or provider latency.
func BenchmarkProjectHealthAdmissionRouting(b *testing.B) {
	workspace := b.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	b.Cleanup(reg.Close)
	a := New(cfg, &llm.Scripted{}, reg, perm.New(config.ModeFast, workspace, nil))
	a.runTool = func(_ context.Context, call llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
		if call.Name != "project_health" || call.ID != projectHealthAdmissionCallID || call.Arguments != "{}" {
			b.Fatalf("unexpected admission call: %#v", call)
		}
		return benchmarkAdmissionHealthReport, nil
	}
	state := a.RuntimeSnapshot()
	regCtx := reg.ContextSnapshot()
	gate := perm.New(config.ModeFast, workspace, nil)

	b.Run("broad", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			admission := a.admitProjectHealth(context.Background(), "make this ready to launch", TaskAgent, "", state, regCtx, gate)
			if !admission.attempted || admission.focus == "" {
				b.Fatal("broad request did not complete bounded admission")
			}
		}
	})

	b.Run("tiny", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			admission := a.admitProjectHealth(context.Background(), "change the button text", TaskAgent, "", state, regCtx, gate)
			if admission.attempted || admission.focus != "" {
				b.Fatal("tiny request incurred broad admission")
			}
		}
	})
}
