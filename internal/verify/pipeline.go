package verify

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DefaultMaxRepairAttempts = 3
	HardMaxRepairAttempts    = 3
)

// Command is one executable verification step.
type Command struct {
	Runner  string   `json:"runner"`
	Display string   `json:"display"`
	Args    []string `json:"args,omitempty"`
	Scope   Scope    `json:"scope"`
}

// Plan describes detected targeted and broader verification without running it.
type Plan struct {
	Runner   string    `json:"runner,omitempty"`
	Targeted []Command `json:"targeted,omitempty"`
	Broader  []Command `json:"broader,omitempty"`
	Reason   string    `json:"reason,omitempty"`
}

// RepairRequest gives a caller bounded failure evidence before a repair.
type RepairRequest struct {
	Number      int    `json:"number"`
	MaxAttempts int    `json:"max_attempts"`
	Failure     Result `json:"failure"`
}

// RepairFunc performs one caller-owned repair. Verification always enforces the cap.
type RepairFunc func(context.Context, RepairRequest) error

// Executor lets tests and integrations run commands in a controlled environment.
type Executor func(context.Context, string, Command, int, time.Duration) Result

// RepairAttempt records the trigger and verification after one repair.
type RepairAttempt struct {
	Number   int           `json:"number"`
	Failure  Result        `json:"failure"`
	Result   Result        `json:"result"`
	Status   Status        `json:"status"`
	Reason   string        `json:"reason,omitempty"`
	Duration time.Duration `json:"duration"`
}

// StageResult records every command and output for one pipeline stage.
type StageResult struct {
	Scope    Scope    `json:"scope"`
	Status   Status   `json:"status"`
	Evidence []Result `json:"evidence,omitempty"`
	Reason   string   `json:"reason,omitempty"`
}

// PipelineResult is the complete detect -> targeted -> broader classification.
type PipelineResult struct {
	Status            Status          `json:"status"`
	Plan              Plan            `json:"plan"`
	Stages            []StageResult   `json:"stages"`
	RepairAttempts    []RepairAttempt `json:"repair_attempts,omitempty"`
	MaxRepairAttempts int             `json:"max_repair_attempts"`
	Duration          time.Duration   `json:"duration"`
	Reason            string          `json:"reason,omitempty"`
}

// Options configures targeted inputs, bounded repair, and execution.
type Options struct {
	Targets           []string
	MaxRepairAttempts int
	Repair            RepairFunc
	Executor          Executor
	Timeout           time.Duration
	SkipReason        string
}

// DetectPlan derives safe targeted commands and one broader command.
func DetectPlan(workspace string, targets []string) Plan {
	runner, display, args := Detect(workspace)
	if runner == "" {
		return Plan{Reason: "no test runner found"}
	}
	plan := Plan{
		Runner:  runner,
		Broader: []Command{{Runner: runner, Display: display, Args: append([]string(nil), args...), Scope: ScopeBroader}},
	}
	targets = normalizeTargets(workspace, targets)
	if len(targets) == 0 {
		return plan
	}
	var targeted Command
	switch runner {
	case "go":
		packages := goTargets(targets)
		if len(packages) > 0 {
			targeted = Command{Runner: "go", Args: append([]string{"test"}, packages...), Scope: ScopeTargeted}
		}
	case "npm", "pnpm", "yarn":
		targeted = Command{Runner: runner, Args: append([]string{"test", "--silent", "--"}, targets...), Scope: ScopeTargeted}
	case "bun":
		targeted = Command{Runner: runner, Args: append([]string{"test"}, targets...), Scope: ScopeTargeted}
	case "pytest":
		targeted = Command{Runner: runner, Args: append([]string{"-q"}, targets...), Scope: ScopeTargeted}
	}
	if targeted.Runner != "" {
		targeted.Display = displayCommand(targeted.Runner, targeted.Args)
		plan.Targeted = []Command{targeted}
	}
	return plan
}

// DetectPipeline is an alias for DetectPlan.
func DetectPipeline(workspace string, targets []string) Plan { return DetectPlan(workspace, targets) }

// RunPipeline performs detected targeted verification, then broader verification.
func RunPipeline(ctx context.Context, workspace string, options Options) PipelineResult {
	started := time.Now()
	maxRepairs := options.MaxRepairAttempts
	if maxRepairs <= 0 {
		maxRepairs = DefaultMaxRepairAttempts
	}
	if maxRepairs > HardMaxRepairAttempts {
		maxRepairs = HardMaxRepairAttempts
	}
	result := PipelineResult{MaxRepairAttempts: maxRepairs}
	if strings.TrimSpace(options.SkipReason) != "" {
		result.Status = StatusSkipped
		result.Reason = strings.TrimSpace(options.SkipReason)
		result.Stages = []StageResult{{Scope: ScopeTargeted, Status: StatusSkipped, Reason: result.Reason}, {Scope: ScopeBroader, Status: StatusSkipped, Reason: result.Reason}}
		result.Duration = time.Since(started)
		return result
	}

	result.Plan = DetectPlan(workspace, options.Targets)
	if result.Plan.Runner == "" {
		result.Status = StatusInconclusive
		result.Reason = result.Plan.Reason
		result.Duration = time.Since(started)
		return result
	}
	execute := options.Executor
	if execute == nil {
		execute = runCommand
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	verificationAttempt := 0

	runStage := func(scope Scope, commands []Command) StageResult {
		stage := StageResult{Scope: scope}
		if len(commands) == 0 {
			stage.Status = StatusSkipped
			stage.Reason = "no safe targeted command detected"
			return stage
		}
		stage.Status = StatusPass
		for _, command := range commands {
			verificationAttempt++
			current := normalizeResult(execute(ctx, workspace, command, verificationAttempt, timeout), command, verificationAttempt)
			stage.Evidence = append(stage.Evidence, current)
			for current.Status == StatusFail && options.Repair != nil && len(result.RepairAttempts) < maxRepairs {
				number := len(result.RepairAttempts) + 1
				attempt := RepairAttempt{Number: number, Failure: current}
				repairStarted := time.Now()
				err := options.Repair(ctx, RepairRequest{Number: number, MaxAttempts: maxRepairs, Failure: current})
				attempt.Duration = time.Since(repairStarted)
				if err != nil {
					attempt.Status = StatusFail
					attempt.Reason = err.Error()
					result.RepairAttempts = append(result.RepairAttempts, attempt)
					break
				}
				verificationAttempt++
				current = normalizeResult(execute(ctx, workspace, command, verificationAttempt, timeout), command, verificationAttempt)
				attempt.Result = current
				attempt.Status = current.Status
				result.RepairAttempts = append(result.RepairAttempts, attempt)
				stage.Evidence = append(stage.Evidence, current)
			}
			if current.Status != StatusPass {
				stage.Status = current.Status
				stage.Reason = current.Reason
				return stage
			}
		}
		return stage
	}

	targeted := runStage(ScopeTargeted, result.Plan.Targeted)
	result.Stages = append(result.Stages, targeted)
	if targeted.Status == StatusFail || targeted.Status == StatusInconclusive {
		result.Status = targeted.Status
		result.Reason = targeted.Reason
		result.Duration = time.Since(started)
		return result
	}
	broader := runStage(ScopeBroader, result.Plan.Broader)
	result.Stages = append(result.Stages, broader)
	result.Status = broader.Status
	result.Reason = broader.Reason
	result.Duration = time.Since(started)
	return result
}

// FormatPipeline renders stage-by-stage evidence without hiding skipped or
// inconclusive checks behind a boolean success value.
func FormatPipeline(result PipelineResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "verify %s duration=%s", result.Status, result.Duration.Round(time.Millisecond))
	if result.Reason != "" {
		b.WriteString(" reason=")
		b.WriteString(result.Reason)
	}
	for _, stage := range result.Stages {
		fmt.Fprintf(&b, "\n%s %s", stage.Scope, stage.Status)
		if stage.Reason != "" {
			b.WriteString(" — ")
			b.WriteString(stage.Reason)
		}
		for _, evidence := range stage.Evidence {
			fmt.Fprintf(&b, "\n  %s  passed=%d failed=%d duration=%s", evidence.Command, evidence.Passed, evidence.Failed, evidence.Duration.Round(time.Millisecond))
			if evidence.Output != "" {
				b.WriteString("\n  ")
				b.WriteString(strings.ReplaceAll(evidence.Output, "\n", "\n  "))
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func normalizeResult(result Result, command Command, attempt int) Result {
	if result.Runner == "" {
		result.Runner = command.Runner
	}
	if result.Command == "" {
		result.Command = command.Display
	}
	if result.Scope == "" {
		result.Scope = command.Scope
	}
	if result.Attempt == 0 {
		result.Attempt = attempt
	}
	if result.Status == "" {
		if result.OK {
			result.Status = StatusPass
		} else {
			result.Status = StatusFail
		}
	}
	result.OK = result.Status == StatusPass
	return result
}

func normalizeTargets(workspace string, targets []string) []string {
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if filepath.IsAbs(target) {
			rel, err := filepath.Rel(workspace, target)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			target = rel
		}
		target = filepath.ToSlash(filepath.Clean(target))
		if target == "." || strings.HasPrefix(target, "../") {
			continue
		}
		out = append(out, target)
	}
	sort.Strings(out)
	return unique(out)
}

func goTargets(targets []string) []string {
	packages := make([]string, 0, len(targets))
	for _, target := range targets {
		dir := target
		if filepath.Ext(target) != "" {
			dir = filepath.ToSlash(filepath.Dir(target))
		}
		if dir == "." {
			packages = append(packages, ".")
		} else {
			packages = append(packages, "./"+strings.TrimPrefix(dir, "./"))
		}
	}
	sort.Strings(packages)
	return unique(packages)
}

func unique(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func displayCommand(runner string, args []string) string {
	parts := append([]string{runner}, args...)
	for i, part := range parts {
		if strings.ContainsAny(part, " \t\n\"'") {
			parts[i] = `"` + strings.ReplaceAll(part, `"`, `\"`) + `"`
		}
	}
	return strings.Join(parts, " ")
}
