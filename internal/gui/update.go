package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/saiaathish/picogent/internal/update"
)

// Version is set by the CLI before opening the GUI so release builds and
// development builds use the same update comparison as `picogent version`.
var Version = "1.1.0-dev"

func (s *server) updateAPI(w http.ResponseWriter, r *http.Request) {
	result, err := update.Check(r.Context(), Version)
	if err != nil {
		http.Error(w, "Could not check GitHub releases: "+err.Error(), http.StatusBadGateway)
		return
	}
	if r.Method == http.MethodGet {
		if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
			result.Reason = "Automatic terminal update is not available on this platform yet"
			result.Available = false
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	if !result.Available {
		http.Error(w, result.Reason, http.StatusConflict)
		return
	}
	s.mu.Lock()
	busy := s.busy || s.sideBusy
	s.mu.Unlock()
	if busy {
		http.Error(w, "Finish the current chat before updating", http.StatusConflict)
		return
	}
	if err := launchUpdateTerminal(result.Tag); err != nil {
		http.Error(w, "Could not open update terminal: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func launchUpdateTerminal(tag string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{exe, "_apply-update", tag, strconv.Itoa(os.Getpid()), Version}
	switch runtime.GOOS {
	case "darwin":
		// AppleScript quotes each argument for the shell; no release metadata is
		// interpolated into AppleScript source or executed as a shell command.
		script := `on run argv
set commandText to quoted form of item 1 of argv & " _apply-update " & quoted form of item 2 of argv & " " & quoted form of item 3 of argv & " " & quoted form of item 4 of argv
tell application "Terminal"
activate
do script commandText
end tell
end run`
		return exec.Command("osascript", append([]string{"-e", script}, args[0], args[2], args[3], args[4])...).Run()
	case "linux":
		for _, terminal := range []string{"x-terminal-emulator", "gnome-terminal", "konsole"} {
			if path, err := exec.LookPath(terminal); err == nil {
				var cmd *exec.Cmd
				if terminal == "gnome-terminal" {
					cmd = exec.Command(path, append([]string{"--"}, args...)...)
				} else {
					cmd = exec.Command(path, append([]string{"-e"}, args...)...)
				}
				return cmd.Start()
			}
		}
		return fmt.Errorf("no supported terminal emulator found")
	default:
		return fmt.Errorf("automatic terminal update is unsupported on %s", strings.ToUpper(runtime.GOOS))
	}
}
