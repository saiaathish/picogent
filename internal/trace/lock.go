package trace

import (
	"errors"
	"sync"

	"github.com/saiaathish/picogent/internal/securefile"
)

var traceProcessLock sync.Mutex

func acquireTraceLock(path string) (func() error, error) {
	traceProcessLock.Lock()
	file, err := securefile.OpenLockFile(path + ".lock")
	if err != nil {
		traceProcessLock.Unlock()
		return nil, err
	}
	unlock, err := securefile.LockFile(file, true)
	if err != nil {
		_ = file.Close()
		traceProcessLock.Unlock()
		return nil, err
	}
	return func() error {
		unlockErr := unlock()
		closeErr := file.Close()
		traceProcessLock.Unlock()
		return errors.Join(unlockErr, closeErr)
	}, nil
}
