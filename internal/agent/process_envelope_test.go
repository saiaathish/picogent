package agent

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	processEnvelopeHelperEnv  = "PICOGENT_PROCESS_ENVELOPE_HELPER"
	processEnvelopeReadyEnv   = "PICOGENT_PROCESS_ENVELOPE_READY"
	processEnvelopeReleaseEnv = "PICOGENT_PROCESS_ENVELOPE_RELEASE"
	processEnvelopeTurns      = longHorizonTurns
	processEnvelopeTimeout    = 45 * time.Second
)

// TestLongHorizonProcessEnvelope measures the resident set of a real child
// test process while it performs the existing durable 96-turn workload. Each
// sample follows a save/reload boundary, so the result is a sustained process
// envelope rather than a single startup snapshot or a heap-only statistic.
//
// The samples are diagnostic evidence only. Runner-specific measurements remain
// useful for spotting changes, but this test intentionally sets no product RSS
// budget or release threshold.
func TestLongHorizonProcessEnvelope(t *testing.T) {
	if os.Getenv(processEnvelopeHelperEnv) == "1" {
		longHorizonProcessEnvelopeHelper(t)
		return
	}

	root := t.TempDir()
	readyPath := filepath.Join(root, "ready")
	releasePath := filepath.Join(root, "release")
	ctx, cancel := context.WithTimeout(context.Background(), processEnvelopeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run", "^TestLongHorizonProcessEnvelope$", "-test.count=1")
	cmd.Env = processEnvelopeChildEnv(root, readyPath, releasePath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr boundedProcessOutput
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	scanner := bufio.NewScanner(stdout)
	if err := readEnvelopeReady(scanner); err != nil {
		t.Fatalf("child did not become ready: %v", withProcessStderr(err, stderr.String()))
	}
	first, err := processResidentBytes(cmd.Process.Pid)
	if err != nil {
		t.Skipf("resident-set sampling unavailable on %s: %v", runtime.GOOS, err)
	}
	if err := os.WriteFile(releasePath, []byte("release\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	samples := []int64{first}
	progress := 0
	done := false
	unavailable := 0
	started := time.Now()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if done {
			continue
		}
		if strings.HasPrefix(line, "PICOGENT_PROCESS_ENVELOPE_PROGRESS ") {
			turn, err := strconv.Atoi(strings.TrimPrefix(line, "PICOGENT_PROCESS_ENVELOPE_PROGRESS "))
			if err != nil {
				t.Fatalf("invalid child progress %q: %v", line, err)
			}
			if turn != progress+1 {
				t.Fatalf("child progress = %d after %d checkpoints", turn, progress)
			}
			progress = turn
			if resident, err := processResidentBytes(cmd.Process.Pid); err == nil {
				samples = append(samples, resident)
			} else {
				unavailable++
			}
			continue
		}
		if strings.HasPrefix(line, "PICOGENT_PROCESS_ENVELOPE_DONE ") {
			done = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read child output: %v\n%s", err, stderr.String())
	}
	if !done {
		t.Fatalf("child did not emit completion marker\n%s", stderr.String())
	}
	if progress != processEnvelopeTurns {
		t.Fatalf("child checkpoints = %d, want %d\n%s", progress, processEnvelopeTurns, stderr.String())
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("process envelope child failed: %v\n%s", err, stderr.String())
	}
	if len(samples) < 2 {
		t.Fatalf("resident samples = %d, want at least 2", len(samples))
	}

	minimum, maximum := samples[0], samples[0]
	for _, resident := range samples[1:] {
		if resident < minimum {
			minimum = resident
		}
		if resident > maximum {
			maximum = resident
		}
	}
	growth := maximum - first
	availability := "available"
	if unavailable > 0 {
		availability = "partial"
	}
	t.Logf("process envelope: platform=%s metric=resident_set source=%s unit=bytes availability=%s horizon_turns=%d samples=%d unavailable_samples=%d duration=%s resident_first_bytes=%d resident_last_bytes=%d resident_min_bytes=%d resident_max_bytes=%d resident_peak_growth_bytes=%d", runtime.GOOS, processResidentSource(), availability, progress, len(samples), unavailable, time.Since(started).Round(time.Millisecond), first, samples[len(samples)-1], minimum, maximum, growth)
}

func longHorizonProcessEnvelopeHelper(t *testing.T) {
	t.Helper()
	readyPath := os.Getenv(processEnvelopeReadyEnv)
	releasePath := os.Getenv(processEnvelopeReleaseEnv)
	if readyPath == "" || releasePath == "" {
		t.Fatal("process envelope helper missing marker paths")
	}
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintln(os.Stdout, "PICOGENT_PROCESS_ENVELOPE_READY")
	waitForProcessEnvelopeMarker(t, releasePath)
	fixture := newLongHorizonFixture(t)
	metrics := advanceLongHorizonObserved(t, fixture, processEnvelopeTurns, func(turn int) {
		_, _ = fmt.Fprintf(os.Stdout, "PICOGENT_PROCESS_ENVELOPE_PROGRESS %d\n", turn)
	})
	if metrics.reloads != processEnvelopeTurns {
		t.Fatalf("reloads = %d, want %d", metrics.reloads, processEnvelopeTurns)
	}
	_, _ = fmt.Fprintf(os.Stdout, "PICOGENT_PROCESS_ENVELOPE_DONE reloads=%d\n", metrics.reloads)
}

func processEnvelopeChildEnv(root, readyPath, releasePath string) []string {
	home := filepath.Join(root, "child-home")
	temp := filepath.Join(root, "child-temp")
	_ = os.MkdirAll(home, 0o700)
	_ = os.MkdirAll(temp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"TMPDIR=" + temp,
		"TEMP=" + temp,
		"TMP=" + temp,
		"GOMAXPROCS=2",
		processEnvelopeHelperEnv + "=1",
		processEnvelopeReadyEnv + "=" + readyPath,
		processEnvelopeReleaseEnv + "=" + releasePath,
	}
	if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
		env = append(env, "SystemRoot="+systemRoot)
	}
	return env
}

func readEnvelopeReady(scanner *bufio.Scanner) error {
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "PICOGENT_PROCESS_ENVELOPE_READY" {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("ready marker not found")
}

func waitForProcessEnvelopeMarker(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func withProcessStderr(err error, stderr string) error {
	if strings.TrimSpace(stderr) == "" {
		return err
	}
	return fmt.Errorf("%v\n%s", err, stderr)
}

type boundedProcessOutput struct {
	mu        sync.Mutex
	data      bytes.Buffer
	truncated bool
}

func (b *boundedProcessOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	const limit = 64 << 10
	remaining := limit - b.data.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.data.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.data.Write(p)
	return len(p), nil
}

func (b *boundedProcessOutput) String() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncated {
		return b.data.String() + "\n[child stderr truncated]"
	}
	return b.data.String()
}
