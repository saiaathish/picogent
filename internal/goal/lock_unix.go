//go:build unix

package goal

import (
	"github.com/saiaathish/picogent/internal/securefile"
)

func acquireGoalLock(path string) (func(), error) {
	f, err := securefile.OpenLockFile(path + ".lock")
	if err != nil {
		return nil, err
	}
	unlock, err := securefile.LockFile(f, true)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = unlock()
		_ = f.Close()
	}, nil
}
