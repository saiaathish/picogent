//go:build windows

package setup

import "golang.org/x/sys/windows"

func setupRunningElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
