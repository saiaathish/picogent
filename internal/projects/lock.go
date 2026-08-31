package projects

import (
	"path/filepath"
	"sync"

	"github.com/saiaathish/picogent/internal/securefile"
)

var registryProcessLock sync.Mutex

func acquireRegistryLock(path string) (func(), error) {
	registryProcessLock.Lock()
	if err := securefile.EnsureDir(filepath.Dir(path), 0o700); err != nil {
		registryProcessLock.Unlock()
		return nil, err
	}
	f, err := securefile.OpenLockFile(path + ".lock")
	if err != nil {
		registryProcessLock.Unlock()
		return nil, err
	}
	unlock, err := securefile.LockFile(f, true)
	if err != nil {
		_ = f.Close()
		registryProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = unlock()
		_ = f.Close()
		registryProcessLock.Unlock()
	}, nil
}
