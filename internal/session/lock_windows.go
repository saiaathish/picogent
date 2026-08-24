//go:build windows

package session

import (
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

var sessionsProcessLock sync.Mutex

func acquireSessionsLock(dir string) (func(), error) {
	sessionsProcessLock.Lock()
	f, err := os.OpenFile(dir+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		sessionsProcessLock.Unlock()
		return nil, err
	}
	overlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, overlapped); err != nil {
		_ = f.Close()
		sessionsProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, overlapped)
		_ = f.Close()
		sessionsProcessLock.Unlock()
	}, nil
}
