//go:build !windows

package setup

import "os"

func setupRunningElevated() bool {
	return os.Geteuid() == 0
}
