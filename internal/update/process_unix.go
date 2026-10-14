//go:build !windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// On macOS, close only the Terminal tab that ran this updater, after this
// process has exited. A failed update never schedules a close, leaving the
// error visible. Other Terminal windows and tabs are never targeted.
func scheduleTerminalClose() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	tty, err := filepath.EvalSymlinks("/dev/fd/0")
	if err != nil || !strings.HasPrefix(tty, "/dev/tty") {
		return false
	}
	script := `on run argv
tell application "Terminal"
repeat with terminalWindow in windows
repeat with terminalTab in tabs of terminalWindow
if (tty of terminalTab as text) is (item 1 of argv) then
close terminalTab
return
end if
end repeat
end repeat
end tell
end run`
	cmd := exec.Command("sh", "-c", `while kill -0 "$1" 2>/dev/null; do sleep 0.1; done; osascript -e "$2" "$3"`, "picogent-update", strconv.Itoa(os.Getpid()), script, tty)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return false
	}
	_ = cmd.Process.Release()
	return true
}

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
