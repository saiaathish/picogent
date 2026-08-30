//go:build windows

package workspace

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var workspaceTempSequence atomic.Uint64

func writeAtomic(root, path string, data []byte) error {
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
		return fmt.Errorf("open workspace directory: %w", err)
	}
	current := parent
	defer func() { _ = windows.CloseHandle(current) }()
	for _, part := range parts[:len(parts)-1] {
		child, openErr := openWindowsDirectory(current, part, true)
		if openErr != nil {
			return fmt.Errorf("open workspace directory %q: %w", part, openErr)
		}
		_ = windows.CloseHandle(current)
		current = child
	}

	leaf := parts[len(parts)-1]
	if _, err := workspaceRegularEntry(current, leaf); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect workspace file %q: %w", rel, err)
		}
	}

	tmpName := ""
	var file *os.File
	for attempt := uint64(0); attempt < 32; attempt++ {
		seq := workspaceTempSequence.Add(1)
		tmpName = fmt.Sprintf(".picogent-workspace-%d-%d-%d.tmp", os.Getpid(), time.Now().UnixNano(), seq)
		file, err = openWorkspaceExclusive(current, tmpName, path)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create workspace temporary file %q: %w", rel, err)
		}
	}
	if file == nil {
		return errors.New("could not allocate a workspace temporary file")
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			removeWorkspaceTemp(current, tmpName, file)
			_ = file.Close()
		}
	}()

	if err := writeWorkspaceAll(file, data); err != nil {
		return fmt.Errorf("write workspace file %q: %w", rel, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync workspace file %q: %w", rel, err)
	}
	if matches, err := workspaceTempMatches(current, tmpName, file); err != nil {
		return fmt.Errorf("validate workspace temporary file %q: %w", rel, err)
	} else if !matches {
		return fmt.Errorf("workspace temporary file %q changed before commit", rel)
	}
	if _, err := workspaceRegularEntry(current, leaf); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("validate workspace target %q: %w", rel, err)
	}
	for attempt := 0; ; attempt++ {
		if err := renameWorkspaceHandle(windows.Handle(file.Fd()), current, leaf); err == nil {
			break
		} else if !retryWorkspaceRename(err) || attempt >= 99 {
			return fmt.Errorf("publish workspace file %q: %w", rel, err)
		}
		time.Sleep(time.Millisecond)
	}
	removeTemp = false
	if err := file.Close(); err != nil {
		return fmt.Errorf("close workspace file %q: %w", rel, err)
	}
	return nil
}

func openWorkspaceExclusive(parent windows.Handle, name, display string) (*os.File, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	oa := objectAttributes(objectName, parent)
	var iosb windows.IO_STATUS_BLOCK
	var allocation int64
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE,
		&oa,
		&iosb,
		&allocation,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_CREATE,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	if err != nil {
		var status windows.NTStatus
		if errors.As(err, &status) && status == windows.STATUS_OBJECT_NAME_COLLISION {
			return nil, fmt.Errorf("%w: %v", os.ErrExist, err)
		}
		return nil, translateNTError(err)
	}
	file := os.NewFile(uintptr(handle), display)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("open workspace file %q: could not wrap handle", display)
	}
	return file, nil
}

func workspaceRegularEntry(parent windows.Handle, name string) (Identity, error) {
	h, err := openWindowsFile(parent, name, openEdit)
	if err != nil {
		return Identity{}, err
	}
	f := os.NewFile(uintptr(h), name)
	if f == nil {
		_ = windows.CloseHandle(h)
		return Identity{}, errors.New("could not wrap workspace file handle")
	}
	defer f.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return Identity{}, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return Identity{}, fmt.Errorf("workspace path %q is a reparse point", name)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return Identity{}, fmt.Errorf("workspace path %q is not a regular file", name)
	}
	stat, err := f.Stat()
	if err != nil {
		return Identity{}, err
	}
	if !stat.Mode().IsRegular() {
		return Identity{}, fmt.Errorf("workspace path %q is not a regular file", name)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_READONLY != 0 {
		return Identity{}, fmt.Errorf("workspace path %q is read-only", name)
	}
	return Identity{Volume: uint64(info.VolumeSerialNumber), File: uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow), Known: true}, nil
}

func workspaceTempMatches(parent windows.Handle, name string, source *os.File) (bool, error) {
	if source == nil {
		return false, errors.New("workspace temporary source is nil")
	}
	expected, err := identityForFile(source)
	if err != nil {
		return false, err
	}
	namedHandle, err := openWindowsFile(parent, name, openRead)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	named := os.NewFile(uintptr(namedHandle), name)
	if named == nil {
		_ = windows.CloseHandle(namedHandle)
		return false, errors.New("could not wrap workspace temporary handle")
	}
	defer named.Close()
	actual, err := identityForFile(named)
	if err != nil {
		return false, err
	}
	return expected == actual, nil
}

func removeWorkspaceTemp(parent windows.Handle, name string, source *os.File) {
	if source == nil {
		return
	}
	expected, err := identityForFile(source)
	if err != nil {
		return
	}
	file, err := openWorkspaceDelete(parent, name)
	if err != nil {
		return
	}
	defer file.Close()
	actual, err := identityForFile(file)
	if err != nil || expected != actual {
		return
	}
	_ = deleteWorkspaceHandle(windows.Handle(file.Fd()))
}

func openWorkspaceDelete(parent windows.Handle, name string) (*os.File, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	oa := objectAttributes(objectName, parent)
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
	if err != nil {
		return nil, translateNTError(err)
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("could not wrap workspace delete handle")
	}
	return file, nil
}

func deleteWorkspaceHandle(handle windows.Handle) error {
	var iosb windows.IO_STATUS_BLOCK
	disposition := uint32(windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE)
	return translateNTError(windows.NtSetInformationFile(
		handle,
		&iosb,
		(*byte)(unsafe.Pointer(&disposition)),
		uint32(unsafe.Sizeof(disposition)),
		windows.FileDispositionInformationEx,
	))
}

type workspaceFileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func renameWorkspaceHandle(source, parent windows.Handle, name string) error {
	utf16Name, err := windows.UTF16FromString(name)
	if err != nil {
		return err
	}
	fileNameLength := (len(utf16Name) - 1) * 2
	var header workspaceFileRenameInformation
	bufferSize := int(unsafe.Offsetof(header.FileName)) + fileNameLength
	buffer := make([]byte, bufferSize)
	info := (*workspaceFileRenameInformation)(unsafe.Pointer(&buffer[0]))
	info.ReplaceIfExists = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	info.RootDirectory = parent
	info.FileNameLength = uint32(fileNameLength)
	copy((*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&info.FileName[0]))[:fileNameLength/2:fileNameLength/2], utf16Name[:len(utf16Name)-1])
	var iosb windows.IO_STATUS_BLOCK
	return translateNTError(windows.NtSetInformationFile(
		source,
		&iosb,
		&buffer[0],
		uint32(bufferSize),
		windows.FileRenameInformation,
	))
}

func retryWorkspaceRename(err error) bool {
	var status windows.NTStatus
	if errors.As(err, &status) {
		return status == windows.STATUS_ACCESS_DENIED || status == windows.STATUS_SHARING_VIOLATION
	}
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
