//go:build !windows

package tools

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type unixWorkspaceOpenKind uint8

const (
	unixWorkspaceRead unixWorkspaceOpenKind = iota
	unixWorkspaceWrite
	unixWorkspaceEdit
)

func openWorkspaceRead(workspace, abs string) (*os.File, error) {
	return openUnixWorkspaceFile(workspace, abs, unixWorkspaceRead)
}

func openWorkspaceWrite(workspace, abs string) (*os.File, error) {
	return openUnixWorkspaceFile(workspace, abs, unixWorkspaceWrite)
}

func openWorkspaceEdit(workspace, abs string) (*os.File, error) {
	return openUnixWorkspaceFile(workspace, abs, unixWorkspaceEdit)
}

// openUnixWorkspaceFile anchors the operation at a directory descriptor and
// walks every parent with openat(O_NOFOLLOW). Once the final descriptor is
// returned, renaming or replacing any path component cannot redirect the I/O.
func openUnixWorkspaceFile(workspace, abs string, kind unixWorkspaceOpenKind) (*os.File, error) {
	rel, err := secureWorkspaceRelative(workspace, abs)
	if err != nil {
		return nil, err
	}
	createParents := kind == unixWorkspaceWrite
	parent, leaf, err := openUnixWorkspaceParent(workspace, rel, createParents)
	if err != nil {
		return nil, err
	}
	defer unix.Close(parent)

	flags := unix.O_CLOEXEC | unix.O_NOFOLLOW
	switch kind {
	case unixWorkspaceRead:
		flags |= unix.O_RDONLY
	case unixWorkspaceWrite:
		// Do not truncate while opening. The caller checks cancellation and
		// then truncates the already-validated descriptor.
		flags |= unix.O_WRONLY | unix.O_CREAT
	case unixWorkspaceEdit:
		flags |= unix.O_RDWR
	default:
		return nil, fmt.Errorf("unknown secure workspace operation")
	}

	fd, err := unix.Openat(parent, leaf, flags, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open workspace file %q: %w", rel, err)
	}
	f := os.NewFile(uintptr(fd), abs)
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

func openUnixWorkspaceParent(workspace, rel string, create bool) (int, string, error) {
	parts, err := securePathParts(rel)
	if err != nil {
		return -1, "", err
	}
	fd, err := unix.Open(workspace, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
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
