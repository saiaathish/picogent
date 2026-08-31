//go:build unix

package session

import (
	"sync"

	"github.com/saiaathish/picogent/internal/securefile"
)

var sessionsProcessLock sync.Mutex

func acquireSessionsLock(dir string) (func(), error) {
	sessionsProcessLock.Lock()
	f, err := securefile.OpenLockFile(dir + ".lock")
	if err != nil {
		sessionsProcessLock.Unlock()
		return nil, err
	}
	unlock, err := securefile.LockFile(f, true)
	if err != nil {
		_ = f.Close()
		sessionsProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = unlock()
		_ = f.Close()
		sessionsProcessLock.Unlock()
	}, nil
}
