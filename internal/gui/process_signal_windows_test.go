//go:build windows

package gui

import (
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func prepareSignalChild(cmd *exec.Cmd) error {
	if cmd == nil {
		return fmt.Errorf("signal child command is nil")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return nil
}

func sendInterruptToChild(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("signal child process is not running")
	}
	// Windows delivers CTRL_BREAK_EVENT to a process group created with
	// CREATE_NEW_PROCESS_GROUP. Go's signal package maps it to os.Interrupt.
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
}
