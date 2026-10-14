//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func detach(cmd *exec.Cmd)        {}
func scheduleTerminalClose() bool { return false }

func stopParent(pid int) error {
	if pid == os.Getpid() {
		return fmt.Errorf("refusing to stop updater process")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

func waitParent(pid int, limit time.Duration) error {
	// Windows Kill waits for process termination before returning.
	return nil
}
