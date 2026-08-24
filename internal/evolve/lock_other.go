//go:build !unix && !windows

package evolve

import (
	"os"
	"path/filepath"
	"sync"
)

var evolveProcessLock sync.Mutex

func acquireStoreLock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	evolveProcessLock.Lock()
	return evolveProcessLock.Unlock, nil
}
