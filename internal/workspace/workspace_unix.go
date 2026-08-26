//go:build !windows

package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type openKind uint8

const (
	openRead openKind = iota
	openWrite
	openEdit
)

func open(root, path string, kind openKind) (*os.File, error) {
	rel, err := Relative(root, path)
	if err != nil {
		return nil, err
	}
	createParents := kind == openWrite
	parent, leaf, err := openParent(root, rel, createParents)
	if err != nil {
		return nil, err
	}
	defer unix.Close(parent)

	flags := unix.O_CLOEXEC | unix.O_NOFOLLOW
	switch kind {
	case openRead:
		flags |= unix.O_RDONLY
	case openWrite:
		flags |= unix.O_WRONLY | unix.O_CREAT
	case openEdit:
		flags |= unix.O_RDWR
	default:
		return nil, fmt.Errorf("unknown workspace operation")
	}

	fd, err := unix.Openat(parent, leaf, flags, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open workspace file %q: %w", rel, err)
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open workspace file %q: could not wrap descriptor", rel)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat workspace file %q: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("workspace path %q is not a regular file", rel)
	}
	return f, nil
}

func openParent(root, rel string, create bool) (int, string, error) {
	parts, err := pathParts(rel)
	if err != nil {
		return -1, "", err
	}
	fd, err := openUnixRoot(root)
	if err != nil {
		return -1, "", fmt.Errorf("open workspace directory: %w", err)
	}
	current := fd
	for _, part := range parts[:len(parts)-1] {
		child, openErr := unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil && create && errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(current, part, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(current)
				return -1, "", fmt.Errorf("create workspace directory %q: %w", part, mkdirErr)
			}
			child, openErr = unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			_ = unix.Close(current)
			return -1, "", fmt.Errorf("open workspace directory %q: %w", part, openErr)
		}
		_ = unix.Close(current)
		current = child
	}
	return current, parts[len(parts)-1], nil
}

func openUnixRoot(root string) (int, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return -1, err
	}
	if !filepath.IsAbs(root) {
		return -1, fmt.Errorf("workspace root is not absolute")
	}
	current, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(root), string(filepath.Separator)), string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		child, openErr := unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			_ = unix.Close(current)
			return -1, openErr
		}
		_ = unix.Close(current)
		current = child
	}
	return current, nil
}
