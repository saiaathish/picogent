//go:build unix

package evolve

import (
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

var evolveProcessLock sync.Mutex

func acquireStoreLock(path string) (func(), error) {
	evolveProcessLock.Lock()
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		evolveProcessLock.Unlock()
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		evolveProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
		evolveProcessLock.Unlock()
	}, nil
}
