//go:build windows

package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type openKind uint8

const (
	openRead openKind = iota
	openWrite
	openEdit
)

func isWorkspaceNotExist(err error) bool {
	return os.IsNotExist(err)
}

func open(root, path string, kind openKind) (*os.File, error) {
	rel, err := Relative(root, path)
	if err != nil {
		return nil, err
	}
	parts, err := pathParts(rel)
	if err != nil {
		return nil, err
	}

	parent, err := openWindowsRoot(root)
	if err != nil {
		return nil, err
	}
	current := parent
	for _, part := range parts[:len(parts)-1] {
		child, openErr := openWindowsDirectory(current, part, kind == openWrite)
		if openErr != nil {
			_ = windows.CloseHandle(current)
			return nil, fmt.Errorf("open workspace directory %q: %w", part, openErr)
		}
		_ = windows.CloseHandle(current)
		current = child
	}

	h, err := openWindowsFile(current, parts[len(parts)-1], kind)
	_ = windows.CloseHandle(current)
	if err != nil {
		return nil, fmt.Errorf("open workspace file %q: %w", rel, err)
	}
	f := os.NewFile(uintptr(h), path)
	if f == nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("open workspace file %q: could not wrap handle", rel)
	}
	if err := verifyHandle(root, f, false); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("workspace file %q failed containment check: %w", rel, err)
	}
	return f, nil
}

func openDir(root, path string) (*os.File, error) {
	parts, err := directoryParts(root, path)
	if err != nil {
		return nil, err
	}
	current, err := openWindowsRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open workspace directory: %w", err)
	}
	for _, part := range parts {
		child, openErr := openWindowsDirectory(current, part, false)
		_ = windows.CloseHandle(current)
		if openErr != nil {
			return nil, fmt.Errorf("open workspace directory %q: %w", part, openErr)
		}
		current = child
	}
	f := os.NewFile(uintptr(current), path)
	if f == nil {
		_ = windows.CloseHandle(current)
		return nil, errors.New("open workspace directory: could not wrap handle")
	}
	if err := verifyHandle(root, f, true); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("workspace directory %q failed containment check: %w", path, err)
	}
	return f, nil
}

// NtCreateFile is used with RootDirectory handles instead of pathname-based
// creation. OBJ_DONT_REPARSE and FILE_OPEN_REPARSE_POINT make every component
// fail closed if a symlink, junction, or other reparse point is introduced.
func openWindowsRoot(root string) (windows.Handle, error) {
	name, err := windows.NewNTUnicodeString(ntPath(root))
	if err != nil {
		return 0, err
	}
	oa := objectAttributes(name, 0)
	var iosb windows.IO_STATUS_BLOCK
	var allocation int64
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_GENERIC_READ,
		&oa,
		&iosb,
		&allocation,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	if err != nil {
		return 0, translateNTError(err)
	}
	return handle, nil
}

func openWindowsDirectory(parent windows.Handle, name string, create bool) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	oa := objectAttributes(objectName, parent)
	var iosb windows.IO_STATUS_BLOCK
	var allocation int64
	disposition := uint32(windows.FILE_OPEN)
	if create {
		disposition = windows.FILE_OPEN_IF
	}
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_GENERIC_READ,
		&oa,
		&iosb,
		&allocation,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		disposition,
		windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	if err != nil {
		return 0, translateNTError(err)
	}
	return handle, nil
}

func openWindowsFile(parent windows.Handle, name string, kind openKind) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	oa := objectAttributes(objectName, parent)
	var iosb windows.IO_STATUS_BLOCK
	var allocation int64
	access := uint32(windows.FILE_GENERIC_READ)
	disposition := uint32(windows.FILE_OPEN)
	if kind == openWrite {
		access = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE
		disposition = windows.FILE_OPEN_IF
	} else if kind == openEdit {
		access = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE
	}
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		access,
		&oa,
		&iosb,
		&allocation,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		disposition,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	if err != nil {
		return 0, translateNTError(err)
	}
	return handle, nil
}

func objectAttributes(name *windows.NTUnicodeString, parent windows.Handle) windows.OBJECT_ATTRIBUTES {
	return windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    name,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
}

func ntPath(path string) string {
	if strings.HasPrefix(path, `\\?\UNC\`) {
		return `\??\UNC\` + strings.TrimPrefix(path, `\\?\UNC\`)
	}
	if strings.HasPrefix(path, `\\?\`) {
		return `\??\` + strings.TrimPrefix(path, `\\?\`)
	}
	if strings.HasPrefix(path, `\\`) {
		return `\??\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\??\` + path
}

func translateNTError(err error) error {
	var status windows.NTStatus
	if errors.As(err, &status) {
		switch status {
		case windows.STATUS_NO_SUCH_FILE, windows.STATUS_OBJECT_NAME_NOT_FOUND, windows.STATUS_OBJECT_PATH_NOT_FOUND:
			return fmt.Errorf("%w: %v", os.ErrNotExist, err)
		}
	}
	return err
}

func verifyHandle(root string, f *os.File, directory bool) error {
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
	actual, err := finalPath(windows.Handle(f.Fd()))
	if err != nil {
		return err
	}
	if !within(root, actual) {
		return fmt.Errorf("resolved path %q is outside workspace", actual)
	}
	return nil
}

func finalPath(handle windows.Handle) (string, error) {
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

func within(root, actual string) bool {
	root = normalizeFinalPath(root)
	actual = normalizeFinalPath(actual)
	if strings.EqualFold(root, actual) {
		return true
	}
	prefix := root
	if !strings.HasSuffix(prefix, `\`) {
		prefix += `\`
	}
	return len(actual) > len(prefix) && strings.EqualFold(actual[:len(prefix)], prefix)
}

func normalizeFinalPath(path string) string {
	path = filepath.Clean(path)
	if strings.HasPrefix(path, `\\?\UNC\`) {
		path = `\\` + strings.TrimPrefix(path, `\\?\UNC\`)
	} else if strings.HasPrefix(path, `\\?\`) {
		path = strings.TrimPrefix(path, `\\?\`)
	}
	return strings.TrimRight(path, `\`)
}

func remove(root, path string) error {
	rel, err := Relative(root, path)
	if err != nil {
		return err
	}
	parts, err := pathParts(rel)
	if err != nil {
		return err
	}
	parent, err := openWindowsRoot(root)
	if err != nil {
		return err
	}
	current := parent
	for _, part := range parts[:len(parts)-1] {
		child, openErr := openWindowsDirectory(current, part, false)
		if openErr != nil {
			_ = windows.CloseHandle(current)
			return fmt.Errorf("open workspace directory %q: %w", part, openErr)
		}
		_ = windows.CloseHandle(current)
		current = child
	}

	objectName, err := windows.NewNTUnicodeString(parts[len(parts)-1])
	if err != nil {
		_ = windows.CloseHandle(current)
		return err
	}
	oa := objectAttributes(objectName, current)
	var iosb windows.IO_STATUS_BLOCK
	var allocation int64
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.DELETE|windows.FILE_GENERIC_READ,
		&oa,
		&iosb,
		&allocation,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	_ = windows.CloseHandle(current)
	if err != nil {
		return fmt.Errorf("remove workspace file %q: %w", rel, translateNTError(err))
	}
	f := os.NewFile(uintptr(handle), path)
	if f == nil {
		_ = windows.CloseHandle(handle)
		return fmt.Errorf("remove workspace file %q: could not wrap handle", rel)
	}
	defer f.Close()
	if err := verifyHandle(root, f, false); err != nil {
		return fmt.Errorf("remove workspace file %q failed containment check: %w", rel, err)
	}
	var disposition uint32 = windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE
	if err := windows.NtSetInformationFile(
		windows.Handle(f.Fd()),
		&iosb,
		(*byte)(unsafe.Pointer(&disposition)),
		uint32(unsafe.Sizeof(disposition)),
		windows.FileDispositionInformationEx,
	); err != nil {
		return fmt.Errorf("remove workspace file %q: %w", rel, translateNTError(err))
	}
	return nil
}
