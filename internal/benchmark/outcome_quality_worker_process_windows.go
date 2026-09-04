//go:build windows

package benchmark

import (
	"os/exec"
	"strconv"
	"syscall"
)

func configureOutcomeQualityWorkerCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func terminateOutcomeQualityWorkerCommand(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	kill := exec.Command("taskkill.exe", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
	kill.Env = outcomeQualityWorkerEnvironment()
	_ = kill.Run()
}
