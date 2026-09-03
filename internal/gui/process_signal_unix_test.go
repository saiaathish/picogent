//go:build !windows

package gui

import (
	"fmt"
	"os"
	"os/exec"
)

func prepareSignalChild(*exec.Cmd) error {
	return nil
}

func sendInterruptToChild(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("signal child process is not running")
	}
	return cmd.Process.Signal(os.Interrupt)
}
