package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCaptureDetectsExternalRewriteOnUnchangedPath(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "already-dirty.go", "before\n")
	before, err := Capture(context.Background(), root, []string{"already-dirty.go"})
	if err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, "already-dirty.go", "after\n")
	after, err := Capture(context.Background(), root, []string{"already-dirty.go"})
	if err != nil {
		t.Fatal(err)
	}
	comparison := Compare(before, after)
	if !comparison.Changed || comparison.Fresh || comparison.Unknown {
		t.Fatalf("rewrite comparison = %+v", comparison)
	}
}

func TestCaptureTreatsSameContentAsFresh(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "same.txt", "same\n")
	before, err := Capture(context.Background(), root, []string{"same.txt"})
	if err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, "same.txt", "same\n")
	after, err := Capture(context.Background(), root, []string{"same.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if comparison := Compare(before, after); !comparison.Fresh || comparison.Changed || comparison.Unknown {
		t.Fatalf("same-content comparison = %+v", comparison)
	}
}

func TestCaptureDetectsCreateAndDelete(t *testing.T) {
	root := t.TempDir()
	before, err := Capture(context.Background(), root, []string{"optional.txt"})
	if err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, "optional.txt", "created\n")
	after, err := Capture(context.Background(), root, []string{"optional.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if comparison := Compare(before, after); !comparison.Changed {
		t.Fatalf("create comparison = %+v", comparison)
	}
}

func TestCaptureRootReplacementFailsClosed(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, "tracked.txt", "one\n")
	before, err := Capture(context.Background(), root, []string{"tracked.txt"})
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(parent, "old-workspace")
	if err := os.Rename(root, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, "tracked.txt", "one\n")
	after, err := Capture(context.Background(), root, []string{"tracked.txt"})
	if err != nil {
		t.Fatal(err)
	}
	comparison := Compare(before, after)
	if !comparison.Changed || comparison.Fresh {
		t.Fatalf("root replacement comparison = %+v", comparison)
	}
}

func TestCaptureTruncationIsNeverFresh(t *testing.T) {
	root := t.TempDir()
	paths := make([]string, MaxTrackedFiles+1)
	for i := range paths {
		paths[i] = filepath.Join("files", fmt.Sprintf("file-%03d-%s.txt", i, strings.Repeat("x", 3)))
		writeWorkspaceFile(t, root, paths[i], "content\n")
	}
	before, err := Capture(context.Background(), root, paths)
	if err != nil {
		t.Fatal(err)
	}
	after, err := Capture(context.Background(), root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if !before.FilesTruncated || !after.FilesTruncated {
		t.Fatalf("truncation flags = %v, %v", before.FilesTruncated, after.FilesTruncated)
	}
	comparison := Compare(before, after)
	if comparison.Fresh || !comparison.Unknown {
		t.Fatalf("truncated comparison = %+v", comparison)
	}
}

func TestCaptureBoundsPathInputProcessing(t *testing.T) {
	root := t.TempDir()
	paths := make([]string, MaxPathInputs+1)
	for i := range paths {
		paths[i] = "missing.txt"
	}
	observation, err := Capture(context.Background(), root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if !observation.FilesTruncated || len(observation.Files) != 1 {
		t.Fatalf("bounded path observation = truncated=%v files=%d", observation.FilesTruncated, len(observation.Files))
	}
	if comparison := Compare(observation, observation); comparison.Fresh || !comparison.Unknown {
		t.Fatalf("bounded path comparison = %+v", comparison)
	}
}

func TestCaptureLargeFileIsUnknown(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "large.bin", strings.Repeat("x", MaxFingerprintBytes+1))
	observation, err := Capture(context.Background(), root, []string{"large.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if len(observation.Files) != 1 || observation.Files[0].Known {
		t.Fatalf("large-file observation = %+v", observation.Files)
	}
	if comparison := Compare(observation, observation); comparison.Fresh || !comparison.Unknown {
		t.Fatalf("large-file comparison = %+v", comparison)
	}
}

func TestCompareWithoutTrackedFilesIsUnknown(t *testing.T) {
	root := t.TempDir()
	observation, err := Capture(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	comparison := Compare(observation, observation)
	if comparison.Fresh || !comparison.Unknown {
		t.Fatalf("root-only comparison = %+v", comparison)
	}
}

func TestCompareRejectsMalformedPublicObservation(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "tracked.txt", "content\n")
	observation, err := Capture(context.Background(), root, []string{"tracked.txt"})
	if err != nil {
		t.Fatal(err)
	}
	observation.Files[0].Identity.Known = false
	comparison := Compare(observation, observation)
	if comparison.Fresh || !comparison.Unknown {
		t.Fatalf("malformed comparison = %+v", comparison)
	}
}

func TestCaptureRejectsUnsafeFilePath(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Capture(context.Background(), root, []string{outside}); err == nil {
		t.Fatal("outside path was accepted")
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
			t.Fatal(err)
		}
		if _, err := Capture(context.Background(), root, []string{"link.txt"}); err == nil {
			t.Fatal("symlinked file was accepted")
		}
	}
}

func writeWorkspaceFile(t *testing.T, root, rel, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
