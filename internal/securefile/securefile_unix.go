//go:build unix

package securefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

// unixParent is anchored to an already-open directory descriptor. Every
// operation below uses *at-style syscalls, so a rename or symlink swap in the
// original absolute path cannot redirect the operation to another tree.
type unixParent struct {
	fd int
}

func openSecureParent(path string, create bool) (secureParent, error) {
	return openSecureParentWithDurability(path, create, false)
}

func openSecureParentDurable(path string, create bool) (secureParent, error) {
	return openSecureParentWithDurability(path, create, true)
}

func openSecureParentWithDurability(path string, create bool, _ bool) (secureParent, error) {
	abs, err := secureAbsolutePath(path)
	if err != nil {
		return nil, err
	}
	fd, err := openUnixDirectory(abs, create, 0o700)
	if err != nil {
		return nil, err
	}
	return &unixParent{fd: fd}, nil
}

func ensureSecureDir(path string, mode os.FileMode) error {
	abs, err := secureAbsolutePath(path)
	if err != nil {
		return err
	}
	fd, err := openUnixDirectory(abs, true, mode)
	if err != nil {
		return err
	}
	return unix.Close(fd)
}

func secureAbsolutePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if runtime.GOOS != "darwin" {
		return abs, nil
	}

	// macOS exposes these stable system directories through symlinks. Resolve
	// only the known aliases; application-created symlink components remain
	// rejected by O_NOFOLLOW during descriptor traversal.
	for alias, target := range map[string]string{
		"/var": "/private/var",
		"/tmp": "/private/tmp",
		"/etc": "/private/etc",
	} {
		if abs != alias && !strings.HasPrefix(abs, alias+string(filepath.Separator)) {
			continue
		}
		resolved, evalErr := filepath.EvalSymlinks(alias)
		if evalErr != nil || filepath.Clean(resolved) != target {
			return "", fmt.Errorf("trusted system alias %q is not stable", alias)
		}
		return target + strings.TrimPrefix(abs, alias), nil
	}
	return abs, nil
}

func openUnixDirectory(path string, create bool, mode os.FileMode) (int, error) {
	current, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), string(filepath.Separator)), string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		child, openErr := unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil && create && errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(current, part, uint32(mode.Perm())); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(current)
				return -1, fmt.Errorf("create secure directory %q: %w", part, mkdirErr)
			}
			child, openErr = unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			_ = unix.Close(current)
			return -1, fmt.Errorf("open secure directory %q: %w", part, openErr)
		}
		_ = unix.Close(current)
		current = child
	}
	return current, nil
}

func (p *unixParent) Close() error {
	if p == nil || p.fd < 0 {
		return nil
	}
	err := unix.Close(p.fd)
	p.fd = -1
	return err
}

func (p *unixParent) stat(name string) (secureEntry, error) {
	var st unix.Stat_t
	if err := unix.Fstatat(p.fd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return secureEntry{}, err
	}
	mode := uint32(st.Mode)
	switch mode & uint32(unix.S_IFMT) {
	case uint32(unix.S_IFREG):
		return secureEntry{kind: secureEntryRegular, mode: os.FileMode(mode & 0o7777)}, nil
	case uint32(unix.S_IFDIR):
		return secureEntry{kind: secureEntryDirectory, mode: os.FileMode(mode & 0o7777)}, nil
	case uint32(unix.S_IFLNK):
		return secureEntry{kind: secureEntrySymlink, mode: os.FileMode(mode & 0o7777)}, nil
	default:
		return secureEntry{kind: secureEntryOther, mode: os.FileMode(mode & 0o7777)}, nil
	}
}

func unixFile(fd int, name string) (*os.File, error) {
	if fd < 0 {
		return nil, errors.New("invalid secure file descriptor")
	}
	f := os.NewFile(uintptr(fd), name)
	if f == nil {
		_ = unix.Close(fd)
		return nil, errors.New("could not wrap secure file descriptor")
	}
	return f, nil
}

func (p *unixParent) openRead(name string) (*os.File, error) {
	fd, err := unix.Openat(p.fd, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return unixFile(fd, name)
}

func (p *unixParent) openLock(name string) (*os.File, error) {
	fd, err := unix.Openat(p.fd, name, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	return unixFile(fd, name)
}

func (p *unixParent) openExclusive(name string, mode os.FileMode) (*os.File, error) {
	fd, err := unix.Openat(p.fd, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
	if err != nil {
		return nil, err
	}
	return unixFile(fd, name)
}

func (p *unixParent) remove(name string) error {
	return unix.Unlinkat(p.fd, name, 0)
}

func (p *unixParent) removeMatching(name string, source *os.File) error {
	if source == nil {
		return errors.New("atomic cleanup source is nil")
	}
	if name == "" {
		return nil
	}
	var expected unix.Stat_t
	if err := unix.Fstat(int(source.Fd()), &expected); err != nil {
		return fmt.Errorf("stat atomic cleanup source: %w", err)
	}
	var named unix.Stat_t
	if err := unix.Fstatat(p.fd, name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	// A replacement that raced cleanup is not ours. Leave it in place rather
	// than deleting an attacker-controlled inode.
	if !sameUnixFile(&expected, &named) {
		return nil
	}
	return unix.Unlinkat(p.fd, name, 0)
}

func (p *unixParent) replace(oldName, newName string, source *os.File) error {
	if source == nil {
		return errors.New("atomic replacement source is nil")
	}
	var expected unix.Stat_t
	if err := unix.Fstat(int(source.Fd()), &expected); err != nil {
		return fmt.Errorf("stat atomic replacement source: %w", err)
	}
	if uint32(expected.Mode)&uint32(unix.S_IFMT) != uint32(unix.S_IFREG) {
		return fmt.Errorf("atomic replacement source %q is not a regular file", oldName)
	}
	var named unix.Stat_t
	if err := unix.Fstatat(p.fd, oldName, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("stat atomic replacement name %q: %w", oldName, err)
	}
	if !sameUnixFile(&expected, &named) {
		return fmt.Errorf("atomic replacement name %q changed before commit", oldName)
	}

	// Validate the destination without following symlinks. Renameat is the
	// publication point: readers of newName observe one complete inode, never
	// a destination that is being truncated and refilled. The oldName identity
	// check is only a best-effort race detector; POSIX renameat has no
	// compare-and-rename-by-inode form, so a same-UID directory writer can still
	// replace oldName after this check.
	if destination, err := p.stat(newName); err == nil {
		if destination.kind == secureEntrySymlink {
			return fmt.Errorf("atomic replacement target %q is a symbolic link", newName)
		}
		if destination.kind != secureEntryRegular {
			return fmt.Errorf("atomic replacement target %q is not a regular file", newName)
		}
	} else if !errors.Is(err, unix.ENOENT) {
		return fmt.Errorf("stat atomic replacement target %q: %w", newName, err)
	}
	if err := unix.Renameat(p.fd, oldName, p.fd, newName); err != nil {
		return fmt.Errorf("publish atomic replacement %q: %w", newName, err)
	}
	return nil
}

func sameUnixFile(a, b *unix.Stat_t) bool {
	return a != nil && b != nil && a.Dev == b.Dev && a.Ino == b.Ino
}

func (p *unixParent) sync() error {
	if err := p.syncDurable(); err != nil && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOTSUP) && !errors.Is(err, unix.EOPNOTSUPP) {
		return err
	}
	return nil
}

func (p *unixParent) syncDurable() error {
	return unix.Fsync(p.fd)
}
