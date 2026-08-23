//go:build !windows

package taskstate

import "os"

func replaceFile(src, dst string) error {
	return os.Rename(src, dst)
}
