package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const releaseURL = "https://api.github.com/repos/saiaathish/picogent/releases/latest"
const module = "github.com/saiaathish/picogent/cmd/picogent"

type Release struct {
	Tag       string `json:"tag"`
	Current   string `json:"current"`
	Available bool   `json:"available"`
	URL       string `json:"url"`
	Reason    string `json:"reason,omitempty"`
}

var HTTPClient = &http.Client{Timeout: 5 * time.Second}
var APIURL = releaseURL

// Check only trusts a published, non-prerelease GitHub release and never
// treats an older stable tag as an upgrade from a newer development build.
func Check(ctx context.Context, current string) (Release, error) {
	result := Release{Current: current}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, APIURL, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "picogent-updater")
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("GitHub release check returned HTTP %d", resp.StatusCode)
	}
	var remote struct {
		Tag        string `json:"tag_name"`
		URL        string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&remote); err != nil {
		return result, err
	}
	if remote.Draft || remote.Prerelease || !validTag(remote.Tag) {
		return result, errors.New("GitHub did not return a valid stable release")
	}
	result.Tag, result.URL = remote.Tag, remote.URL
	result.Available = compare(remote.Tag, current) > 0
	if !result.Available {
		result.Reason = "Already on this version or a newer development build"
	}
	return result, nil
}

func validTag(tag string) bool {
	if !strings.HasPrefix(tag, "v") || strings.ContainsAny(tag, "/@\\ \t\r\n") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(tag, "v"), ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func parseVersion(v string) ([3]uint64, bool, bool) {
	v = strings.TrimPrefix(v, "v")
	pre := strings.Contains(v, "-")
	v = strings.SplitN(v, "+", 2)[0]
	v = strings.SplitN(v, "-", 2)[0]
	parts := strings.Split(v, ".")
	var nums [3]uint64
	if len(parts) != 3 {
		return nums, false, false
	}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return nums, false, false
		}
		nums[i] = n
	}
	return nums, pre, true
}

func compare(tag, current string) int {
	a, _, _ := parseVersion(tag)
	b, pre, ok := parseVersion(current)
	if !ok {
		return 0
	} // unknown local build: no automatic upgrade offer
	for i := range a {
		if a[i] > b[i] {
			return 1
		}
		if a[i] < b[i] {
			return -1
		}
	}
	if pre {
		return 1
	}
	return 0
}

// Apply runs in a separate process. It builds and validates before touching
// the installed executable, then waits for the old GUI process to exit.
func Apply(ctx context.Context, tag, current string, parentPID int) error {
	if !validTag(tag) {
		return errors.New("invalid release tag")
	}
	release, err := Check(ctx, current)
	if err != nil {
		return err
	}
	if release.Tag != tag || !release.Available {
		return errors.New("release changed or is not newer than this build")
	}
	target, err := os.Executable()
	if err != nil {
		return err
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("Go is required to install this release; install Go and retry")
	}
	stage, err := os.MkdirTemp("", "picogent-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	fmt.Println("Downloading and building", tag)
	buildCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(buildCtx, "go", "install", "-v", module+"@"+tag)
	cmd.Env = append(os.Environ(), "GOBIN="+stage, "GOMAXPROCS=2")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err = <-done:
			goto built
		case <-ticker.C:
			fmt.Println("Still downloading or compiling…")
		}
	}
built:
	if err != nil {
		return fmt.Errorf("build failed: %w (existing Picogent is unchanged)", err)
	}
	name := "picogent"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	built := filepath.Join(stage, name)
	output, err := exec.CommandContext(ctx, built, "version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "picogent "+strings.TrimPrefix(tag, "v") {
		return fmt.Errorf("built binary did not report %s (got %q); existing Picogent is unchanged", tag, strings.TrimSpace(string(output)))
	}
	// Copy into the target directory before stopping the old server. A failed
	// permission or disk check must leave the active installation running.
	prepared, err := os.CreateTemp(filepath.Dir(target), ".picogent-update-*")
	if err != nil {
		return fmt.Errorf("cannot write install directory: %w", err)
	}
	defer os.Remove(prepared.Name())
	source, err := os.Open(built)
	if err != nil {
		prepared.Close()
		return err
	}
	_, err = io.Copy(prepared, source)
	source.Close()
	if err == nil {
		err = prepared.Chmod(0755)
	}
	if err == nil {
		err = prepared.Sync()
	}
	if closeErr := prepared.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	backup := target + ".previous-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := os.Stat(backup); err == nil {
		return fmt.Errorf("backup already exists at %s; refusing to overwrite it", backup)
	} else if !os.IsNotExist(err) {
		return err
	}
	fmt.Println("Build verified. Restarting Picogent…")
	if parentPID > 0 {
		if err := stopParent(parentPID); err != nil {
			return err
		}
		if err := waitParent(parentPID, 15*time.Second); err != nil {
			return err
		}
	}
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("could not move old build: %w (restart: %v)", err, restart(target))
	}
	if err := os.Rename(prepared.Name(), target); err != nil {
		if restoreErr := os.Rename(backup, target); restoreErr != nil {
			return fmt.Errorf("replacement failed: %w; restore failed: %v (backup: %s)", err, restoreErr, backup)
		}
		return fmt.Errorf("replacement failed; previous build restored: %w (restart: %v)", err, restart(target))
	}
	if err := restart(target); err != nil {
		_ = os.Rename(target, prepared.Name())
		if restoreErr := os.Rename(backup, target); restoreErr != nil {
			return fmt.Errorf("restart failed: %w; restore failed: %v (backup: %s)", err, restoreErr, backup)
		}
		return fmt.Errorf("restart failed; previous build restored: %w (previous build restart: %v)", err, restart(target))
	}
	fmt.Println("Installed", tag, "(previous build:", backup+")")
	if scheduleTerminalClose() {
		fmt.Println("Picogent reopened. This update tab will close.")
	} else {
		fmt.Println("Picogent reopened.")
	}
	return nil
}

func restart(target string) error {
	start := exec.Command(target)
	start.Dir = filepath.Dir(target)
	start.Env = os.Environ()
	detach(start)
	if err := start.Start(); err != nil {
		return err
	}
	_ = start.Process.Release()
	return nil
}
