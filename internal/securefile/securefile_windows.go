//go:build windows

package securefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// windowsParent is anchored to a directory handle opened with
// OBJ_DONT_REPARSE. Child opens use RootDirectory handles, so junctions and
// symlinks introduced after validation cannot redirect an operation.
type windowsParent struct {
	handle windows.Handle
}

func openSecureParent(path string, create bool) (secureParent, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	rootPath, parts, err := windowsPathParts(filepath.Clean(abs))
	if err != nil {
		return nil, err
	}
	current, err := openWindowsRoot(rootPath, create)
	if err != nil {
		return nil, err
	}
	for _, part := range parts {
		child, openErr := openWindowsDirectory(current, part, create)
		_ = windows.CloseHandle(current)
		if openErr != nil {
			return nil, fmt.Errorf("open secure directory %q: %w", part, openErr)
		}
		if err := verifyWindowsDirectory(child); err != nil {
			_ = windows.CloseHandle(child)
			return nil, fmt.Errorf("secure directory %q failed validation: %w", part, err)
		}
		current = child
	}
	return &windowsParent{handle: current}, nil
}

func ensureSecureDir(path string, _ os.FileMode) error {
	parent, err := openSecureParent(path, true)
	if err != nil {
		return err
	}
	return parent.Close()
}

func windowsPathParts(path string) (string, []string, error) {
	volume := filepath.VolumeName(path)
	if volume == "" {
		return "", nil, errors.New("secure path has no Windows volume")
	}
	root := volume + string(filepath.Separator)
	rest := strings.TrimLeft(strings.TrimPrefix(path, volume), `/\`)
	if rest == "" {
		return root, nil, nil
	}
	parts := strings.FieldsFunc(rest, func(r rune) bool { return r == '/' || r == '\\' })
	return root, parts, nil
}

func (p *windowsParent) Close() error {
	if p == nil || p.handle == 0 || p.handle == windows.InvalidHandle {
		return nil
	}
	err := windows.CloseHandle(p.handle)
	p.handle = 0
	return err
}

func windowsEntry(h windows.Handle) (secureEntry, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return secureEntry{}, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return secureEntry{kind: secureEntrySymlink}, nil
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return secureEntry{kind: secureEntryDirectory}, nil
	}
	return secureEntry{kind: secureEntryRegular}, nil
}

func (p *windowsParent) stat(name string) (secureEntry, error) {
	// Try a directory first so a directory reparse point is classified as a
	// symlink rather than as a missing/non-directory file.
	if h, err := openWindowsDirectory(p.handle, name, false); err == nil {
		defer windows.CloseHandle(h)
		return windowsEntry(h)
	}
	h, err := openWindowsFile(p.handle, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN)
	if err != nil {
		return secureEntry{}, translateWindowsError(err)
	}
	defer windows.CloseHandle(h)
	return windowsEntry(h)
}

func wrapWindowsFile(h windows.Handle, name string) (*os.File, error) {
	if h == 0 || h == windows.InvalidHandle {
		return nil, errors.New("invalid secure file handle")
	}
	f := os.NewFile(uintptr(h), name)
	if f == nil {
		_ = windows.CloseHandle(h)
		return nil, errors.New("could not wrap secure file handle")
	}
	return f, nil
}

func (p *windowsParent) openRead(name string) (*os.File, error) {
	h, err := openWindowsFile(p.handle, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN)
	if err != nil {
		return nil, translateWindowsError(err)
	}
	entry, err := windowsEntry(h)
	if err != nil || entry.kind != secureEntryRegular {
		_ = windows.CloseHandle(h)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("file %q is not a regular file", name)
	}
	return wrapWindowsFile(h, name)
}

func (p *windowsParent) openLock(name string) (*os.File, error) {
	h, err := openWindowsFile(p.handle, name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE, windows.FILE_OPEN_IF)
	if err != nil {
		return nil, translateWindowsError(err)
	}
	entry, err := windowsEntry(h)
	if err != nil || entry.kind != secureEntryRegular {
		_ = windows.CloseHandle(h)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("file %q is not a regular file", name)
	}
	return wrapWindowsFile(h, name)
}

func (p *windowsParent) openExclusive(name string, _ os.FileMode) (*os.File, error) {
	h, err := openWindowsFile(p.handle, name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE)
	if err != nil {
		return nil, translateWindowsError(err)
	}
	entry, err := windowsEntry(h)
	if err != nil || entry.kind != secureEntryRegular {
		_ = windows.CloseHandle(h)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("file %q is not a regular file", name)
	}
	return wrapWindowsFile(h, name)
}

func (p *windowsParent) remove(name string) error {
	h, err := openWindowsFile(p.handle, name, windows.DELETE|windows.FILE_GENERIC_READ, windows.FILE_OPEN)
	if err != nil {
		return translateWindowsError(err)
	}
	defer windows.CloseHandle(h)
	entry, err := windowsEntry(h)
	if err != nil {
		return err
	}
	if entry.kind == secureEntrySymlink {
		return fmt.Errorf("file %q is a reparse point", name)
	}
	if entry.kind != secureEntryRegular {
		return fmt.Errorf("file %q is not a regular file", name)
	}
	var iosb windows.IO_STATUS_BLOCK
	disposition := uint32(windows.FILE_DISPOSITION_DELETE)
	return translateWindowsError(windows.NtSetInformationFile(
		h,
		&iosb,
		(*byte)(unsafe.Pointer(&disposition)),
		uint32(unsafe.Sizeof(disposition)),
		windows.FileDispositionInformationEx,
	))
}

func windowsFileInfo(h windows.Handle) (windows.ByHandleFileInformation, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return windows.ByHandleFileInformation{}, err
	}
	return info, nil
}

func sameWindowsFile(a, b *windows.ByHandleFileInformation) bool {
	return a != nil && b != nil &&
		a.VolumeSerialNumber == b.VolumeSerialNumber &&
		a.FileIndexHigh == b.FileIndexHigh &&
		a.FileIndexLow == b.FileIndexLow
}

func (p *windowsParent) removeMatching(name string, source *os.File) error {
	if source == nil {
		return errors.New("atomic cleanup source is nil")
	}
	if name == "" {
		return nil
	}
	sourceInfo, err := windowsFileInfo(windows.Handle(source.Fd()))
	if err != nil {
		return err
	}
	target, err := openWindowsFile(p.handle, name, windows.FILE_GENERIC_READ|windows.DELETE, windows.FILE_OPEN)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(target)
	targetInfo, err := windowsFileInfo(target)
	if err != nil {
		return err
	}
	if targetInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		targetInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 ||
		!sameWindowsFile(&sourceInfo, &targetInfo) {
		// A replacement that raced cleanup is not ours. Leave it in place
		// rather than deleting an attacker-controlled handle.
		return nil
	}
	var iosb windows.IO_STATUS_BLOCK
	disposition := uint32(windows.FILE_DISPOSITION_DELETE)
	return translateWindowsError(windows.NtSetInformationFile(
		target,
		&iosb,
		(*byte)(unsafe.Pointer(&disposition)),
		uint32(unsafe.Sizeof(disposition)),
		windows.FileDispositionInformationEx,
	))
}

func (p *windowsParent) replace(oldName, newName string, source *os.File) error {
	if source == nil {
		return errors.New("atomic replacement source is nil")
	}
	h := windows.Handle(source.Fd())
	if h == 0 || h == windows.InvalidHandle {
		return errors.New("invalid atomic replacement handle")
	}
	entry, err := windowsEntry(h)
	if err != nil {
		return err
	}
	if entry.kind != secureEntryRegular {
		return fmt.Errorf("file %q is not a regular file", oldName)
	}
	sourceInfo, err := windowsFileInfo(h)
	if err != nil {
		return err
	}
	// Validate the temporary name for diagnostics, but never reopen it as the
	// source of truth after this point. A raced replacement therefore cannot
	// become the published file.
	named, err := openWindowsFile(p.handle, oldName, windows.FILE_GENERIC_READ, windows.FILE_OPEN)
	if err != nil {
		return fmt.Errorf("open atomic replacement name %q: %w", oldName, err)
	}
	namedInfo, err := windowsFileInfo(named)
	_ = windows.CloseHandle(named)
	if err != nil {
		return err
	}
	if !sameWindowsFile(&sourceInfo, &namedInfo) {
		return fmt.Errorf("atomic replacement name %q changed before commit", oldName)
	}

	// Validate the destination without following reparse points. The source
	// handle is then renamed directly, making publication atomic for ordinary
	// readers instead of truncating and copying into the destination.
	if destination, err := p.stat(newName); err == nil {
		if destination.kind == secureEntrySymlink {
			return fmt.Errorf("atomic replacement target %q is a reparse point", newName)
		}
		if destination.kind != secureEntryRegular {
			return fmt.Errorf("atomic replacement target %q is not a regular file", newName)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat atomic replacement target %q: %w", newName, err)
	}
	for attempt := 0; ; attempt++ {
		if err := renameWindowsHandle(windows.Handle(source.Fd()), p.handle, newName); err == nil {
			return nil
		} else if !retryWindowsRename(err) || attempt >= 99 {
			return fmt.Errorf("publish atomic replacement %q: %w", newName, err)
		}
		// Standard Windows path readers may keep the destination open without
		// delete sharing. Wait briefly for those short-lived handles to close;
		// replacing the destination remains atomic once the rename is accepted.
		time.Sleep(time.Millisecond)
	}
}

func retryWindowsRename(err error) bool {
	var status windows.NTStatus
	if errors.As(err, &status) {
		return status == windows.STATUS_ACCESS_DENIED || status == windows.STATUS_SHARING_VIOLATION
	}
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

type windowsFileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func renameWindowsHandle(source, parent windows.Handle, name string) error {
	utf16Name, err := windows.UTF16FromString(name)
	if err != nil {
		return err
	}
	fileNameLength := (len(utf16Name) - 1) * 2
	var header windowsFileRenameInformation
	bufferSize := int(unsafe.Offsetof(header.FileName)) + fileNameLength
	buffer := make([]byte, bufferSize)
	info := (*windowsFileRenameInformation)(unsafe.Pointer(&buffer[0]))
	info.ReplaceIfExists = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	info.RootDirectory = parent
	info.FileNameLength = uint32(fileNameLength)
	copy((*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&info.FileName[0]))[:fileNameLength/2:fileNameLength/2], utf16Name[:len(utf16Name)-1])
	var iosb windows.IO_STATUS_BLOCK
	return translateWindowsError(windows.NtSetInformationFile(
		source,
		&iosb,
		&buffer[0],
		uint32(bufferSize),
		windows.FileRenameInformation,
	))
}

func (p *windowsParent) sync() error { return nil }

func openWindowsRoot(path string, create bool) (windows.Handle, error) {
	access := uint32(windows.FILE_GENERIC_READ)
	if create {
		access |= windows.FILE_GENERIC_WRITE
	}
	return openWindowsHandle(0, ntPath(path), access, windows.FILE_OPEN, windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT)
}

func openWindowsDirectory(parent windows.Handle, name string, create bool) (windows.Handle, error) {
	access := uint32(windows.FILE_GENERIC_READ)
	if create {
		// The parent handle must carry directory-create rights when a caller
		// may create this component or a child file below the final directory.
		access |= windows.FILE_GENERIC_WRITE
	}
	disposition := uint32(windows.FILE_OPEN)
	if create {
		disposition = windows.FILE_OPEN_IF
	}
	return openWindowsHandle(parent, name, access, disposition, windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT)
}

func openWindowsFile(parent windows.Handle, name string, access, disposition uint32) (windows.Handle, error) {
	return openWindowsHandle(parent, name, access, disposition, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT)
}

func openWindowsHandle(parent windows.Handle, name string, access, disposition, options uint32) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	oa := windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var iosb windows.IO_STATUS_BLOCK
	var allocation int64
	var handle windows.Handle
	if err := windows.NtCreateFile(
		&handle,
		access,
		&oa,
		&iosb,
		&allocation,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		disposition,
		options,
		0,
		0,
	); err != nil {
		return 0, translateWindowsError(err)
	}
	return handle, nil
}

func verifyWindowsDirectory(h windows.Handle) error {
	entry, err := windowsEntry(h)
	if err != nil {
		return err
	}
	if entry.kind == secureEntrySymlink {
		return errors.New("reparse points are not allowed")
	}
	if entry.kind != secureEntryDirectory {
		return errors.New("not a directory")
	}
	return nil
}

func ntPath(path string) string {
	if strings.HasPrefix(path, `\\`) {
		return `\??\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\??\` + path
}

func translateWindowsError(err error) error {
	if err == nil {
		return nil
	}
	var status windows.NTStatus
	if errors.As(err, &status) {
		switch status {
		case windows.STATUS_NO_SUCH_FILE, windows.STATUS_OBJECT_NAME_NOT_FOUND, windows.STATUS_OBJECT_PATH_NOT_FOUND:
			return fmt.Errorf("%w: %v", os.ErrNotExist, err)
		case windows.STATUS_OBJECT_NAME_COLLISION, windows.STATUS_OBJECT_NAME_EXISTS:
			return fmt.Errorf("%w: %v", os.ErrExist, err)
		}
	}
	return err
}
