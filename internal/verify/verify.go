package verify

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Result is a structured test run.
type Result struct {
	OK      bool   `json:"ok"`
	Runner  string `json:"runner"`
	Command string `json:"command"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Output  string `json:"output"`
	Reason  string `json:"reason,omitempty"`
}

// Detect picks a test command for the workspace.
func Detect(workspace string) (runner, command string, args []string) {
	if fileExists(filepath.Join(workspace, "go.mod")) {
		return "go", "go test ./...", []string{"test", "./..."}
	}
	if fileExists(filepath.Join(workspace, "package.json")) {
		return "npm", "npm test --silent", []string{"test", "--silent"}
	}
	if fileExists(filepath.Join(workspace, "pytest.ini")) || fileExists(filepath.Join(workspace, "pyproject.toml")) {
		return "pytest", "pytest -q", []string{"-q"}
	}
	return "", "", nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// Run executes the detected test command.
func Run(ctx context.Context, workspace string) Result {
	runner, display, args := Detect(workspace)
	if runner == "" {
		return Result{OK: false, Runner: "none", Reason: "no test runner found"}
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
	}

	var cmd *exec.Cmd
	switch runner {
	case "go":
		cmd = exec.CommandContext(ctx, "go", args...)
	case "npm":
		cmd = exec.CommandContext(ctx, "npm", args...)
	case "pytest":
		cmd = exec.CommandContext(ctx, "pytest", args...)
	default:
		return Result{OK: false, Reason: "unknown runner"}
	}
	cmd.Dir = workspace
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if len(out) > 8<<10 {
		out = out[:8<<10] + "\n… truncated …"
	}
	passed, failed := count(out)
	res := Result{
		OK:      err == nil,
		Runner:  runner,
		Command: display,
		Passed:  passed,
		Failed:  failed,
		Output:  strings.TrimSpace(out),
	}
	if err != nil {
		res.Reason = err.Error()
		if failed == 0 {
			res.Failed = 1
		}
	}
	return res
}

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
	return passed, failed
}

func Format(r Result) string {
	if r.Runner == "none" {
		return "verify INCONCLUSIVE (no test runner found)"
	}
	status := "PASS"
	if !r.OK {
		status = "FAIL"
	}
	return fmt.Sprintf("verify %s (%s)  passed=%d failed=%d\n%s", status, r.Command, r.Passed, r.Failed, r.Output)
}
