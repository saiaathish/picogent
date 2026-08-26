package setup

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// runLoginCommand is a test seam for the synchronous CLI login path. Its
// production implementation revalidates the executable and supplies the same
// restricted environment used by interactive provider setup.
var runLoginCommand = runValidatedCommand

// OpenInteractive runs one already-resolved executable with fixed arguments in
// a visible terminal so beginners can finish interactive logins. It accepts an
// argv-shaped command instead of a caller-provided shell program and passes a
// credential-free setup environment to the terminal.
func OpenInteractive(bin string, args ...string) error {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return fmt.Errorf("empty command")
	}
	if setupElevatedFn() {
		return fmt.Errorf("refusing interactive provider login from an elevated process; rerun Picogent as the normal user")
	}
	validatedBin, err := validateLaunchExecutable(bin)
	if err != nil {
		return fmt.Errorf("login command %q is unavailable: %w", bin, err)
	}
	bin = validatedBin
	cmdLine := shellCommandLine(bin, args...)
	env := interactiveEnv(bin)
	switch runtime.GOOS {
	case "darwin":
		osascript := externalLook("osascript")
		if osascript == "" {
			return fmt.Errorf("osascript is unavailable")
		}
		osascript, err = validateLaunchExecutable(osascript)
		if err != nil {
			return err
		}
		script := fmt.Sprintf(`tell application "Terminal"
  activate
  do script %q
end tell`, cleanEnvironmentCommandLine(env, cmdLine))
		cmd := exec.Command(osascript, "-e", script)
		cmd.Env = env
		return cmd.Start()
	case "windows":
		cmdShell := externalLook("cmd.exe")
		if cmdShell == "" {
			return fmt.Errorf("cmd.exe is unavailable")
		}
		cmdShell, err = validateLaunchExecutable(cmdShell)
		if err != nil {
			return err
		}
		cmd := exec.Command(cmdShell, windowsInteractiveArgs(cmdShell, cmdLine)...)
		cmd.Env = env
		return cmd.Start()
	default:
		// Linux: try common terminal emulators.
		bash := externalLook("bash")
		if bash == "" {
			return fmt.Errorf("bash is unavailable")
		}
		bash, err = validateLaunchExecutable(bash)
		if err != nil {
			return err
		}
		for _, try := range []struct {
			bin  string
			args []string
		}{
			{"gnome-terminal", []string{"--", bash, "--noprofile", "--norc", "-c", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
			{"konsole", []string{"-e", bash, "--noprofile", "--norc", "-c", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
			{"xfce4-terminal", xfceTerminalArgs(bash, cmdLine)},
			{"x-terminal-emulator", []string{"-e", bash, "--noprofile", "--norc", "-c", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
			{"xterm", []string{"-e", bash, "--noprofile", "--norc", "-c", cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"}},
		} {
			terminal := externalLook(try.bin)
			if terminal == "" {
				continue
			}
			terminal, err = validateLaunchExecutable(terminal)
			if err != nil {
				continue
			}
			cmd := exec.Command(terminal, try.args...)
			cmd.Env = env
			if err := cmd.Start(); err == nil {
				return nil
			}
		}
		// Last resort: start detached (may lack a TTY for interactive auth).
		cmd := exec.Command(bash, "--noprofile", "--norc", "-c", cmdLine)
		cmd.Env = env
		return cmd.Start()
	}
}

func runValidatedCommand(bin string, args ...string) error {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return fmt.Errorf("empty command")
	}
	if setupElevatedFn() {
		return fmt.Errorf("refusing provider login from an elevated process; rerun Picogent as the normal user")
	}
	validatedBin, err := validateLaunchExecutable(bin)
	if err != nil {
		return fmt.Errorf("login command %q is unavailable: %w", bin, err)
	}
	cmd := exec.Command(validatedBin, args...)
	cmd.Env = interactiveEnv(validatedBin)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func windowsInteractiveArgs(cmdShell, cmdLine string) []string {
	return []string{"/D", "/C", "start", "", cmdShell, "/D", "/K", cmdLine}
}

func xfceTerminalArgs(bash, cmdLine string) []string {
	script := cmdLine + "; echo; read -n 1 -s -r -p 'Press any key to close…'"
	return []string{"-e", shellQuote(bash) + " --noprofile --norc -c " + shellQuote(script)}
}

func cleanEnvironmentCommandLine(env []string, cmdLine string) string {
	parts := []string{shellQuote("/usr/bin/env"), "-i"}
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		parts = append(parts, shellQuote(key+"="+value))
	}
	parts = append(parts, cmdLine)
	return strings.Join(parts, " ")
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
