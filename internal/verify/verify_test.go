package verify

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDetectGo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner, cmd, args := Detect(dir)
	if runner != "go" || cmd != "go test ./..." || len(args) < 2 {
		t.Fatalf("%s %s %v", runner, cmd, args)
	}
}

func TestDetectNone(t *testing.T) {
	runner, _, _ := Detect(t.TempDir())
	if runner != "" {
		t.Fatal(runner)
	}
}

func TestRunNoneIsInconclusive(t *testing.T) {
	res := Run(t.Context(), t.TempDir())
	if res.OK || res.Runner != "none" || res.Status != StatusInconclusive {
		t.Fatalf("%+v", res)
	}
	got := Format(res)
	if !strings.Contains(got, "INCONCLUSIVE") {
		t.Fatalf("format: %q", got)
	}
}

func TestDetectPlanTargetsGoPackageThenBroader(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	plan := DetectPlan(dir, []string{"internal/auth/auth.go", "internal/auth/auth_test.go"})
	if len(plan.Targeted) != 1 || plan.Targeted[0].Display != "go test ./internal/auth" {
		t.Fatalf("targeted: %+v", plan.Targeted)
	}
	if len(plan.Broader) != 1 || plan.Broader[0].Display != "go test ./..." {
		t.Fatalf("broader: %+v", plan.Broader)
	}
}

func TestRunPipelinePassEvidenceOrder(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	var commands []string
	result := RunPipeline(t.Context(), dir, Options{
		Targets: []string{"internal/auth/auth.go"},
		Executor: func(_ context.Context, _ string, command Command, attempt int, _ time.Duration) Result {
			commands = append(commands, command.Display)
			return Result{OK: true, Status: StatusPass, Output: "ok", Passed: 1, Attempt: attempt}
		},
	})
	if result.Status != StatusPass {
		t.Fatalf("%+v", result)
	}
	if strings.Join(commands, "|") != "go test ./internal/auth|go test ./..." {
		t.Fatalf("commands: %v", commands)
	}
	if len(result.Stages) != 2 || result.Stages[0].Status != StatusPass || result.Stages[1].Status != StatusPass {
		t.Fatalf("stages: %+v", result.Stages)
	}
}

func TestRunPipelineCapsRepairAttempts(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	repairs := 0
	result := RunPipeline(t.Context(), dir, Options{
		Targets:           []string{"internal/auth/auth.go"},
		MaxRepairAttempts: 99,
		Executor: func(_ context.Context, _ string, _ Command, attempt int, _ time.Duration) Result {
			return Result{Status: StatusFail, Reason: "still broken", Failed: 1, Attempt: attempt}
		},
		Repair: func(_ context.Context, request RepairRequest) error {
			repairs++
			if request.Number != repairs || request.MaxAttempts != HardMaxRepairAttempts {
				t.Fatalf("request: %+v", request)
			}
			return nil
		},
	})
	if result.Status != StatusFail || repairs != HardMaxRepairAttempts {
		t.Fatalf("result=%+v repairs=%d", result, repairs)
	}
	if result.MaxRepairAttempts != HardMaxRepairAttempts || len(result.RepairAttempts) != HardMaxRepairAttempts {
		t.Fatalf("repair metadata: %+v", result)
	}
	if len(result.Stages) != 1 || len(result.Stages[0].Evidence) != 1+HardMaxRepairAttempts {
		t.Fatalf("evidence: %+v", result.Stages)
	}
}

func TestRunPipelineRepairThenBroaderPasses(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	runs := 0
	result := RunPipeline(t.Context(), dir, Options{
		Targets: []string{"internal/auth/auth.go"},
		Executor: func(_ context.Context, _ string, _ Command, _ int, _ time.Duration) Result {
			runs++
			if runs == 1 {
				return Result{Status: StatusFail, Failed: 1, Reason: "regression"}
			}
			return Result{OK: true, Status: StatusPass, Passed: 1}
		},
		Repair: func(context.Context, RepairRequest) error { return nil },
	})
	if result.Status != StatusPass || len(result.RepairAttempts) != 1 || runs != 3 {
		t.Fatalf("%+v runs=%d", result, runs)
	}
}

func TestRunPipelineRepairErrorStops(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	result := RunPipeline(t.Context(), dir, Options{
		Targets: []string{"x.go"},
		Executor: func(context.Context, string, Command, int, time.Duration) Result {
			return Result{Status: StatusFail, Failed: 1}
		},
		Repair: func(context.Context, RepairRequest) error { return errors.New("repair unavailable") },
	})
	if result.Status != StatusFail || len(result.RepairAttempts) != 1 || result.RepairAttempts[0].Reason != "repair unavailable" {
		t.Fatalf("%+v", result)
	}
}

func TestRunPipelineInconclusiveAndSkipped(t *testing.T) {
	inconclusive := RunPipeline(t.Context(), t.TempDir(), Options{})
	if inconclusive.Status != StatusInconclusive {
		t.Fatalf("%+v", inconclusive)
	}
	skipped := RunPipeline(t.Context(), t.TempDir(), Options{SkipReason: "not requested"})
	if skipped.Status != StatusSkipped || len(skipped.Stages) != 2 {
		t.Fatalf("%+v", skipped)
	}
}

func TestCountSummary(t *testing.T) {
	passed, failed := count("================ 18 passed, 2 failed in 2.30s ================")
	if passed != 18 || failed != 2 {
		t.Fatalf("passed=%d failed=%d", passed, failed)
	}
}

func TestFormatPipelineShowsStageEvidence(t *testing.T) {
	result := PipelineResult{
		Status:   StatusPass,
		Duration: 2300 * time.Millisecond,
		Stages: []StageResult{
			{Scope: ScopeTargeted, Status: StatusPass, Evidence: []Result{{Command: "go test ./internal/auth", Passed: 18, Duration: 2300 * time.Millisecond}}},
			{Scope: ScopeBroader, Status: StatusPass, Evidence: []Result{{Command: "go test ./...", Passed: 30, Duration: 4 * time.Second}}},
		},
	}
	out := FormatPipeline(result)
	for _, want := range []string{"verify PASS", "targeted PASS", "go test ./internal/auth", "passed=18", "broader PASS", "go test ./..."} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func writeVerifyFile(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
