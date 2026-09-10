//go:build !unix && !windows

package goal

import (
	"path/filepath"
	"sync"

	"github.com/saiaathish/picogent/internal/securefile"
)

var goalLock sync.Mutex

func acquireGoalLock(path string) (func(), error) {
	if err := securefile.EnsureDir(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	goalLock.Lock()
	return goalLock.Unlock, nil
}
