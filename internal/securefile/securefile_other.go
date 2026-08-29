//go:build !unix && !windows

package securefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The supported desktop targets use descriptor/handle implementations. This
// fallback keeps the package buildable on other Go targets while retaining the
// existing fail-closed symlink checks where the platform lacks *at APIs.
type rootParent struct {
	root *os.Root
}

func openSecureParent(path string, _ bool) (secureParent, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// There is no descriptor-relative mkdir traversal available on this
	// fallback target. Refuse missing or symlinked parents instead of
	// reintroducing a path-based race just to preserve convenience.
	if err := validateFallbackDirectory(path); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &rootParent{root: root}, nil
}

func ensureSecureDir(path string, mode os.FileMode) error {
	_ = mode
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := validateFallbackDirectory(path); err != nil {
		return err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	return root.Close()
}

func validateFallbackDirectory(path string) error {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	rootPath := volume + string(filepath.Separator)
	if rootPath == string(filepath.Separator) && strings.HasPrefix(path, string(filepath.Separator)) {
		// Keep the absolute root as the first component on Unix-like fallback
		// targets such as Plan 9.
		rootPath = string(filepath.Separator)
	}
	rest := strings.TrimPrefix(path, rootPath)
	if rest == path && volume == "" {
		return fmt.Errorf("secure parent %q is not absolute", path)
	}
	if info, err := os.Lstat(rootPath); err != nil {
		return err
	} else if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("secure parent %q is not a real directory", rootPath)
	}
	current := rootPath
	for _, part := range strings.FieldsFunc(rest, func(r rune) bool { return r == rune(filepath.Separator) }) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("secure parent %q is not a real directory", current)
		}
	}
	return nil
}

func (p *rootParent) Close() error { return p.root.Close() }

func (p *rootParent) stat(name string) (secureEntry, error) {
	info, err := p.root.Lstat(name)
	if err != nil {
		return secureEntry{}, err
	}
	kind := secureEntryOther
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		kind = secureEntrySymlink
	case info.Mode().IsRegular():
		kind = secureEntryRegular
	case info.IsDir():
		kind = secureEntryDirectory
	}
	return secureEntry{kind: kind, mode: info.Mode().Perm()}, nil
}

func (p *rootParent) openRead(name string) (*os.File, error) {
	return p.root.Open(name)
}

func (p *rootParent) openLock(name string) (*os.File, error) {
	return p.root.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
}

func (p *rootParent) openExclusive(name string, mode os.FileMode) (*os.File, error) {
	return p.root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode.Perm())
}

func (p *rootParent) remove(name string) error { return p.root.Remove(name) }

func (p *rootParent) removeMatching(name string, source *os.File) error {
	if source == nil {
		return errors.New("atomic cleanup source is nil")
	}
	if name == "" {
		return nil
	}
	expected, err := source.Stat()
	if err != nil {
		return err
	}
	named, err := p.root.Lstat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	// A replacement that raced cleanup is not ours. Leave it in place rather
	// than deleting an attacker-controlled inode.
	if !os.SameFile(expected, named) {
		return nil
	}
	return p.root.Remove(name)
}

func (p *rootParent) replace(oldName, newName string, source *os.File) error {
	if source == nil {
		return errors.New("atomic replacement source is nil")
	}
	expected, err := source.Stat()
	if err != nil {
		return err
	}
	named, err := p.root.Lstat(oldName)
	if err != nil {
		return err
	}
	if !os.SameFile(expected, named) {
		return fmt.Errorf("atomic replacement name %q changed before commit", oldName)
	}

	// Root.Rename is the fallback target's atomic publication primitive. Keep
	// the explicit no-symlink/type validation because a rename can otherwise
	// replace a symlink entry without following it.
	if destination, err := p.root.Lstat(newName); err == nil {
		if destination.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("atomic replacement target %q is a symbolic link", newName)
		}
		if !destination.Mode().IsRegular() {
			return fmt.Errorf("atomic replacement target %q is not a regular file", newName)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat atomic replacement target %q: %w", newName, err)
	}
	if err := p.root.Rename(oldName, newName); err != nil {
		return fmt.Errorf("publish atomic replacement %q: %w", newName, err)
	}
	return nil
}

func (p *rootParent) sync() error {
	file, err := p.root.Open(".")
	if err != nil {
		return nil
	}
	defer file.Close()
	if err := file.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		return fmt.Errorf("sync secure directory: %w", err)
	}
	return nil
}
