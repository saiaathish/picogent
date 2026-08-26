//go:build windows

package workspace

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func identityForFile(f *os.File) (Identity, error) {
	if f == nil {
		return Identity{}, errors.New("file is nil")
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return Identity{}, err
	}
	fileIndex := uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)
	return Identity{Volume: uint64(info.VolumeSerialNumber), File: fileIndex, Known: true}, nil
}
