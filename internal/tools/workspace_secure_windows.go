//go:build windows

package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

type windowsWorkspaceOpenKind uint8

const (
	windowsWorkspaceRead windowsWorkspaceOpenKind = iota
	windowsWorkspaceWrite
	windowsWorkspaceEdit
)

func openWorkspaceRead(workspace, abs string) (*os.File, error) {
	return openWindowsWorkspaceFile(workspace, abs, windowsWorkspaceRead)
}

func openWorkspaceWrite(workspace, abs string) (*os.File, error) {
	return openWindowsWorkspaceFile(workspace, abs, windowsWorkspaceWrite)
}

func openWorkspaceEdit(workspace, abs string) (*os.File, error) {
	return openWindowsWorkspaceFile(workspace, abs, windowsWorkspaceEdit)
}

// Windows has no openat equivalent exposed by the standard library. This
// implementation revalidates every component with OPEN_REPARSE_POINT and
// verifies the final handle's resolved path before returning it. All actual
// reads and writes use that handle, so a final symlink/reparse-point swap
// cannot redirect I/O after validation.
func openWindowsWorkspaceFile(workspace, abs string, kind windowsWorkspaceOpenKind) (*os.File, error) {
	rel, err := secureWorkspaceRelative(workspace, abs)
	if err != nil {
		return nil, err
	}
	parent, leaf, err := openWindowsWorkspaceParent(workspace, rel, kind == windowsWorkspaceWrite)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(parent, leaf)

	access := uint32(windows.GENERIC_READ)
	disposition := uint32(windows.OPEN_EXISTING)
	if kind == windowsWorkspaceWrite {
		access = windows.GENERIC_WRITE
		disposition = windows.OPEN_ALWAYS
	} else if kind == windowsWorkspaceEdit {
		access = windows.GENERIC_READ | windows.GENERIC_WRITE
	}
	f, err := openWindowsNoFollow(path, access, disposition, false)
	if err != nil {
		return nil, fmt.Errorf("open workspace file %q: %w", rel, err)
	}
	if err := verifyWindowsHandle(workspace, f, false); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("workspace file %q failed containment check: %w", rel, err)
	}
	return f, nil
}

func openWindowsWorkspaceParent(workspace, rel string, create bool) (string, string, error) {
	parts, err := securePathParts(rel)
	if err != nil {
		return "", "", err
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", err
	}
	rootHandle, err := openWindowsNoFollow(root, windows.GENERIC_READ, windows.OPEN_EXISTING, true)
	if err != nil {
		return "", "", fmt.Errorf("open workspace directory: %w", err)
	}
	if err := verifyWindowsHandle(root, rootHandle, true); err != nil {
		_ = rootHandle.Close()
		return "", "", fmt.Errorf("workspace directory failed containment check: %w", err)
	}
	_ = rootHandle.Close()

	current := root
	for _, part := range parts[:len(parts)-1] {
		next := filepath.Join(current, part)
		_, statErr := os.Lstat(next)
		if errors.Is(statErr, os.ErrNotExist) && create {
			if mkdirErr := os.Mkdir(next, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return "", "", fmt.Errorf("create workspace directory %q: %w", part, mkdirErr)
			}
		} else if statErr != nil {
			return "", "", fmt.Errorf("inspect workspace directory %q: %w", part, statErr)
		}
		h, openErr := openWindowsNoFollow(next, windows.GENERIC_READ, windows.OPEN_EXISTING, true)
		if openErr != nil {
			return "", "", fmt.Errorf("open workspace directory %q: %w", part, openErr)
		}
		verifyErr := verifyWindowsHandle(root, h, true)
		_ = h.Close()
		if verifyErr != nil {
			return "", "", fmt.Errorf("workspace directory %q failed containment check: %w", part, verifyErr)
		}
		current = next
	}
	return current, parts[len(parts)-1], nil
}

func openWindowsNoFollow(path string, access, disposition uint32, directory bool) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	h, err := windows.CreateFile(
		p,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		disposition,
		flags|windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	if f == nil {
		_ = windows.CloseHandle(h)
		return nil, errors.New("could not wrap Windows handle")
	}
	return f, nil
}

func verifyWindowsHandle(root string, f *os.File, directory bool) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("reparse points are not allowed")
	}
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if directory {
		if !stat.IsDir() {
			return errors.New("not a directory")
		}
	} else if !stat.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	actual, err := windowsFinalPath(windows.Handle(f.Fd()))
	if err != nil {
		return err
	}
	if !windowsWithin(root, actual) {
		return fmt.Errorf("resolved path %q is outside workspace", actual)
	}
	return nil
}

func windowsFinalPath(handle windows.Handle) (string, error) {
	for size := uint32(256); size <= 1<<16; size *= 2 {
		buf := make([]uint16, size)
		n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], size, 0)
		if err == nil && n < size-1 {
			return filepath.Clean(windows.UTF16ToString(buf[:n])), nil
		}
		if err != nil && !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			return "", err
		}
	}
	return "", errors.New("Windows final path is too long")
}

func windowsWithin(root, actual string) bool {
	root = normalizeWindowsFinalPath(root)
	actual = normalizeWindowsFinalPath(actual)
	if strings.EqualFold(root, actual) {
		return true
	}
	prefix := root
	if !strings.HasSuffix(prefix, `\`) {
		prefix += `\`
	}
	return len(actual) > len(prefix) && strings.EqualFold(actual[:len(prefix)], prefix)
}

func normalizeWindowsFinalPath(path string) string {
	path = filepath.Clean(path)
	if strings.HasPrefix(path, `\\?\UNC\`) {
		path = `\\` + strings.TrimPrefix(path, `\\?\UNC\`)
	} else if strings.HasPrefix(path, `\\?\`) {
		path = strings.TrimPrefix(path, `\\?\`)
	}
	return strings.TrimRight(path, `\`)
}
