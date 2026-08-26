//go:build windows

package taskstate

import (
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

var taskStoresProcessLock sync.Mutex

func acquireTaskStoreLock(dir string) (func(), error) {
	taskStoresProcessLock.Lock()
	f, err := os.OpenFile(dir+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		taskStoresProcessLock.Unlock()
		return nil, err
	}
	overlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, overlapped); err != nil {
		_ = f.Close()
		taskStoresProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, overlapped)
		_ = f.Close()
		taskStoresProcessLock.Unlock()
	}, nil
}
