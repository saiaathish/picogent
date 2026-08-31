//go:build !windows

package agent

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// processResidentBytes reports the live resident set of a child process. The
// measurement is test-only: Linux has a stable procfs source, while the other
// supported Unix targets use the platform ps utility already present on the
// runner. It intentionally reports an error when the process has disappeared
// instead of turning a missing sample into zero.
func processResidentBytes(pid int) (int64, error) {
	if pid <= 0 {
		return 0, fmt.Errorf("invalid process id %d", pid)
	}
	if runtime.GOOS == "linux" {
		if resident, err := linuxResidentBytes(pid); err == nil {
			return resident, nil
		}
	}
	output, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, fmt.Errorf("read ps RSS for pid %d: %w", pid, err)
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return 0, fmt.Errorf("ps returned no RSS for pid %d", pid)
	}
	kilobytes, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || kilobytes <= 0 {
		return 0, fmt.Errorf("parse ps RSS for pid %d: %q", pid, strings.TrimSpace(string(output)))
	}
	return kilobytes * 1024, nil
}

func processResidentSource() string {
	if runtime.GOOS == "linux" {
		return "/proc/<pid>/status VmRSS with ps fallback"
	}
	return "ps -o rss= -p <pid>"
}

func linuxResidentBytes(pid int) (int64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != "VmRSS:" || fields[2] != "kB" {
			continue
		}
		kilobytes, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || kilobytes <= 0 {
			return 0, fmt.Errorf("parse /proc RSS for pid %d: %q", pid, line)
		}
		return kilobytes * 1024, nil
	}
	return 0, fmt.Errorf("/proc status has no VmRSS for pid %d", pid)
}
