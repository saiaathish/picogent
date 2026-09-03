//go:build !windows

package tui

import (
	"os"
	"os/exec"
)

func prepareSignalChild(*exec.Cmd) error {
	return nil
}

func sendInterruptToChild(cmd *exec.Cmd) error {
	return cmd.Process.Signal(os.Interrupt)
}
