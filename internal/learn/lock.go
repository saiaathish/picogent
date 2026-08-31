package learn

import (
	"path/filepath"
	"sync"

	"github.com/saiaathish/picogent/internal/securefile"
)

var learnProcessLock sync.Mutex

func acquireLearnLock(path string) (func(), error) {
	learnProcessLock.Lock()
	if err := securefile.EnsureDir(filepath.Dir(path), 0o700); err != nil {
		learnProcessLock.Unlock()
		return nil, err
	}
	f, err := securefile.OpenLockFile(path + ".lock")
	if err != nil {
		learnProcessLock.Unlock()
		return nil, err
	}
	unlock, err := securefile.LockFile(f, true)
	if err != nil {
		_ = f.Close()
		learnProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = unlock()
		_ = f.Close()
		learnProcessLock.Unlock()
	}, nil
}
