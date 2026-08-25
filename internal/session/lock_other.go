//go:build !unix && !windows

package session

import (
	"os"
	"path/filepath"
	"sync"
)

var sessionsProcessLock sync.Mutex

func acquireSessionsLock(dir string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return nil, err
	}
	sessionsProcessLock.Lock()
	return sessionsProcessLock.Unlock, nil
}
