//go:build unix

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/verify"
)

func TestRunRejectsSymlinkedReleaseGateLedger(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	root := t.TempDir()
	outside := filepath.Join(root, "outside.json")
	writeLedger(t, outside, verify.ReleaseGateLedger{
		Schema:       verify.ReleaseGateSchema,
		CandidateSHA: candidateSHA,
		Event:        "pull_request",
		Gates: []verify.ReleaseGateRecord{
			{CandidateSHA: candidateSHA, Event: "pull_request", Job: "test", OS: "matrix", Command: "go test ./...", Status: "PASS"},
		},
	})
	linked := filepath.Join(root, "ledger.json")
	if err := os.Symlink(outside, linked); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{
		"--ledger", linked, "--expected-sha", candidateSHA, "--event", "pull_request", "--required", "test",
	}, &stdout, &stderr); code == 0 || (!strings.Contains(stderr.String(), "symbolic link") && !strings.Contains(stderr.String(), "not a regular file")) {
		t.Fatalf("run code/stdout/stderr = %d / %q / %q", code, stdout.String(), stderr.String())
	}
}

func TestRunRejectsSymlinkedReleaseGateLedgerParent(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeLedger(t, filepath.Join(realDir, "ledger.json"), verify.ReleaseGateLedger{
		Schema:       verify.ReleaseGateSchema,
		CandidateSHA: candidateSHA,
		Event:        "pull_request",
		Gates: []verify.ReleaseGateRecord{
			{CandidateSHA: candidateSHA, Event: "pull_request", Job: "test", OS: "matrix", Command: "go test ./...", Status: "PASS"},
		},
	})
	linkedDir := filepath.Join(root, "linked")
	if err := os.Symlink(realDir, linkedDir); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{
		"--ledger", filepath.Join(linkedDir, "ledger.json"), "--expected-sha", candidateSHA, "--event", "pull_request", "--required", "test",
	}, &stdout, &stderr); code == 0 {
		t.Fatalf("run unexpectedly passed: stdout/stderr = %q / %q", stdout.String(), stderr.String())
	}
}
