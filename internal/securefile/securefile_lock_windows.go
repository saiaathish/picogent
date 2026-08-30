//go:build windows

package securefile

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func lockSecureFile(file *os.File, exclusive bool) (func() error, error) {
	flags := uint32(0)
	if exclusive {
		flags = windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	overlapped := new(windows.Overlapped)
	handle := windows.Handle(file.Fd())
	if err := windows.LockFileEx(handle, flags, 0, 1, 0, overlapped); err != nil {
		if !errors.Is(err, windows.ERROR_IO_PENDING) {
			return nil, err
		}
		var transferred uint32
		if err := windows.GetOverlappedResult(handle, overlapped, &transferred, true); err != nil {
			return nil, err
		}
	}
	return func() error {
		err := windows.UnlockFileEx(handle, 0, 1, 0, overlapped)
		if !errors.Is(err, windows.ERROR_IO_PENDING) {
			return err
		}
		var transferred uint32
		return windows.GetOverlappedResult(handle, overlapped, &transferred, true)
	}, nil
}

func tryLockSecureFile(file *os.File, exclusive bool) (func() error, error) {
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if exclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	overlapped := new(windows.Overlapped)
	handle := windows.Handle(file.Fd())
	if err := windows.LockFileEx(handle, flags, 0, 1, 0, overlapped); err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errors.Join(ErrLocked, err)
		}
		if !errors.Is(err, windows.ERROR_IO_PENDING) {
			return nil, err
		}
		var transferred uint32
		if err := windows.GetOverlappedResult(handle, overlapped, &transferred, false); err != nil {
			if errors.Is(err, windows.ERROR_IO_INCOMPLETE) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
				return nil, errors.Join(ErrLocked, err)
			}
			return nil, err
		}
	}
	return func() error {
		err := windows.UnlockFileEx(handle, 0, 1, 0, overlapped)
		if !errors.Is(err, windows.ERROR_IO_PENDING) {
			return err
		}
		var transferred uint32
		return windows.GetOverlappedResult(handle, overlapped, &transferred, true)
	}, nil
}
