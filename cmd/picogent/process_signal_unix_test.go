//go:build !windows

package main

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
