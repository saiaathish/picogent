package mcpbridge

import (
	"os"

	"github.com/saiaathish/picogent/internal/securefile"
)

func writeMCPFile(path string, data []byte, mode os.FileMode) error {
	return securefile.WriteAtomic(path, data, mode)
}
