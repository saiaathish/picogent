//go:build windows

package securefile_test

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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/securefile"
)

const (
	windowsHostileHelperEnv   = "PICOGENT_WINDOWS_HOSTILE_PARENT_SWAP_HELPER"
	windowsHostileParentEnv   = "PICOGENT_WINDOWS_HOSTILE_PARENT"
	windowsHostileBackupEnv   = "PICOGENT_WINDOWS_HOSTILE_BACKUP"
	windowsHostileOutsideEnv  = "PICOGENT_WINDOWS_HOSTILE_OUTSIDE"
	windowsHostileReadyEnv    = "PICOGENT_WINDOWS_HOSTILE_READY"
	windowsHostileReleaseEnv  = "PICOGENT_WINDOWS_HOSTILE_RELEASE"
	windowsHostileStopEnv     = "PICOGENT_WINDOWS_HOSTILE_STOP"
	windowsHostileSwapsEnv    = "PICOGENT_WINDOWS_HOSTILE_SWAPS"
	windowsHostileSourceEnv   = "PICOGENT_WINDOWS_HOSTILE_SOURCE_SHA"
	windowsHostileEvidenceEnv = "PICOGENT_WINDOWS_HOSTILE_EVIDENCE_OUT"
	windowsHostileAttempts    = 128
	windowsHostilePause       = 250 * time.Microsecond
)

type windowsHostileEvidence struct {
	Schema                string                            `json:"schema"`
	CandidateSHA          string                            `json:"candidate_sha"`
	OS                    string                            `json:"os"`
	Architecture          string                            `json:"architecture"`
	Environment           string                            `json:"environment"`
	Package               string                            `json:"package"`
	AttackerConfirmed     bool                              `json:"attacker_confirmed"`
	OutsideTreeBefore     string                            `json:"outside_tree_before_sha256"`
	OutsideTreeAfter      string                            `json:"outside_tree_after_sha256"`
	OutsideEscapeObserved bool                              `json:"outside_escape_observed"`
	Operations            []windowsHostileOperationEvidence `json:"operations"`
	Verdict               string                            `json:"verdict"`
	ObservedAt            string                            `json:"observed_at"`
	BroadTOCTOUClaim      string                            `json:"broad_toctou_claim"`
	SourceTreeModified    bool                              `json:"source_tree_modified"`
}

type windowsHostileOperationEvidence struct {
	ID             string `json:"id"`
	Attempts       int    `json:"attempts"`
	Successes      int    `json:"successes"`
	Errors         int    `json:"errors"`
	AttackerSwaps  int    `json:"attacker_swaps"`
	EscapeObserved bool   `json:"escape_observed"`
	Verdict        string `json:"verdict"`
}

func TestWindowsSameUIDParentSwapConfinement(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only same-UID parent-replacement harness")
	}
	if os.Getenv(windowsHostileHelperEnv) == "1" {
		windowsHostileParentSwapHelper(t)
		return
	}
	if err := windowsHostileReparseProbe(t); err != nil {
		if strings.TrimSpace(os.Getenv(windowsHostileEvidenceEnv)) != "" {
			t.Fatalf("junction support is required in evidence mode: %v", err)
		}
		t.Skipf("junction support unavailable: %v", err)
	}

	candidateSHA, sourceTreeModified := windowsHostileSourceState()
	if expected := strings.TrimSpace(os.Getenv(windowsHostileSourceEnv)); expected != "" && expected != candidateSHA {
		t.Fatalf("source SHA mismatch: expected %s, current HEAD is %s", expected, candidateSHA)
	}
	if sourceTreeModified && strings.TrimSpace(os.Getenv(windowsHostileEvidenceEnv)) != "" {
		t.Fatalf("source tree must be clean before retaining hostile evidence")
	}

	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	trustedRoot := filepath.Join(root, "trusted")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(trustedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "state.yaml"), []byte("outside-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "remove-target.txt"), []byte("outside-remove\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "sentinel.txt"), []byte("outside-sentinel\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeTree, err := windowsHostileTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}

	operations := []struct {
		id     string
		prep   func(string) error
		run    func(string) (successes, failures int, escaped bool)
		verify func(string) error
	}{
		{
			id: "securefile-write-atomic",
			run: func(parent string) (int, int, bool) {
				successes, failures := 0, 0
				for i := 0; i < windowsHostileAttempts; i++ {
					err := securefile.WriteAtomic(filepath.Join(parent, "state.yaml"), []byte(fmt.Sprintf("inside-%d\n", i)), 0o600)
					if err == nil {
						successes++
					} else {
						failures++
					}
					if mutated, checkErr := windowsHostileTreeMutated(outside, beforeTree); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						return successes, failures, true
					}
				}
				return successes, failures, false
			},
			verify: func(parent string) error {
				if err := securefile.WriteAtomic(filepath.Join(parent, "state.yaml"), []byte("inside-verified\n"), 0o600); err != nil {
					return fmt.Errorf("trusted atomic write: %w", err)
				}
				data, err := os.ReadFile(filepath.Join(parent, "state.yaml"))
				if err != nil {
					return fmt.Errorf("read trusted atomic write: %w", err)
				}
				if string(data) != "inside-verified\n" {
					return fmt.Errorf("trusted atomic write returned %q", data)
				}
				return nil
			},
		},
		{
			id: "securefile-read-file",
			prep: func(parent string) error {
				return os.WriteFile(filepath.Join(parent, "state.yaml"), []byte("inside\n"), 0o600)
			},
			run: func(parent string) (int, int, bool) {
				successes, failures := 0, 0
				for i := 0; i < windowsHostileAttempts; i++ {
					data, err := securefile.ReadFile(filepath.Join(parent, "state.yaml"))
					switch {
					case err != nil:
						failures++
					case string(data) == "inside\n":
						successes++
					case strings.HasPrefix(string(data), "outside"):
						return successes, failures, true
					default:
						failures++
					}
					if mutated, checkErr := windowsHostileTreeMutated(outside, beforeTree); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						return successes, failures, true
					}
				}
				return successes, failures, false
			},
			verify: func(parent string) error {
				data, err := securefile.ReadFile(filepath.Join(parent, "state.yaml"))
				if err != nil {
					return fmt.Errorf("trusted read: %w", err)
				}
				if string(data) != "inside\n" {
					return fmt.Errorf("trusted read returned %q", data)
				}
				return nil
			},
		},
		{
			id: "securefile-write-exclusive",
			run: func(parent string) (int, int, bool) {
				successes, failures := 0, 0
				for i := 0; i < windowsHostileAttempts; i++ {
					name := filepath.Join(parent, fmt.Sprintf("exclusive-%d.json", i))
					err := securefile.WriteExclusive(name, []byte(fmt.Sprintf("inside-%d\n", i)), 0o600)
					if err == nil {
						successes++
					} else {
						failures++
					}
					if mutated, checkErr := windowsHostileTreeMutated(outside, beforeTree); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						return successes, failures, true
					}
				}
				return successes, failures, false
			},
			verify: func(parent string) error {
				name := filepath.Join(parent, "exclusive-verified.json")
				if err := securefile.WriteExclusive(name, []byte("exclusive-verified\n"), 0o600); err != nil {
					return fmt.Errorf("trusted exclusive write: %w", err)
				}
				data, err := os.ReadFile(name)
				if err != nil {
					return fmt.Errorf("read trusted exclusive write: %w", err)
				}
				if string(data) != "exclusive-verified\n" {
					return fmt.Errorf("trusted exclusive write returned %q", data)
				}
				return nil
			},
		},
		{
			id: "securefile-remove-file",
			prep: func(parent string) error {
				return os.WriteFile(filepath.Join(parent, "remove-target.txt"), []byte("inside-remove\n"), 0o600)
			},
			run: func(parent string) (int, int, bool) {
				successes, failures := 0, 0
				for i := 0; i < windowsHostileAttempts; i++ {
					err := securefile.RemoveFile(filepath.Join(parent, "remove-target.txt"))
					if err == nil {
						successes++
					} else {
						failures++
					}
					if mutated, checkErr := windowsHostileTreeMutated(outside, beforeTree); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						return successes, failures, true
					}
				}
				return successes, failures, false
			},
			verify: func(parent string) error {
				name := filepath.Join(parent, "remove-target.txt")
				if err := os.WriteFile(name, []byte("inside-remove-verified\n"), 0o600); err != nil {
					return fmt.Errorf("prepare trusted remove: %w", err)
				}
				if err := securefile.RemoveFile(name); err != nil {
					return fmt.Errorf("trusted remove: %w", err)
				}
				if _, err := os.Lstat(name); !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("trusted remove target remained: %v", err)
				}
				return nil
			},
		},
	}

	operationsEvidence := make([]windowsHostileOperationEvidence, 0, len(operations))
	anyEscape := false
	attackerConfirmed := false
	for _, operation := range operations {
		parent := filepath.Join(trustedRoot, operation.id)
		backup := parent + "-real"
		if err := os.MkdirAll(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		if operation.prep != nil {
			if err := operation.prep(parent); err != nil {
				t.Fatalf("%s prep: %v", operation.id, err)
			}
		}

		ready := filepath.Join(root, operation.id+"-ready")
		release := filepath.Join(root, operation.id+"-release")
		stop := filepath.Join(root, operation.id+"-stop")
		swapsPath := filepath.Join(root, operation.id+"-swaps")
		cmd := exec.Command(os.Args[0], "-test.run", "^TestWindowsSameUIDParentSwapAttackerHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			windowsHostileHelperEnv+"=1",
			windowsHostileParentEnv+"="+parent,
			windowsHostileBackupEnv+"="+backup,
			windowsHostileOutsideEnv+"="+outside,
			windowsHostileReadyEnv+"="+ready,
			windowsHostileReleaseEnv+"="+release,
			windowsHostileStopEnv+"="+stop,
			windowsHostileSwapsEnv+"="+swapsPath,
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start attacker for %s: %v", operation.id, err)
		}
		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()
		waited := false
		var waitErr error
		waitForAttackerExit := func() error {
			if waited {
				return waitErr
			}
			timer := time.NewTimer(10 * time.Second)
			defer timer.Stop()
			select {
			case waitErr = <-waitCh:
				waited = true
			case <-timer.C:
				_ = cmd.Process.Kill()
				waitErr = <-waitCh
				waitErr = fmt.Errorf("attacker did not stop within 10s: %w", waitErr)
				waited = true
			}
			return waitErr
		}
		cleanup := func() error {
			_ = os.WriteFile(release, []byte("go\n"), 0o600)
			_ = os.WriteFile(stop, []byte("stop\n"), 0o600)
			waitErr := waitForAttackerExit()
			var restoreErr error
			if _, backupErr := os.Lstat(backup); backupErr == nil {
				restoreErr = windowsHostileRestoreParent(parent, backup)
			} else if !errors.Is(backupErr, os.ErrNotExist) {
				restoreErr = backupErr
			}
			return errors.Join(waitErr, restoreErr)
		}
		cleaned := false
		defer func() {
			if !cleaned {
				_ = cleanup()
			}
		}()

		windowsHostileWaitForPath(t, ready)
		if err := os.WriteFile(release, []byte("go\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		windowsHostileWaitForPath(t, swapsPath)
		successes, failures, escaped := operation.run(parent)
		time.Sleep(50 * time.Millisecond)
		if err := cleanup(); err != nil {
			t.Fatalf("attacker for %s failed: %v\nstdout=%q\nstderr=%q", operation.id, err, stdout.String(), stderr.String())
		}
		cleaned = true
		swaps := windowsHostileSwapCount(t, swapsPath)
		if swaps < 1 {
			t.Fatalf("%s observed no confirmed attacker swaps", operation.id)
		}
		attackerConfirmed = true
		if escaped {
			anyEscape = true
		}
		if operation.verify != nil {
			if err := operation.verify(parent); err != nil {
				t.Fatalf("%s trusted in-tree effect: %v", operation.id, err)
			}
		}
		verdict := "PASS"
		if escaped {
			verdict = "FAIL"
		}
		operationsEvidence = append(operationsEvidence, windowsHostileOperationEvidence{
			ID:             operation.id,
			Attempts:       windowsHostileAttempts,
			Successes:      successes,
			Errors:         failures,
			AttackerSwaps:  swaps,
			EscapeObserved: escaped,
			Verdict:        verdict,
		})
		if escaped {
			t.Fatalf("%s escaped into the outside fixture", operation.id)
		}
	}

	afterTree, err := windowsHostileTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}
	if afterTree != beforeTree {
		anyEscape = true
		t.Fatalf("outside tree mutated: before=%s after=%s", beforeTree, afterTree)
	}
	overall := "PASS"
	if anyEscape {
		overall = "FAIL"
	}
	if !attackerConfirmed {
		overall = "INCONCLUSIVE"
	}

	evidence := windowsHostileEvidence{
		Schema:                "picogent.v4.windows-hostile-parent-replacement-evidence.v1",
		CandidateSHA:          candidateSHA,
		OS:                    runtime.GOOS,
		Architecture:          runtime.GOARCH,
		Environment:           "task-owned-disposable",
		Package:               "securefile",
		AttackerConfirmed:     attackerConfirmed,
		OutsideTreeBefore:     beforeTree,
		OutsideTreeAfter:      afterTree,
		OutsideEscapeObserved: anyEscape,
		Operations:            operationsEvidence,
		Verdict:               overall,
		ObservedAt:            time.Now().UTC().Format(time.RFC3339),
		BroadTOCTOUClaim:      "UNVERIFIED",
		SourceTreeModified:    sourceTreeModified,
	}
	payload, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	outPath := filepath.Join(root, "windows-hostile-parent-replacement-evidence.json")
	if err := os.WriteFile(outPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if destination := strings.TrimSpace(os.Getenv(windowsHostileEvidenceEnv)); destination != "" {
		if err := os.WriteFile(destination, payload, 0o600); err != nil {
			t.Fatalf("retain evidence: %v", err)
		}
	}
	t.Logf("windows hostile parent-replacement evidence verdict=%s path=%s", overall, outPath)
	if overall != "PASS" {
		t.Fatalf("windows hostile parent-replacement verdict=%s", overall)
	}
}

func TestWindowsSameUIDParentSwapAttackerHelper(t *testing.T) {
	if os.Getenv(windowsHostileHelperEnv) != "1" {
		return
	}
	windowsHostileParentSwapHelper(t)
}

func windowsHostileParentSwapHelper(t *testing.T) {
	parent := os.Getenv(windowsHostileParentEnv)
	backup := os.Getenv(windowsHostileBackupEnv)
	outside := os.Getenv(windowsHostileOutsideEnv)
	ready := os.Getenv(windowsHostileReadyEnv)
	release := os.Getenv(windowsHostileReleaseEnv)
	stop := os.Getenv(windowsHostileStopEnv)
	swapsPath := os.Getenv(windowsHostileSwapsEnv)
	if parent == "" || backup == "" || outside == "" || ready == "" || release == "" || stop == "" || swapsPath == "" {
		t.Fatal("hostile Windows helper environment is incomplete")
	}
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	swaps := 0
	for {
		if _, err := os.Stat(stop); err == nil {
			break
		}
		if err := os.Rename(parent, backup); err != nil {
			time.Sleep(time.Millisecond)
			continue
		}
		if err := windowsHostileCreateJunction(parent, outside); err != nil {
			_ = os.Rename(backup, parent)
			time.Sleep(time.Millisecond)
			continue
		}
		swaps++
		if err := os.WriteFile(swapsPath, []byte(strconv.Itoa(swaps)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(windowsHostilePause)
		if err := os.Remove(parent); err != nil {
			t.Fatal(fmt.Errorf("remove hostile junction: %w", err))
		}
		if err := windowsHostileRestoreParent(parent, backup); err != nil {
			t.Fatal(fmt.Errorf("restore trusted parent: %w", err))
		}
		time.Sleep(windowsHostilePause)
	}
	if err := os.WriteFile(swapsPath, []byte(strconv.Itoa(swaps)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(parent); errors.Is(err, os.ErrNotExist) {
		if err := windowsHostileRestoreParent(parent, backup); err != nil {
			t.Fatal(err)
		}
	}
}

func windowsHostileReparseProbe(t *testing.T) error {
	t.Helper()
	root := t.TempDir()
	link := filepath.Join(root, "junction")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	if err := windowsHostileCreateJunction(link, target); err != nil {
		return err
	}
	return os.Remove(link)
}

func windowsHostileCreateJunction(link, target string) error {
	output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J %q -> %q: %w (%s)", link, target, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func windowsHostileRestoreParent(parent, backup string) error {
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := os.Rename(backup, parent); err == nil {
			return nil
		} else {
			lastErr = err
		}
		info, err := os.Lstat(parent)
		if errors.Is(err, os.ErrNotExist) {
			time.Sleep(time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
		recreated := fmt.Sprintf("%s-recreated-%d", parent, time.Now().UnixNano())
		if err := os.Rename(parent, recreated); err != nil {
			lastErr = err
			time.Sleep(time.Millisecond)
			continue
		}
		if info.IsDir() {
			continue
		}
		return fmt.Errorf("hostile parent replacement is not a directory: %v", info.Mode())
	}
	return fmt.Errorf("restore trusted parent: %w", lastErr)
}

func windowsHostileWaitForPath(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func windowsHostileSwapCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	swaps, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || swaps < 1 {
		t.Fatalf("unexpected hostile swap count %q", data)
	}
	return swaps
}

func windowsHostileTreeMutated(root, before string) (bool, error) {
	after, err := windowsHostileTreeSHA256(root)
	if err != nil {
		return false, err
	}
	return after != before, nil
}

func windowsHostileTreeSHA256(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	digest := sha256.New()
	for _, entry := range entries {
		if entry.IsDir() {
			return "", fmt.Errorf("outside evidence tree contains directory %q", entry.Name())
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, name))
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

func windowsHostileSourceState() (string, bool) {
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
