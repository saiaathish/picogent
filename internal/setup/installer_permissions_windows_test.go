//go:build windows

package setup

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWindowsACLProtectedPathAcceptsDefaultTempACL(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "provider.exe")
	if err := os.WriteFile(target, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !executableAncestorsProtected(root, target) {
		logWindowsACLChain(t, target)
		t.Fatal("default temporary-directory ACL was rejected")
	}
}

func TestWindowsACLRejectsWritableAncestor(t *testing.T) {
	icacls := requireICACLS(t)
	root := t.TempDir()
	target := filepath.Join(root, "bin", "provider.exe")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	grantModify(t, icacls, root)
	if executableAncestorsProtected(root, target) {
		t.Fatal("writable Windows ancestor was accepted")
	}
}

func TestWindowsACLRejectsWritableTarget(t *testing.T) {
	icacls := requireICACLS(t)
	root := t.TempDir()
	target := filepath.Join(root, "provider.exe")
	if err := os.WriteFile(target, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	grantModify(t, icacls, target)
	if executableAncestorsProtected(root, target) {
		t.Fatal("writable Windows target was accepted")
	}
}

func requireICACLS(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("icacls.exe")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			t.Skip("UNVERIFIED: icacls.exe is unavailable")
		}
		t.Skipf("UNVERIFIED: cannot locate icacls.exe: %v", err)
	}
	return path
}

func grantModify(t *testing.T, icacls, path string) {
	t.Helper()
	// S-1-5-32-545 is the locale-independent built-in Users group. Modify
	// includes write data, write attributes, and delete rights.
	cmd := exec.Command(icacls, path, "/grant", "*S-1-5-32-545:M", "/C")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("UNVERIFIED: icacls could not grant a hostile ACL: %v (%s)", err, output)
	}
}

func logWindowsACLChain(t *testing.T, target string) {
	t.Helper()
	icacls, err := exec.LookPath("icacls.exe")
	if err != nil {
		t.Logf("icacls.exe unavailable: %v", err)
		return
	}
	seen := make(map[string]struct{})
	for current := target; ; current = filepath.Dir(current) {
		if _, ok := seen[current]; ok {
			break
		}
		seen[current] = struct{}{}
		output, _ := exec.Command(icacls, current).CombinedOutput()
		t.Logf("icacls %s:\n%s", current, output)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
}
