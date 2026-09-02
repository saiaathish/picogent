package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsExternalEvidenceDirectory(t *testing.T) {
	workspace := t.TempDir()
	evidenceDir := filepath.Join(filepath.Dir(workspace), "picogent-release-evidence")
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{
		"--workspace", workspace,
		"--evidence-dir", evidenceDir,
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("run exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "release evidence layout PASS\n" || stderr.Len() != 0 {
		t.Fatalf("stdout/stderr = %q / %q", stdout.String(), stderr.String())
	}
}

func TestRunRejectsWorkspaceEvidenceDirectory(t *testing.T) {
	workspace := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{
		"--workspace", workspace,
		"--evidence-dir", filepath.Join(workspace, "artifacts"),
	}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "inside workspace") {
		t.Fatalf("run code/stdout/stderr = %d / %q / %q", code, stdout.String(), stderr.String())
	}
}
