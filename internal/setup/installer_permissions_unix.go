//go:build !windows

package setup

import (
	"os"
	"path/filepath"
	"strings"
)

func trustedManagedDirectory(path string, leaf bool) bool {
	st, err := os.Lstat(path)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if leaf {
		return st.Mode().Perm()&0o077 == 0
	}
	return st.Mode().Perm()&0o022 == 0
}

func executableAncestorsProtected(root, target string) bool {
	root, ok := canonicalPath(root)
	if !ok {
		return false
	}
	target, ok = canonicalPath(target)
	if !ok {
		return false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	for current := root; ; current = filepath.Dir(current) {
		rootInfo, err := os.Stat(current)
		if err != nil || !rootInfo.IsDir() || (rootInfo.Mode().Perm()&0o022 != 0 && rootInfo.Mode()&os.ModeSticky == 0) {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	current := root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		st, err := os.Stat(current)
		if err != nil {
			return false
		}
		if i == len(parts)-1 {
			continue
		}
		if !st.IsDir() || (st.Mode().Perm()&0o022 != 0 && st.Mode()&os.ModeSticky == 0) {
			return false
		}
	}
	return true
}
