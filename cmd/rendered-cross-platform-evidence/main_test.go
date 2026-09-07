package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/runtimeboundary"
)

func TestRunAggregatesAndRetainsAllRequiredPlatforms(t *testing.T) {
	workspace := t.TempDir()
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	sha := strings.Repeat("a", 40)
	args := []string{"--workspace", workspace, "--candidate-sha", sha, "--out", filepath.Join(outputDir, "aggregate.json")}
	for _, platform := range []string{"darwin", "linux", "windows"} {
		path := filepath.Join(inputDir, platform+".json")
		writeInput(t, path, runtimeboundary.RenderedPlatformEvidence{
			Schema:             runtimeboundary.RenderedEvidenceSchema,
			CandidateSHA:       sha,
			Platform:           platform,
			Architecture:       "amd64",
			Environment:        "task-owned-disposable",
			Browser:            "browseros-neo",
			Fixture:            "rendered-recovery",
			ObservationSHA256:  strings.Repeat("1", 64),
			ScreenshotSHA256:   "UNRECORDED",
			ObservedAt:         "2026-09-07T00:00:00Z",
			Verdict:            runtimeboundary.VerdictPass,
			SourceTreeModified: false,
		})
		args = append(args, "--"+platform, path)
	}

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("run exit code = %d, stderr = %q", code, stderr.String())
	}
	var got runtimeboundary.RenderedCrossPlatformEvidence
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if got.Verdict != runtimeboundary.VerdictPass || len(got.Platforms) != 3 {
		t.Fatalf("aggregate = %+v", got)
	}
	if !strings.Contains(stderr.String(), "retained rendered cross-platform evidence") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(outputDir, "aggregate.json")); err != nil {
		t.Fatalf("retained aggregate missing: %v", err)
	}
}

func TestRunRejectsInputInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	inputDir := t.TempDir()
	sha := strings.Repeat("b", 40)
	args := []string{"--workspace", workspace, "--candidate-sha", sha, "--out", filepath.Join(inputDir, "aggregate.json")}
	for _, platform := range []string{"darwin", "linux", "windows"} {
		path := filepath.Join(workspace, platform+".json")
		writeInput(t, path, runtimeboundary.RenderedPlatformEvidence{
			Schema:             runtimeboundary.RenderedEvidenceSchema,
			CandidateSHA:       sha,
			Platform:           platform,
			Architecture:       "amd64",
			Environment:        "task-owned-disposable",
			Browser:            "browseros-neo",
			Fixture:            "rendered-recovery",
			ObservationSHA256:  strings.Repeat("2", 64),
			ScreenshotSHA256:   "UNRECORDED",
			ObservedAt:         "2026-09-07T00:00:00Z",
			Verdict:            runtimeboundary.VerdictPass,
			SourceTreeModified: false,
		})
		args = append(args, "--"+platform, path)
	}

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), args, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "inside workspace") {
		t.Fatalf("run code/stdout/stderr = %d / %q / %q", code, stdout.String(), stderr.String())
	}
}

func TestRunRequiresAggregateOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--candidate-sha", strings.Repeat("c", 40)}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "-out is required") {
		t.Fatalf("run code/stdout/stderr = %d / %q / %q", code, stdout.String(), stderr.String())
	}
}

func writeInput(t *testing.T, path string, evidence runtimeboundary.RenderedPlatformEvidence) {
	t.Helper()
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
