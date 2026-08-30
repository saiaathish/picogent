//go:build !windows

package taskstate

import (
	"sync"

	"github.com/saiaathish/picogent/internal/securefile"
)

// The process mutex prevents same-process Store values from depending on the
// platform-specific semantics of flock for duplicated open descriptions.
var taskStoresProcessLock sync.Mutex

func acquireTaskStoreLock(dir string) (func(), error) {
	taskStoresProcessLock.Lock()
	f, err := securefile.OpenLockFile(dir + ".lock")
	if err != nil {
		taskStoresProcessLock.Unlock()
		return nil, err
	}
	unlock, err := securefile.LockFile(f, true)
	if err != nil {
		_ = f.Close()
		taskStoresProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = unlock()
		_ = f.Close()
		taskStoresProcessLock.Unlock()
	}, nil
}
