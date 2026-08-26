//go:build !windows

package workspace

import (
	"errors"
	"os"
	"syscall"
)

func identityForFile(f *os.File) (Identity, error) {
	if f == nil {
		return Identity{}, errors.New("file is nil")
	}
	info, err := f.Stat()
	if err != nil {
		return Identity{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return Identity{}, errors.New("filesystem identity is unavailable")
	}
	return Identity{Volume: uint64(stat.Dev), File: uint64(stat.Ino), Known: true}, nil
}
