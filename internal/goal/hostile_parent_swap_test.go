package goal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	goalHostileHelperEnv   = "PICOGENT_GOAL_HOSTILE_HELPER"
	goalHostileParentEnv   = "PICOGENT_GOAL_HOSTILE_PARENT"
	goalHostileBackupEnv   = "PICOGENT_GOAL_HOSTILE_BACKUP"
	goalHostileOutsideEnv  = "PICOGENT_GOAL_HOSTILE_OUTSIDE"
	goalHostileReadyEnv    = "PICOGENT_GOAL_HOSTILE_READY"
	goalHostileStartEnv    = "PICOGENT_GOAL_HOSTILE_START"
	goalHostileStopEnv     = "PICOGENT_GOAL_HOSTILE_STOP"
	goalHostileSwapsEnv    = "PICOGENT_GOAL_HOSTILE_SWAPS"
	goalHostileSourceEnv   = "PICOGENT_GOAL_HOSTILE_SOURCE_SHA"
	goalHostileEvidenceEnv = "PICOGENT_GOAL_HOSTILE_EVIDENCE_OUT"
	goalHostileAttempts    = 256
	goalHostilePause       = 250 * time.Microsecond
)

type goalHostileEvidence struct {
	Schema                 string `json:"schema"`
	CandidateSHA           string `json:"candidate_sha"`
	HostOS                 string `json:"host_os"`
	HostArch               string `json:"host_arch"`
	Attempts               int    `json:"attempts"`
	TrustedLoads           int    `json:"trusted_loads"`
	SuccessfulWrites       int    `json:"successful_writes"`
	SuccessfulRecoveries   int    `json:"successful_recoveries"`
	ConfirmedAttackerSwaps bool   `json:"confirmed_attacker_swaps"`
	OutsideMarkerObserved  bool   `json:"outside_marker_observed"`
	OutsideBeforeSHA256    string `json:"outside_before_sha256"`
	OutsideAfterSHA256     string `json:"outside_after_sha256"`
	SourceTreeModified     bool   `json:"source_tree_modified"`
	Verdict                string `json:"verdict"`
	BroadTOCTOUClaim       string `json:"broad_toctou_claim"`
}

// TestGoalHostileParentSwapConfinement exercises goal reads, writes, and
// backup recovery while an independent same-UID process swaps the goals
// directory for a symlink or Windows junction to an outside directory. The
// outside marker must never be loaded or mutated. This is a bounded named
// persistence observation, not a universal filesystem TOCTOU claim.
func TestGoalHostileParentSwapConfinement(t *testing.T) {
	if err := goalHostileReparseProbe(t); err != nil {
		if strings.TrimSpace(os.Getenv(goalHostileEvidenceEnv)) != "" {
			t.Fatalf("reparse support is required for evidence mode: %v", err)
		}
		t.Skipf("reparse support unavailable: %v", err)
	}
	if os.Getenv(goalHostileHelperEnv) == "1" {
		goalHostileParentSwapHelper(t)
		return
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	goalsDir := filepath.Join(home, "goals")
	goalsBackupDir := filepath.Join(home, "goals-real")
	outside := filepath.Join(root, "outside")
	t.Setenv("PICOGENT_HOME", home)
	if err := os.MkdirAll(goalsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := SetState(workspace, "inside-goal-marker"); err != nil {
		t.Fatal(err)
	}
	path, err := readPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(path)
	outsidePath := filepath.Join(outside, name)
	outsideBackupPath := stateBackupPath(outsidePath)
	outsideState := encodeState(State{Text: "outside-goal-marker", Revision: 9001})
	if err := os.WriteFile(outsidePath, outsideState, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideBackupPath, outsideState, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsidePath+".lock", []byte("lock\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideBefore, err := goalHostileTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}

	// Leave a trusted backup in place so the campaign covers the recovery path
	// as well as ordinary reads and writes.
	if err := os.Rename(path, stateBackupPath(path)); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(root, "ready")
	start := filepath.Join(root, "start")
	stop := filepath.Join(root, "stop")
	swaps := filepath.Join(root, "swaps")
	cmd := exec.Command(os.Args[0], "-test.run", "^TestGoalHostileParentSwapHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		goalHostileHelperEnv+"=1",
		goalHostileParentEnv+"="+goalsDir,
		goalHostileBackupEnv+"="+goalsBackupDir,
		goalHostileOutsideEnv+"="+outside,
		goalHostileReadyEnv+"="+ready,
		goalHostileStartEnv+"="+start,
		goalHostileStopEnv+"="+stop,
		goalHostileSwapsEnv+"="+swaps,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	goalHostileWaitForPath(t, ready)
	recovered, err := LoadState(workspace)
	if err != nil || recovered.Text != "inside-goal-marker" {
		t.Fatalf("trusted backup recovery = %#v err=%v", recovered, err)
	}
	successfulRecoveries := 1
	if err := os.WriteFile(start, []byte("start\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	goalHostileWaitForPath(t, swaps)

	trustedLoads := 0
	successfulWrites := 0
	outsideMarkerObserved := false
	for i := 0; i < goalHostileAttempts; i++ {
		state, loadErr := LoadState(workspace)
		if loadErr == nil {
			switch {
			case state.Text == "outside-goal-marker":
				outsideMarkerObserved = true
			case state.Text == "inside-goal-marker":
				trustedLoads++
			case strings.HasPrefix(state.Text, "inside-write-"):
				trustedLoads++
			}
		}
		if i%4 == 0 {
			if _, writeErr := SetState(workspace, fmt.Sprintf("inside-write-%d", i)); writeErr == nil {
				successfulWrites++
			}
		}
	}
	if err := os.WriteFile(stop, []byte("stop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("hostile goal helper failed: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}

	outsideAfter, err := goalHostileTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}
	if outsideMarkerObserved {
		t.Fatal("goal state loaded the outside marker through a reparse point")
	}
	t.Logf("hostile goal counts: trusted_loads=%d writes=%d recoveries=%d", trustedLoads, successfulWrites, successfulRecoveries)
	if trustedLoads == 0 {
		t.Fatal("hostile goal campaign did not observe a trusted load")
	}
	if successfulWrites == 0 {
		t.Fatal("hostile goal campaign did not observe a trusted write")
	}
	if successfulRecoveries == 0 {
		t.Fatal("hostile goal campaign did not observe backup recovery")
	}
	if outsideBefore != outsideAfter {
		t.Fatalf("outside goal tree changed: before=%s after=%s", outsideBefore, outsideAfter)
	}
	if state, err := LoadState(workspace); err != nil || strings.HasPrefix(state.Text, "outside-") {
		t.Fatalf("post-campaign trusted goal load = %#v err=%v", state, err)
	}

	sourceSHA, sourceTreeModified := goalHostileSourceState()
	if expected := strings.TrimSpace(os.Getenv(goalHostileSourceEnv)); expected != "" && expected != sourceSHA {
		t.Fatalf("source SHA mismatch: expected %s, got %s", expected, sourceSHA)
	}
	if evidencePath := strings.TrimSpace(os.Getenv(goalHostileEvidenceEnv)); evidencePath != "" {
		if sourceTreeModified {
			t.Fatal("source tree must be clean before retaining hostile goal evidence")
		}
		if err := goalHostileWriteEvidence(evidencePath, goalHostileEvidence{
			Schema:                 "picogent.v4.goal-state-persistence-evidence.v1",
			CandidateSHA:           sourceSHA,
			HostOS:                 runtime.GOOS,
			HostArch:               runtime.GOARCH,
			Attempts:               goalHostileAttempts,
			TrustedLoads:           trustedLoads,
			SuccessfulWrites:       successfulWrites,
			SuccessfulRecoveries:   successfulRecoveries,
			ConfirmedAttackerSwaps: true,
			OutsideMarkerObserved:  outsideMarkerObserved,
			OutsideBeforeSHA256:    outsideBefore,
			OutsideAfterSHA256:     outsideAfter,
			SourceTreeModified:     sourceTreeModified,
			Verdict:                "PASS",
			BroadTOCTOUClaim:       "UNVERIFIED",
		}); err != nil {
			t.Fatal(err)
		}
		t.Logf("goal hostile evidence path=%s source=%s", evidencePath, sourceSHA)
	}
}

func TestGoalHostileParentSwapHelper(t *testing.T) {
	if os.Getenv(goalHostileHelperEnv) != "1" {
		t.Skip("hostile helper subprocess only")
	}
	goalHostileParentSwapHelper(t)
}

func goalHostileReparseProbe(t *testing.T) error {
	t.Helper()
	root := t.TempDir()
	link := filepath.Join(root, "reparse")
	target := t.TempDir()
	if err := goalHostileCreateReparse(link, target); err != nil {
		return err
	}
	if err := os.Remove(link); err != nil {
		return fmt.Errorf("remove reparse probe: %w", err)
	}
	return nil
}

func goalHostileCreateReparse(link, target string) error {
	if runtime.GOOS != "windows" {
		return os.Symlink(target, link)
	}
	output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J %q -> %q: %w (%s)", link, target, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func goalHostileParentSwapHelper(t *testing.T) {
	parent := os.Getenv(goalHostileParentEnv)
	backup := os.Getenv(goalHostileBackupEnv)
	outside := os.Getenv(goalHostileOutsideEnv)
	ready := os.Getenv(goalHostileReadyEnv)
	start := os.Getenv(goalHostileStartEnv)
	stop := os.Getenv(goalHostileStopEnv)
	swapsPath := os.Getenv(goalHostileSwapsEnv)
	if parent == "" || backup == "" || outside == "" || ready == "" || start == "" || stop == "" || swapsPath == "" {
		t.Fatal("hostile goal helper environment is incomplete")
	}
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	goalHostileWaitForPath(t, start)
	confirmed := false
	for {
		if _, err := os.Stat(stop); err == nil {
			break
		}
		if err := os.Rename(parent, backup); err != nil {
			time.Sleep(time.Millisecond)
			continue
		}
		if err := goalHostileCreateReparse(parent, outside); err != nil {
			_ = os.Rename(backup, parent)
			time.Sleep(time.Millisecond)
			continue
		}
		confirmed = true
		if err := os.WriteFile(swapsPath, []byte("confirmed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(goalHostilePause)
		if err := os.Remove(parent); err != nil {
			t.Fatal(fmt.Errorf("remove hostile goal parent: %w", err))
		}
		if err := goalHostileRestoreParent(parent, backup); err != nil {
			t.Fatal(err)
		}
		time.Sleep(goalHostilePause)
	}
	if _, err := os.Stat(parent); errors.Is(err, os.ErrNotExist) {
		if err := goalHostileRestoreParent(parent, backup); err != nil {
			t.Fatal(err)
		}
	}
	if !confirmed {
		t.Fatal("hostile goal helper did not confirm a reparse replacement")
	}
}

func goalHostileRestoreParent(parent, backup string) error {
	if err := os.Rename(backup, parent); err == nil {
		return nil
	}
	// A victim write may recreate the missing trusted directory during the
	// short interval between removing the hostile entry and restoring the
	// original. Preserve that disposable directory under a unique name rather
	// than deleting it or letting it block restoration.
	info, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("restore trusted goal parent: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("restore trusted goal parent: destination is not a recreated directory")
	}
	recreated := fmt.Sprintf("%s-recreated-%d", parent, time.Now().UnixNano())
	// Windows can briefly reject a directory rename while the victim is
	// releasing a handle opened during the hostile interval. Retry only this
	// disposable-directory handoff; do not delete the directory or shorten the
	// evidence window just to make restoration appear successful.
	if err := goalHostileRenameWithRetry(parent, recreated); err != nil {
		return fmt.Errorf("preserve recreated goal parent: %w", err)
	}
	if err := goalHostileRenameWithRetry(backup, parent); err != nil {
		return fmt.Errorf("restore trusted goal parent: %w", err)
	}
	return nil
}

func goalHostileRenameWithRetry(oldPath, newPath string) error {
	deadline := time.Now().Add(5 * time.Second)
	var err error
	for {
		if err = os.Rename(oldPath, newPath); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(time.Millisecond)
	}
}

func goalHostileWaitForPath(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func goalHostileTreeSHA256(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			return "", fmt.Errorf("outside evidence tree contains directory %q", entry.Name())
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	digest := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		_, _ = digest.Write([]byte(name))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(data)
		_, _ = digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func goalHostileSourceState() (string, bool) {
	shaOutput, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "UNRECORDED", false
	}
	statusOutput, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return strings.TrimSpace(string(shaOutput)), false
	}
	return strings.TrimSpace(string(shaOutput)), len(bytes.TrimSpace(statusOutput)) != 0
}

func goalHostileWriteEvidence(path string, evidence goalHostileEvidence) error {
	if !filepath.IsAbs(path) {
		return errors.New("hostile goal evidence path must be absolute")
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
