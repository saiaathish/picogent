//go:build unix

package session

import (
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

var sessionsProcessLock sync.Mutex

func acquireSessionsLock(dir string) (func(), error) {
	sessionsProcessLock.Lock()
	f, err := os.OpenFile(dir+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		sessionsProcessLock.Unlock()
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		sessionsProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
		sessionsProcessLock.Unlock()
	}, nil
}
