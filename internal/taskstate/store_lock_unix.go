//go:build !windows

package taskstate

import (
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// The process mutex prevents same-process Store values from depending on the
// platform-specific semantics of flock for duplicated open descriptions.
var taskStoresProcessLock sync.Mutex

func acquireTaskStoreLock(dir string) (func(), error) {
	taskStoresProcessLock.Lock()
	f, err := os.OpenFile(dir+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		taskStoresProcessLock.Unlock()
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		taskStoresProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
		taskStoresProcessLock.Unlock()
	}, nil
}
