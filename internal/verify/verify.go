package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Status classifies verification without confusing missing evidence with success.
type Status string

const (
	StatusPass         Status = "PASS"
	StatusFail         Status = "FAIL"
	StatusInconclusive Status = "INCONCLUSIVE"
	StatusSkipped      Status = "SKIPPED"

	// Short aliases match the external status vocabulary.
	PASS         = StatusPass
	FAIL         = StatusFail
	INCONCLUSIVE = StatusInconclusive
	SKIPPED      = StatusSkipped
)

// Scope identifies the verification stage that produced evidence.
type Scope string

const (
	ScopeTargeted Scope = "targeted"
	ScopeBroader  Scope = "broader"
)

// Result is a structured test run. Existing fields remain source-compatible.
type Result struct {
	OK       bool          `json:"ok"`
	Status   Status        `json:"status"`
	Scope    Scope         `json:"scope,omitempty"`
	Runner   string        `json:"runner"`
	Command  string        `json:"command"`
	Passed   int           `json:"passed"`
	Failed   int           `json:"failed"`
	Output   string        `json:"output"`
	Reason   string        `json:"reason,omitempty"`
	Duration time.Duration `json:"duration"`
	Attempt  int           `json:"attempt,omitempty"`
}

// Detect picks the broad test command for the workspace.
func Detect(workspace string) (runner, command string, args []string) {
	if fileExists(filepath.Join(workspace, "go.mod")) {
		return "go", "go test ./...", []string{"test", "./..."}
	}
	if fileExists(filepath.Join(workspace, "package.json")) {
		switch {
		case fileExists(filepath.Join(workspace, "pnpm-lock.yaml")):
			return "pnpm", "pnpm test --silent", []string{"test", "--silent"}
		case fileExists(filepath.Join(workspace, "yarn.lock")):
			return "yarn", "yarn test --silent", []string{"test", "--silent"}
		case fileExists(filepath.Join(workspace, "bun.lock")) || fileExists(filepath.Join(workspace, "bun.lockb")):
			return "bun", "bun test", []string{"test"}
		default:
			return "npm", "npm test --silent", []string{"test", "--silent"}
		}
	}
	if fileExists(filepath.Join(workspace, "pytest.ini")) || fileExists(filepath.Join(workspace, "pyproject.toml")) {
		return "pytest", "pytest -q", []string{"-q"}
	}
	if fileExists(filepath.Join(workspace, "Cargo.toml")) {
		return "cargo", "cargo test", []string{"test"}
	}
	return "", "", nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// Run executes the detected broad test command.
func Run(ctx context.Context, workspace string) Result {
	runner, display, args := Detect(workspace)
	if runner == "" {
		return Result{OK: false, Status: StatusInconclusive, Runner: "none", Reason: "no test runner found"}
	}
	return runCommand(ctx, workspace, Command{Runner: runner, Display: display, Args: args, Scope: ScopeBroader}, 1, 90*time.Second)
}

func runCommand(ctx context.Context, workspace string, command Command, attempt int, timeout time.Duration) Result {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, command.Runner, command.Args...)
	cmd.Dir = workspace
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	started := time.Now()
	err := cmd.Run()
	duration := time.Since(started)
	out := buf.String()
	if len(out) > 8<<10 {
		out = out[:8<<10] + "\n… truncated …"
	}
	passed, failed := count(out)
	res := Result{
		OK:       err == nil,
		Status:   StatusPass,
		Scope:    command.Scope,
		Runner:   command.Runner,
		Command:  command.Display,
		Passed:   passed,
		Failed:   failed,
		Output:   strings.TrimSpace(out),
		Duration: duration,
		Attempt:  attempt,
	}
	if err == nil {
		if failed > 0 {
			res.OK = false
			res.Status = StatusFail
			res.Reason = "verification reported failed tests"
			return res
		}
		if passed == 0 && failed == 0 {
			res.OK = false
			res.Status = StatusInconclusive
			res.Reason = "runner exited successfully without test evidence"
		}
		return res
	}
	res.OK = false
	res.Reason = err.Error()
	res.Status = StatusFail
	var execErr *exec.Error
	if errors.As(err, &execErr) || errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.Status = StatusInconclusive
	}
	if res.Status == StatusFail && failed == 0 {
		res.Failed = 1
	}
	return res
}

var summaryCount = regexp.MustCompile(`(?i)(\d+)\s+(passed|failed)\b`)

func count(out string) (passed, failed int) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ok ") || strings.Contains(line, " PASS") {
			passed++
		}
		if strings.HasPrefix(line, "FAIL") || strings.Contains(line, "--- FAIL:") || strings.Contains(line, " FAIL ") {
			failed++
		}
	}
	for _, match := range summaryCount.FindAllStringSubmatch(out, -1) {
		n, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		if strings.EqualFold(match[2], "passed") && n > passed {
			passed = n
		}
		if strings.EqualFold(match[2], "failed") && n > failed {
			failed = n
		}
	}
	return passed, failed
}

// Format renders concise evidence for humans and agents.
func Format(r Result) string {
	status := r.Status
	if status == "" {
		switch {
		case r.Runner == "none":
			status = StatusInconclusive
		case r.OK && r.Passed > 0:
			status = StatusPass
		case r.OK:
			status = StatusInconclusive
		default:
			status = StatusFail
		}
	}
	if r.Runner == "none" {
		return "verify INCONCLUSIVE (no test runner found)"
	}
	duration := ""
	if r.Duration > 0 {
		duration = " duration=" + r.Duration.Round(time.Millisecond).String()
	}
	line := fmt.Sprintf("verify %s (%s)  passed=%d failed=%d%s", status, r.Command, r.Passed, r.Failed, duration)
	if r.Output == "" {
		return line
	}
	return line + "\n" + r.Output
}
