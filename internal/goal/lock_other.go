//go:build !unix && !windows

package goal

import (
	"os"
	"path/filepath"
	"sync"
)

var goalLock sync.Mutex

func acquireGoalLock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	goalLock.Lock()
	return goalLock.Unlock, nil
}
