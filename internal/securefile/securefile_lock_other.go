//go:build !unix && !windows

package securefile

import (
	"os"
	"sync"
)

var secureFileLock sync.RWMutex

func lockSecureFile(file *os.File, exclusive bool) (func() error, error) {
	if exclusive {
		secureFileLock.Lock()
		return func() error {
			secureFileLock.Unlock()
			return nil
		}, nil
	}
	secureFileLock.RLock()
	return func() error {
		secureFileLock.RUnlock()
		return nil
	}, nil
}

func tryLockSecureFile(file *os.File, exclusive bool) (func() error, error) {
	if exclusive {
		if !secureFileLock.TryLock() {
			return nil, ErrLocked
		}
		return func() error {
			secureFileLock.Unlock()
			return nil
		}, nil
	}
	if !secureFileLock.TryRLock() {
		return nil, ErrLocked
	}
	return func() error {
		secureFileLock.RUnlock()
		return nil
	}, nil
}
