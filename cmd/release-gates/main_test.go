package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/verify"
)

func TestRunAcceptsValidReleaseGateLedger(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	path := filepath.Join(t.TempDir(), "release-gates.json")
	writeLedger(t, path, verify.ReleaseGateLedger{
		Schema:       verify.ReleaseGateSchema,
		CandidateSHA: candidateSHA,
		Event:        "pull_request",
		Gates: []verify.ReleaseGateRecord{
			{CandidateSHA: candidateSHA, Event: "pull_request", Job: "test", OS: "matrix", Command: "go test ./...", Status: "PASS"},
			{CandidateSHA: candidateSHA, Event: "pull_request", Job: "security", OS: "ubuntu-latest", Command: "govulncheck ./...", Status: "PASS"},
		},
	})

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{
		"--ledger", path, "--expected-sha", candidateSHA, "--event", "pull_request", "--required", "test,security",
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("run exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "release gates PASS") || stderr.Len() != 0 {
		t.Fatalf("stdout/stderr = %q / %q", stdout.String(), stderr.String())
	}
}

func TestRunRejectsFailedReleaseGate(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	path := filepath.Join(t.TempDir(), "release-gates.json")
	ledger := verify.ReleaseGateLedger{
		Schema:       verify.ReleaseGateSchema,
		CandidateSHA: candidateSHA,
		Event:        "push",
		Gates: []verify.ReleaseGateRecord{
			{CandidateSHA: candidateSHA, Event: "push", Job: "test", OS: "matrix", Command: "go test ./...", Status: "FAIL"},
		},
	}
	writeLedger(t, path, ledger)

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{
		"--ledger", path, "--expected-sha", candidateSHA, "--event", "push", "--required", "test",
	}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "gate test is FAIL") {
		t.Fatalf("run code/stdout/stderr = %d / %q / %q", code, stdout.String(), stderr.String())
	}
}

func TestRunRejectsOversizedReleaseGateLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release-gates.json")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", verify.MaxReleaseGateBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--ledger", path}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "exceeds size limit") {
		t.Fatalf("run code/stdout/stderr = %d / %q / %q", code, stdout.String(), stderr.String())
	}
}

func writeLedger(t *testing.T, path string, ledger verify.ReleaseGateLedger) {
	t.Helper()
	data, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
