package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

func TestStatusFromEvidenceRequiresExactStatusToken(t *testing.T) {
	for _, tc := range []struct {
		evidence string
		want     Status
	}{
		{"verify PASS\ngo test ./...", StatusPass},
		{"VERIFY FAIL (targeted)", StatusFail},
		{"verify INCONCLUSIVE timeout", StatusInconclusive},
		{"verify SKIPPED no runner", StatusSkipped},
		{"verify PASSIVE", StatusInconclusive},
		{"tests passed", StatusInconclusive},
		{"", StatusInconclusive},
	} {
		if got := StatusFromEvidence(tc.evidence); got != tc.want {
			t.Fatalf("StatusFromEvidence(%q)=%s want %s", tc.evidence, got, tc.want)
		}
	}
}

func TestRunCommandZeroEvidenceIsInconclusive(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	writeVerifyFile(t, dir, "empty.go", "package empty\n")
	res := runCommand(t.Context(), dir, Command{Runner: "go", Display: "go test ./...", Args: []string{"test", "./..."}, Scope: ScopeBroader}, 1, time.Second)
	if res.Status != StatusInconclusive || res.OK {
		t.Fatalf("zero-evidence command = %+v", res)
	}
}

func TestRunCommandSanitizesEnvironment(t *testing.T) {
	t.Setenv("VERIFY_TEST_SECRET_TOKEN", "must-not-cross")
	t.Setenv("VERIFY_HELPER", "1")
	// Race-instrumented test binaries can take longer than one second to
	// start. This test checks environment isolation, not timeout handling.
	result := runCommand(t.Context(), t.TempDir(), Command{
		Runner:  os.Args[0],
		Display: "verify helper",
		Args:    []string{"-test.run=^TestVerifyHelperProcess$"},
	}, 1, 5*time.Second)
	if result.Status != StatusPass || result.Failed != 0 || !strings.Contains(result.Output, "ok verify/helper") {
		t.Fatalf("sanitized verifier result = %+v", result)
	}
	if strings.Contains(result.Output, "leaked") {
		t.Fatal("verifier child inherited a secret")
	}
}

func TestRunCommandBoundsOutputDuringExecution(t *testing.T) {
	t.Setenv("VERIFY_HELPER", "noisy")
	result := runCommand(t.Context(), t.TempDir(), Command{
		Runner:  os.Args[0],
		Display: "verify noisy helper",
		Args:    []string{"-test.run=^TestVerifyHelperProcess$"},
	}, 1, 5*time.Second)
	if result.Status != StatusPass || result.Failed != 0 || !strings.Contains(result.Output, "ok verify/noisy") {
		t.Fatalf("bounded verifier result = %+v", result)
	}
	if !result.OutputTruncated || len(result.Output) > MaxOutputBytes || !strings.Contains(result.Output, "truncated") {
		t.Fatalf("noisy verifier output = len %d truncated=%v", len(result.Output), result.OutputTruncated)
	}
}

func TestBoundedCaptureConcurrentWrites(t *testing.T) {
	var capture boundedCapture
	var writers sync.WaitGroup
	for i := 0; i < 8; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			_, _ = capture.Write([]byte(strings.Repeat("x", 4<<10)))
		}()
	}
	writers.Wait()
	out, truncated := capture.result()
	if !truncated || len(out) > MaxOutputBytes || !strings.Contains(out, "truncated") {
		t.Fatalf("concurrent capture = len %d truncated=%v", len(out), truncated)
	}
}

func TestVerifyHelperProcess(t *testing.T) {
	if os.Getenv("VERIFY_HELPER") == "" {
		return
	}
	if os.Getenv("VERIFY_HELPER") == "noisy" {
		fmt.Fprintln(os.Stdout, "ok verify/noisy")
		_, _ = os.Stdout.Write([]byte(strings.Repeat("x", MaxOutputBytes*4)))
		os.Exit(0)
	}
	if os.Getenv("VERIFY_TEST_SECRET_TOKEN") != "" {
		fmt.Fprintln(os.Stdout, "leaked")
	} else {
		fmt.Fprintln(os.Stdout, "ok verify/helper")
	}
	os.Exit(0)
}

func TestBoundedOutputMarksTruncation(t *testing.T) {
	out, truncated := boundedOutput(strings.Repeat("x", MaxOutputBytes+100))
	if !truncated || len(out) > MaxOutputBytes || !strings.Contains(out, "truncated") {
		t.Fatalf("bounded output = len %d truncated=%v", len(out), truncated)
	}
}

func TestCollectProvenanceExactHeadAndTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	verifyGitRun(t, dir, "init", "--quiet")
	verifyGitRun(t, dir, "config", "user.name", "Picogent Test")
	verifyGitRun(t, dir, "config", "user.email", "picogent@example.test")
	writeVerifyFile(t, dir, "go.mod", "module example.test/manifest\n\ngo 1.25\n")
	verifyGitRun(t, dir, "add", "go.mod")
	verifyGitRun(t, dir, "commit", "--quiet", "-m", "initial")
	head := strings.TrimSpace(verifyGitRun(t, dir, "rev-parse", "--verify", "HEAD^{commit}"))

	clean := CollectProvenance(t.Context(), dir, head)
	if clean.Match != ManifestPass || clean.Tree != "CLEAN" || clean.SHA != head || clean.GitRoot == "" {
		t.Fatalf("clean provenance = %+v", clean)
	}
	writeVerifyFile(t, dir, "go.mod", "module example.test/manifest\n\ngo 1.25\n\n// dirty\n")
	dirty := CollectProvenance(t.Context(), dir, head)
	if dirty.Match != ManifestPass || dirty.Tree != "DIRTY" {
		t.Fatalf("dirty provenance = %+v", dirty)
	}
	wrong := CollectProvenance(t.Context(), dir, strings.Repeat("b", 40))
	if wrong.Match != ManifestFail || wrong.Tree != "DIRTY" {
		t.Fatalf("mismatched provenance = %+v", wrong)
	}
	missing := CollectProvenance(t.Context(), dir, "")
	if missing.Match != ManifestUnverified {
		t.Fatalf("missing expected SHA = %+v", missing)
	}

	nonGit := CollectProvenance(t.Context(), t.TempDir(), head)
	if nonGit.Match != ManifestUnverified || nonGit.Tree != "UNVERIFIED" {
		t.Fatalf("non-git provenance = %+v", nonGit)
	}
}

func TestCollectProvenanceDisablesRepositoryFsmonitor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper script")
	}
	dir := t.TempDir()
	verifyGitRun(t, dir, "init", "--quiet")
	verifyGitRun(t, dir, "config", "user.name", "Picogent Test")
	verifyGitRun(t, dir, "config", "user.email", "picogent@example.test")
	writeVerifyFile(t, dir, "go.mod", "module example.test/manifest\n\ngo 1.25\n")
	verifyGitRun(t, dir, "add", "go.mod")
	verifyGitRun(t, dir, "commit", "--quiet", "-m", "initial")
	wantHead := strings.TrimSpace(verifyGitRun(t, dir, "rev-parse", "--verify", "HEAD^{commit}"))

	marker := filepath.Join(t.TempDir(), "fsmonitor-ran")
	hook := filepath.Join(t.TempDir(), "untrusted-fsmonitor")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\n: > \"$PICOGENT_TEST_VERIFY_MARKER\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_TEST_VERIFY_MARKER", marker)
	verifyGitRun(t, dir, "config", "core.fsmonitor", hook)
	// Also try the inherited config injection path. The Git boundary must
	// remove these variables before launching the repository command.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.fsmonitor")
	t.Setenv("GIT_CONFIG_VALUE_0", hook)

	evidence := CollectProvenance(t.Context(), dir, wantHead)
	if evidence.Match != ManifestPass {
		t.Fatalf("provenance = %+v", evidence)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repository fsmonitor hook ran: %v", err)
	}
}

func TestManifestPassRequiresCoverageEvidence(t *testing.T) {
	pipeline := PipelineResult{
		Status: StatusPass,
		Stages: []StageResult{{
			Scope:  ScopeBroader,
			Status: StatusPass,
			Evidence: []Result{{
				Scope: ScopeBroader, Runner: "go", Command: "go test ./...", Status: StatusPass, Passed: 1,
			}},
		}},
	}
	manifest := ManifestFromPipeline(pipeline, HeadEvidence{
		SHA:         strings.Repeat("a", 40),
		ExpectedSHA: strings.Repeat("a", 40),
		Match:       ManifestPass,
		Tree:        "CLEAN",
	})
	if manifest.Status != ManifestUnverified || !strings.Contains(manifest.Reason, "coverage") {
		t.Fatalf("manifest = %+v", manifest)
	}
	if len(manifest.Checks) != 1 || manifest.Checks[0].Coverage.Status != ManifestUnverified {
		t.Fatalf("coverage evidence = %+v", manifest.Checks)
	}
}

func TestManifestPropagatesPipelineStatuses(t *testing.T) {
	for _, tc := range []struct {
		pipeline Status
		manifest ManifestStatus
	}{
		{StatusFail, ManifestFail},
		{StatusInconclusive, ManifestInconclusive},
		{StatusSkipped, ManifestSkipped},
	} {
		got := ManifestFromPipeline(PipelineResult{Status: tc.pipeline, Reason: "recorded reason"}, HeadEvidence{})
		if got.Status != tc.manifest || got.Reason != "recorded reason" {
			t.Fatalf("pipeline %s -> %+v", tc.pipeline, got)
		}
	}
}

func TestManifestTruncatedOutputIsUnverified(t *testing.T) {
	manifest := ManifestFromPipeline(PipelineResult{
		Status: StatusPass,
		Stages: []StageResult{{Scope: ScopeBroader, Status: StatusPass, Evidence: []Result{{
			Scope: ScopeBroader, Runner: "go", Command: "go test ./...", Status: StatusPass, Passed: 1, OutputTruncated: true,
		}}}},
	}, HeadEvidence{
		SHA: strings.Repeat("a", 40), ExpectedSHA: strings.Repeat("a", 40), Match: ManifestPass, Tree: "CLEAN",
	})
	if manifest.Status != ManifestUnverified || !strings.Contains(manifest.Reason, "truncated") || !manifest.Checks[0].OutputTruncated {
		t.Fatalf("truncated manifest = %+v", manifest)
	}
}

func TestManifestIsBoundedAndOmitsRawOutput(t *testing.T) {
	secret := "do-not-persist-this-command-output"
	evidence := make([]Result, 0, 100)
	for i := 0; i < 100; i++ {
		evidence = append(evidence, Result{Scope: ScopeBroader, Runner: "go", Command: strings.Repeat("command-", 100), Status: StatusPass, Passed: 1, Output: secret})
	}
	manifest := ManifestFromPipeline(PipelineResult{
		Status: StatusPass,
		Stages: []StageResult{{Scope: ScopeBroader, Status: StatusPass, Evidence: evidence}},
	}, HeadEvidence{GitRoot: strings.Repeat("/workspace", 100), SHA: strings.Repeat("a", 40), Match: ManifestUnverified, Tree: "UNVERIFIED"})
	var output bytes.Buffer
	if err := WriteJSON(&output, manifest); err != nil {
		t.Fatal(err)
	}
	trimmed := bytes.TrimSpace(output.Bytes())
	if len(trimmed) > MaxManifestBytes || !json.Valid(trimmed) {
		t.Fatalf("manifest output = %d bytes", len(trimmed))
	}
	if bytes.Contains(trimmed, []byte(secret)) || !strings.Contains(string(trimmed), `"checks_truncated": true`) {
		t.Fatalf("raw output or truncation evidence missing: %s", trimmed)
	}
}

func TestRunPipelineRejectsUncoveredTargets(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "Cargo.toml", "[package]\nname = \"fixture\"\nversion = \"0.1.0\"\n")
	result := RunPipeline(t.Context(), dir, Options{Targets: []string{"src/main.rs"}, Executor: func(context.Context, string, Command, int, time.Duration) Result {
		t.Fatal("broader verification must not run when requested targets have no safe targeted command")
		return Result{}
	}})
	if result.Status != StatusInconclusive || len(result.Stages) != 1 {
		t.Fatalf("uncovered targets = %+v", result)
	}
}

func TestRunPipelineWithoutTargetsRunsBroaderSuite(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	var commands []string
	result := RunPipeline(t.Context(), dir, Options{
		Executor: func(_ context.Context, _ string, command Command, _ int, _ time.Duration) Result {
			commands = append(commands, command.Display)
			return Result{OK: true, Status: StatusPass, Passed: 1}
		},
	})
	if result.Status != StatusPass || len(result.Stages) != 2 {
		t.Fatalf("no-target result = %+v", result)
	}
	if result.Stages[0].Status != StatusSkipped || result.Stages[1].Status != StatusPass {
		t.Fatalf("no-target stages = %+v", result.Stages)
	}
	if strings.Join(commands, "|") != "go test ./..." {
		t.Fatalf("no-target commands = %v", commands)
	}
}

func TestRunPipelineCancellationCannotReportPass(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	result := RunPipeline(ctx, dir, Options{
		Executor: func(_ context.Context, _ string, _ Command, _ int, _ time.Duration) Result {
			cancel()
			return Result{OK: true, Status: StatusPass, Passed: 1}
		},
	})
	if result.Status != StatusInconclusive || !strings.Contains(result.Reason, "canceled") {
		t.Fatalf("canceled pipeline = %+v", result)
	}
	if len(result.Stages) != 2 || result.Stages[1].Status != StatusInconclusive {
		t.Fatalf("canceled stages = %+v", result.Stages)
	}
	if len(result.Stages[1].Evidence) != 1 || result.Stages[1].Evidence[0].Status != StatusInconclusive {
		t.Fatalf("canceled evidence = %+v", result.Stages[1].Evidence)
	}
}

func TestRunPipelineCancellationSkipsRepairCallback(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	repairs := 0

	result := RunPipeline(ctx, dir, Options{
		Targets: []string{"internal/auth/auth.go"},
		Executor: func(_ context.Context, _ string, _ Command, _ int, _ time.Duration) Result {
			cancel()
			return Result{Status: StatusFail, Failed: 1, Reason: "regression"}
		},
		Repair: func(context.Context, RepairRequest) error {
			repairs++
			return nil
		},
	})
	if result.Status != StatusInconclusive || repairs != 0 {
		t.Fatalf("canceled repair path = %+v repairs=%d", result, repairs)
	}
}

func TestRunPipelineCancellationDuringRepairSkipsRecheck(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runs := 0

	result := RunPipeline(ctx, dir, Options{
		Targets: []string{"internal/auth/auth.go"},
		Executor: func(_ context.Context, _ string, _ Command, _ int, _ time.Duration) Result {
			runs++
			return Result{Status: StatusFail, Failed: 1, Reason: "regression"}
		},
		Repair: func(context.Context, RepairRequest) error {
			cancel()
			return nil
		},
	})
	if result.Status != StatusInconclusive || runs != 1 || len(result.RepairAttempts) != 1 {
		t.Fatalf("canceled repair recheck = %+v runs=%d", result, runs)
	}
	if result.RepairAttempts[0].Status != StatusInconclusive || !strings.Contains(result.RepairAttempts[0].Reason, "canceled") {
		t.Fatalf("canceled repair attempt = %+v", result.RepairAttempts[0])
	}
}

func TestDetectPlanIgnoresNonGoFileTargets(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	writeVerifyFile(t, dir, "README.md", "not Go source\n")
	plan := DetectPlan(dir, []string{"README.md"})
	if len(plan.Targeted) != 0 {
		t.Fatalf("non-Go target became targeted command: %+v", plan.Targeted)
	}
}

func TestDetectPlanKeepsDottedDirectoryTargets(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module x\n")
	writeVerifyFile(t, dir, filepath.Join("internal", "v1.2", "feature.go"), "package feature\n")
	plan := DetectPlan(dir, []string{filepath.Join("internal", "v1.2")})
	if len(plan.Targeted) != 1 || strings.Join(plan.Targeted[0].Args, " ") != "test ./internal/v1.2" {
		t.Fatalf("dotted directory target = %+v", plan.Targeted)
	}
}

func TestNormalizeResultRejectsContradictoryPass(t *testing.T) {
	result := normalizeResult(Result{OK: true, Status: StatusPass, Passed: 1, Failed: 1}, Command{Runner: "go", Display: "go test ./..."}, 1)
	if result.Status != StatusFail || result.OK || result.Reason == "" {
		t.Fatalf("contradictory result = %+v", result)
	}
}

func TestNormalizeResultRejectsTruncatedPass(t *testing.T) {
	result := normalizeResult(Result{OK: true, Status: StatusPass, Passed: 1, OutputTruncated: true}, Command{Runner: "go", Display: "go test ./..."}, 1)
	if result.Status != StatusInconclusive || result.OK || result.Reason != "verification output was truncated" {
		t.Fatalf("truncated result = %+v", result)
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

func verifyGitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}
