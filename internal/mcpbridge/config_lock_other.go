//go:build !unix && !windows

package mcpbridge

import (
	"path/filepath"
	"sync"

	"github.com/saiaathish/picogent/internal/securefile"
)

var mcpConfigProcessLock sync.Mutex

func acquireMCPConfigLock(path string) (func(), error) {
	if err := securefile.EnsureDir(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	mcpConfigProcessLock.Lock()
	return mcpConfigProcessLock.Unlock, nil
}
