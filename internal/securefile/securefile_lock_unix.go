//go:build unix

package securefile

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lockSecureFile(file *os.File, exclusive bool) (func() error, error) {
	operation := unix.LOCK_SH
	if exclusive {
		operation = unix.LOCK_EX
	}
	if err := unix.Flock(int(file.Fd()), operation); err != nil {
		return nil, err
	}
	return func() error { return unix.Flock(int(file.Fd()), unix.LOCK_UN) }, nil
}

func tryLockSecureFile(file *os.File, exclusive bool) (func() error, error) {
	operation := unix.LOCK_SH | unix.LOCK_NB
	if exclusive {
		operation = unix.LOCK_EX | unix.LOCK_NB
	}
	if err := unix.Flock(int(file.Fd()), operation); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, errors.Join(ErrLocked, err)
		}
		return nil, err
	}
	return func() error { return unix.Flock(int(file.Fd()), unix.LOCK_UN) }, nil
}
