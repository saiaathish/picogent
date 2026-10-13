//go:build !windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

func stopParent(pid int) error {
	if pid == os.Getpid() {
		return fmt.Errorf("refusing to stop updater process")
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

func waitParent(pid int, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("old Picogent process %d did not exit", pid)
}
