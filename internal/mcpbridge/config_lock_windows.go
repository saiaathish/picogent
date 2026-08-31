//go:build windows

package mcpbridge

import (
	"sync"

	"github.com/saiaathish/picogent/internal/securefile"
	"golang.org/x/sys/windows"
)

var mcpConfigProcessLock sync.Mutex

func acquireMCPConfigLock(path string) (func(), error) {
	mcpConfigProcessLock.Lock()
	f, err := securefile.OpenLockFile(path + ".lock")
	if err != nil {
		mcpConfigProcessLock.Unlock()
		return nil, err
	}
	overlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, overlapped); err != nil {
		_ = f.Close()
		mcpConfigProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, overlapped)
		_ = f.Close()
		mcpConfigProcessLock.Unlock()
	}, nil
}
