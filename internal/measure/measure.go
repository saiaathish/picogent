// Package measure provides bounded, provider-independent project measurements.
// It deliberately accepts no model-supplied command: measurement is a typed
// producer, not a second shell escape hatch.
package measure

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/saiaathish/picogent/internal/procenv"
)

const (
	// MaxOutputBytes bounds benchmark output while the command is running.
	// The raw output is never stored in task state; only the parsed benchmark
	// lines are returned to the current model turn.
	MaxOutputBytes = 64 << 10

	defaultTimeout = 90 * time.Second
	maxMetricLines = 32
	maxMetricLine  = 512
)

// Status classifies a measurement without treating an absent or incomplete
// benchmark as a pass.
type Status string

const (
	StatusPass         Status = "PASS"
	StatusFail         Status = "FAIL"
	StatusInconclusive Status = "INCONCLUSIVE"
)

// Command is a fixed executable measurement plan. Args are never populated
// from tool-call text.
type Command struct {
	Runner  string
	Display string
	Args    []string
}

// Result is the bounded projection of one measurement run. Metrics contain
// only canonical benchmark lines; arbitrary command output is discarded.
type Result struct {
	Status          Status
	Runner          string
	Command         string
	Benchmarks      int
	Metrics         []string
	OutputTruncated bool
	Reason          string
	Duration        time.Duration
}

// Detect returns the supported fixed measurement plan for a workspace.
// Go is intentionally the first producer slice; unsupported project types
// remain explicit INCONCLUSIVE rather than falling back to arbitrary scripts.
func Detect(workspace string) Command {
	if strings.TrimSpace(workspace) == "" {
		return Command{}
	}
	info, err := os.Stat(filepath.Join(workspace, "go.mod"))
	if err != nil || info.IsDir() {
		return Command{}
	}
	return Command{
		Runner:  "go",
		Display: "go test ./... -run '^$' -bench . -benchtime=100ms -count=1",
		Args:    []string{"test", "./...", "-run", "^$", "-bench", ".", "-benchtime=100ms", "-count=1"},
	}
}

// Run executes the fixed measurement plan with a sanitized environment and a
// bounded timeout. A missing runner or absent benchmark is not a success.
func Run(ctx context.Context, workspace string) Result {
	command := Detect(workspace)
	if command.Runner == "" {
		return Result{Status: StatusInconclusive, Runner: "none", Reason: "no supported benchmark runner found"}
	}
	return runCommand(ctx, workspace, command, defaultTimeout)
}

var metricPattern = regexp.MustCompile(`(?:ns/op|µs/op|us/op|ms/op|s/op|B/op|allocs/op)\b`)

func runCommand(ctx context.Context, workspace string, command Command, timeout time.Duration) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	commandCtx, stopOnOutputLimit := context.WithCancel(timeoutCtx)
	defer stopOnOutputLimit()

	cmd := exec.CommandContext(commandCtx, command.Runner, command.Args...)
	cmd.Dir = workspace
	cmd.Env = procenv.Sanitized()
	var output boundedOutput
	output.onLimit = stopOnOutputLimit
	cmd.Stdout = &output
	cmd.Stderr = &output
	started := time.Now()
	err := cmd.Run()
	duration := time.Since(started)
	raw, truncated := output.result()
	metrics := parseMetrics(raw)
	result := Result{
		Status:          StatusPass,
		Runner:          command.Runner,
		Command:         command.Display,
		Benchmarks:      len(metrics),
		Metrics:         metrics,
		OutputTruncated: truncated,
		Duration:        duration,
	}
	switch {
	case truncated:
		result.Status = StatusInconclusive
		result.Reason = "measurement output was truncated"
	case errors.Is(commandCtx.Err(), context.DeadlineExceeded), errors.Is(timeoutCtx.Err(), context.DeadlineExceeded):
		result.Status = StatusInconclusive
		result.Reason = "measurement timed out"
	case errors.Is(commandCtx.Err(), context.Canceled), errors.Is(timeoutCtx.Err(), context.Canceled):
		result.Status = StatusInconclusive
		result.Reason = "measurement was canceled"
	case err != nil:
		result.Status = StatusFail
		if _, ok := err.(*exec.Error); ok {
			result.Status = StatusInconclusive
			result.Reason = "benchmark runner was unavailable"
		} else {
			result.Reason = "benchmark command failed"
		}
	case len(metrics) == 0:
		result.Status = StatusInconclusive
		result.Reason = "no benchmark metrics were emitted"
	}
	result.Metrics = append([]string(nil), metrics...)
	return result
}

func parseMetrics(output string) []string {
	lines := strings.Split(output, "\n")
	metrics := make([]string, 0, minInt(len(lines), maxMetricLines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Benchmark") || !metricPattern.MatchString(line) {
			continue
		}
		line = strings.Join(strings.Fields(line), " ")
		if len(line) > maxMetricLine {
			line = line[:maxMetricLine] + "…"
		}
		metrics = append(metrics, line)
		if len(metrics) == maxMetricLines {
			break
		}
	}
	return metrics
}

// Format returns only bounded, canonical measurement evidence. It never
// includes arbitrary benchmark stdout/stderr, which keeps tool output from
// becoming an unbounded or credential-shaped durable evidence channel.
func Format(result Result) string {
	status := result.Status
	if status != StatusPass && status != StatusFail && status != StatusInconclusive {
		status = StatusInconclusive
	}
	command := strings.TrimSpace(result.Command)
	if command == "" {
		command = "no command"
	}
	line := fmt.Sprintf("measure %s (%s) benchmarks=%d", status, command, result.Benchmarks)
	if result.Duration > 0 {
		line += " duration=" + result.Duration.Round(time.Millisecond).String()
	}
	if result.Reason != "" {
		line += " reason=" + compactReason(result.Reason)
	}
	if len(result.Metrics) == 0 {
		return line
	}
	var b strings.Builder
	b.WriteString(line)
	for _, metric := range result.Metrics {
		metric = strings.TrimSpace(metric)
		if metric == "" {
			continue
		}
		b.WriteByte('\n')
		b.WriteString(metric)
	}
	return b.String()
}

// StatusFromEvidence recognizes only the canonical header emitted by Format.
// A lookalike sentence or a PASS without at least one parsed benchmark is
// inconclusive.
func StatusFromEvidence(evidence string) Status {
	evidence = strings.TrimSpace(evidence)
	upper := strings.ToUpper(evidence)
	if !strings.HasPrefix(upper, "MEASURE ") {
		return StatusInconclusive
	}
	rest := strings.TrimSpace(strings.TrimPrefix(upper, "MEASURE "))
	var status Status
	for _, candidate := range []Status{StatusPass, StatusFail, StatusInconclusive} {
		if rest == string(candidate) || strings.HasPrefix(rest, string(candidate)+" ") {
			status = candidate
			break
		}
	}
	if status == "" {
		return StatusInconclusive
	}
	if status != StatusPass {
		return status
	}
	marker := ") BENCHMARKS="
	index := strings.LastIndex(rest, marker)
	if index < 0 {
		return StatusInconclusive
	}
	value := rest[index+len(marker):]
	end := 0
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	if end == 0 || value[:end] == "0" || len(parseMetrics(evidence)) == 0 {
		return StatusInconclusive
	}
	return StatusPass
}

func compactReason(reason string) string {
	reason = strings.Join(strings.Fields(reason), " ")
	if len(reason) > 240 {
		return reason[:240] + "…"
	}
	return reason
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

type boundedOutput struct {
	mu        sync.Mutex
	data      bytes.Buffer
	truncated bool
	onLimit   context.CancelFunc
	limitOnce sync.Once
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	remaining := MaxOutputBytes - b.data.Len()
	if remaining <= 0 {
		b.truncated = true
		b.mu.Unlock()
		b.stopOnLimit()
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.data.Write(p[:remaining])
		b.truncated = true
		b.mu.Unlock()
		b.stopOnLimit()
		return len(p), nil
	}
	_, _ = b.data.Write(p)
	b.mu.Unlock()
	return len(p), nil
}

func (b *boundedOutput) stopOnLimit() {
	b.limitOnce.Do(func() {
		if b.onLimit != nil {
			b.onLimit()
		}
	})
}

func (b *boundedOutput) result() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String(), b.truncated
}
