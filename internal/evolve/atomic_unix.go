//go:build !windows

package evolve

import "os"

func replaceFile(src, dst string) error {
	return os.Rename(src, dst)
}
