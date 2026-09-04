//go:build !windows

package benchmark

import (
	"os/exec"
	"syscall"
)

func configureOutcomeQualityWorkerCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachOutcomeQualityWorkerProcess(command *exec.Cmd) (func(), func(), error) {
	return func() { terminateOutcomeQualityWorkerCommand(command) }, func() {}, nil
}

func terminateOutcomeQualityWorkerCommand(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
		_ = command.Process.Kill()
	}
}
