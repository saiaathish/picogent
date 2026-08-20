package setup

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OpenInteractive runs cmdLine in a visible terminal so beginners can finish
// interactive logins (Claude / OpenCode / Antigravity) without typing commands themselves.
func OpenInteractive(cmdLine string) error {
	cmdLine = strings.TrimSpace(cmdLine)
	if cmdLine == "" {
		return fmt.Errorf("empty command")
	}
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`tell application "Terminal"
  activate
  do script %q
end tell`, cmdLine)
		cmd := exec.Command("osascript", "-e", script)
		cmd.Env = os.Environ()
		return cmd.Start()
	case "windows":
		cmd := exec.Command("cmd", "/C", "start", "cmd", "/K", cmdLine)
		cmd.Env = os.Environ()
		return cmd.Start()
	default:
		// Linux: try common terminal emulators.
		for _, try := range []struct {
			bin  string
			args []string
		}{
			{"gnome-terminal", []string{"--", "bash", "-lc", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
			{"konsole", []string{"-e", "bash", "-lc", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
			{"xfce4-terminal", []string{"-e", "bash -lc " + shellQuote(cmdLine+"; echo; read -n 1 -s -r -p 'Press any key to close…'")}},
			{"x-terminal-emulator", []string{"-e", "bash", "-lc", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
			{"xterm", []string{"-e", "bash", "-lc", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
		} {
			if look(try.bin) == "" {
				continue
			}
			cmd := exec.Command(try.bin, try.args...)
			cmd.Env = os.Environ()
			if err := cmd.Start(); err == nil {
				return nil
			}
		}
		// Last resort: start detached (may lack a TTY for interactive auth).
		cmd := exec.Command("bash", "-lc", cmdLine)
		cmd.Env = os.Environ()
		return cmd.Start()
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
