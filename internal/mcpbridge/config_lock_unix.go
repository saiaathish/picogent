//go:build unix

package mcpbridge

import (
	"sync"

	"github.com/saiaathish/picogent/internal/securefile"
	"golang.org/x/sys/unix"
)

// The process mutex keeps same-process callers from depending on the
// platform-specific semantics of flock for duplicated open descriptions.
var mcpConfigProcessLock sync.Mutex

func acquireMCPConfigLock(path string) (func(), error) {
	mcpConfigProcessLock.Lock()
	f, err := securefile.OpenLockFile(path + ".lock")
	if err != nil {
		mcpConfigProcessLock.Unlock()
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		mcpConfigProcessLock.Unlock()
		return nil, err
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
		mcpConfigProcessLock.Unlock()
	}, nil
}
