package setup

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OpenInteractive runs one already-resolved executable with fixed arguments in
// a visible terminal so beginners can finish interactive logins. It accepts an
// argv-shaped command instead of a caller-provided shell program and passes a
// credential-free setup environment to the terminal.
func OpenInteractive(bin string, args ...string) error {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return fmt.Errorf("empty command")
	}
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("login command %q is unavailable: %w", bin, err)
	}
	cmdLine := shellCommandLine(bin, args...)
	env := installerEnv(bin)
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`tell application "Terminal"
  activate
  do script %q
end tell`, cmdLine)
		cmd := exec.Command("osascript", "-e", script)
		cmd.Env = env
		return cmd.Start()
	case "windows":
		cmd := exec.Command("cmd", "/C", "start", "cmd", "/K", cmdLine)
		cmd.Env = env
		return cmd.Start()
	default:
		// Linux: try common terminal emulators.
		for _, try := range []struct {
			bin  string
			args []string
		}{
			{"gnome-terminal", []string{"--", "bash", "--noprofile", "--norc", "-c", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
			{"konsole", []string{"-e", "bash", "--noprofile", "--norc", "-c", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
			{"xfce4-terminal", []string{"-e", "bash --noprofile --norc -c " + shellQuote(cmdLine+"; echo; read -n 1 -s -r -p 'Press any key to close…'")}},
			{"x-terminal-emulator", []string{"-e", "bash", "--noprofile", "--norc", "-c", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
			{"xterm", []string{"-e", "bash", "--noprofile", "--norc", "-c", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
		} {
			if look(try.bin) == "" {
				continue
			}
			cmd := exec.Command(try.bin, try.args...)
			cmd.Env = env
			if err := cmd.Start(); err == nil {
				return nil
			}
		}
		// Last resort: start detached (may lack a TTY for interactive auth).
		cmd := exec.Command("bash", "--noprofile", "--norc", "-c", cmdLine)
		cmd.Env = env
		return cmd.Start()
	}
}

func shellCommandLine(bin string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, bin)
	parts = append(parts, args...)
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		if runtime.GOOS == "windows" {
			quoted = append(quoted, windowsQuote(part))
		} else {
			quoted = append(quoted, shellQuote(part))
		}
	}
	return strings.Join(quoted, " ")
}

func windowsQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '^', '&', '|', '<', '>', '%', '!':
			b.WriteByte('^')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
