//go:build unix

package mcpbridge

import "os"

func replaceMCPFile(src, dst string) error {
	return os.Rename(src, dst)
}
