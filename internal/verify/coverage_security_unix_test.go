//go:build unix

package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCoverProfileRejectsSymlinkedTarget(t *testing.T) {
	root := t.TempDir()
	realPath := filepath.Join(root, "real.out")
	writeCoverageProfile(t, realPath)
	linkedPath := filepath.Join(root, "linked.out")
	if err := os.Symlink(realPath, linkedPath); err != nil {
		t.Fatal(err)
	}

	got := ParseCoverProfile(linkedPath)
	if got.Status != ManifestUnverified || got.Reason != "coverprofile is unreadable" {
		t.Fatalf("coverage = %+v, want fail-closed unreadable result", got)
	}
}

func TestParseCoverProfileRejectsSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	realPath := filepath.Join(realDir, "coverage.out")
	writeCoverageProfile(t, realPath)
	linkedDir := filepath.Join(root, "linked")
	if err := os.Symlink(realDir, linkedDir); err != nil {
		t.Fatal(err)
	}

	got := ParseCoverProfile(filepath.Join(linkedDir, "coverage.out"))
	if got.Status != ManifestUnverified || got.Reason != "coverprofile is unreadable" {
		t.Fatalf("coverage = %+v, want fail-closed unreadable result", got)
	}
}

func TestParseCoverProfileRejectsOversizedProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", MaxCoverageProfileBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}

	got := ParseCoverProfile(path)
	if got.Status != ManifestUnverified || got.Reason != "coverprofile exceeds size limit" {
		t.Fatalf("coverage = %+v, want bounded unreadable result", got)
	}
}

func writeCoverageProfile(t *testing.T, path string) {
	t.Helper()
	content := "mode: set\nexample.com/pkg/file.go:1.1,2.2 2 1\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
