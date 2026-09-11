package evolve

import (
	"sync"

	"github.com/saiaathish/picogent/internal/securefile"
)

var evolveProcessLock sync.Mutex

func acquireStoreLock(path string) (func(), error) {
	evolveProcessLock.Lock()
	file, err := securefile.OpenLockFile(path + ".lock")
	if err != nil {
		evolveProcessLock.Unlock()
		return nil, err
	}
	release, err := securefile.LockFile(file, true)
	if err != nil {
		_ = file.Close()
		evolveProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = release()
		_ = file.Close()
		evolveProcessLock.Unlock()
	}, nil
}
