//go:build windows

package evolve

import (
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

var evolveProcessLock sync.Mutex

func acquireStoreLock(path string) (func(), error) {
	evolveProcessLock.Lock()
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		evolveProcessLock.Unlock()
		return nil, err
	}
	overlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, overlapped); err != nil {
		_ = f.Close()
		evolveProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, overlapped)
		_ = f.Close()
		evolveProcessLock.Unlock()
	}, nil
}
