//go:build windows

package workspace_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/workspace"
)

const (
	windowsWorkspaceHostileHelperEnv   = "PICOGENT_WINDOWS_WORKSPACE_HOSTILE_HELPER"
	windowsWorkspaceHostileParentEnv   = "PICOGENT_WINDOWS_WORKSPACE_HOSTILE_PARENT"
	windowsWorkspaceHostileBackupEnv   = "PICOGENT_WINDOWS_WORKSPACE_HOSTILE_BACKUP"
	windowsWorkspaceHostileOutsideEnv  = "PICOGENT_WINDOWS_WORKSPACE_HOSTILE_OUTSIDE"
	windowsWorkspaceHostileReadyEnv    = "PICOGENT_WINDOWS_WORKSPACE_HOSTILE_READY"
	windowsWorkspaceHostileReleaseEnv  = "PICOGENT_WINDOWS_WORKSPACE_HOSTILE_RELEASE"
	windowsWorkspaceHostileStopEnv     = "PICOGENT_WINDOWS_WORKSPACE_HOSTILE_STOP"
	windowsWorkspaceHostileSwapsEnv    = "PICOGENT_WINDOWS_WORKSPACE_HOSTILE_SWAPS"
	windowsWorkspaceHostileSourceEnv   = "PICOGENT_WINDOWS_WORKSPACE_HOSTILE_SOURCE_SHA"
	windowsWorkspaceHostileEvidenceEnv = "PICOGENT_WINDOWS_WORKSPACE_HOSTILE_EVIDENCE_OUT"
	windowsWorkspaceHostileAttempts    = 128
	windowsWorkspaceHostilePause       = 250 * time.Microsecond
)

type windowsWorkspaceHostileEvidence struct {
	Schema                string                                     `json:"schema"`
	CandidateSHA          string                                     `json:"candidate_sha"`
	OS                    string                                     `json:"os"`
	Architecture          string                                     `json:"architecture"`
	Environment           string                                     `json:"environment"`
	Package               string                                     `json:"package"`
	AttackerConfirmed     bool                                       `json:"attacker_confirmed"`
	OutsideTreeBefore     string                                     `json:"outside_tree_before_sha256"`
	OutsideTreeAfter      string                                     `json:"outside_tree_after_sha256"`
	OutsideEscapeObserved bool                                       `json:"outside_escape_observed"`
	Operations            []windowsWorkspaceHostileOperationEvidence `json:"operations"`
	Verdict               string                                     `json:"verdict"`
	ObservedAt            string                                     `json:"observed_at"`
	BroadTOCTOUClaim      string                                     `json:"broad_toctou_claim"`
	SourceTreeModified    bool                                       `json:"source_tree_modified"`
}

type windowsWorkspaceHostileOperationEvidence struct {
	ID             string `json:"id"`
	Attempts       int    `json:"attempts"`
	Successes      int    `json:"successes"`
	Errors         int    `json:"errors"`
	AttackerSwaps  int    `json:"attacker_swaps"`
	EscapeObserved bool   `json:"escape_observed"`
	Verdict        string `json:"verdict"`
}

func TestWindowsSameUIDWorkspaceParentSwapConfinement(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only same-UID workspace parent-replacement harness")
	}
	if os.Getenv(windowsWorkspaceHostileHelperEnv) == "1" {
		windowsWorkspaceHostileParentSwapHelper(t)
		return
	}
	if err := windowsWorkspaceHostileReparseProbe(t); err != nil {
		if strings.TrimSpace(os.Getenv(windowsWorkspaceHostileEvidenceEnv)) != "" {
			t.Fatalf("junction support is required in evidence mode: %v", err)
		}
		t.Skipf("junction support unavailable: %v", err)
	}

	candidateSHA, sourceTreeModified := windowsWorkspaceHostileSourceState()
	if expected := strings.TrimSpace(os.Getenv(windowsWorkspaceHostileSourceEnv)); expected != "" && expected != candidateSHA {
		t.Fatalf("source SHA mismatch: expected %s, current HEAD is %s", expected, candidateSHA)
	}
	if sourceTreeModified && strings.TrimSpace(os.Getenv(windowsWorkspaceHostileEvidenceEnv)) != "" {
		t.Fatalf("source tree must be clean before retaining hostile evidence")
	}

	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	workspaceRoot := filepath.Join(root, "workspace")
	nested := filepath.Join(workspaceRoot, "nested")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"seed.txt":          "outside-seed\n",
		"write-target.txt":  "outside-write\n",
		"remove-target.txt": "outside-remove\n",
		"sentinel.txt":      "outside-sentinel\n",
	} {
		if err := os.WriteFile(filepath.Join(outside, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := workspace.WriteAtomic(workspaceRoot, "nested/seed.txt", []byte("inside-seed\n")); err != nil {
		t.Fatal(err)
	}
	beforeTree, err := windowsWorkspaceHostileTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}

	operations := []struct {
		id     string
		prep   func() error
		run    func() (successes, failures int, escaped bool)
		verify func() error
	}{
		{
			id: "workspace-write-atomic",
			run: func() (int, int, bool) {
				successes, failures := 0, 0
				for i := 0; i < windowsWorkspaceHostileAttempts; i++ {
					err := workspace.WriteAtomic(workspaceRoot, "nested/write-target.txt", []byte(fmt.Sprintf("inside-write-%d\n", i)))
					if err == nil {
						successes++
					} else {
						failures++
					}
					if mutated, checkErr := windowsWorkspaceHostileTreeMutated(outside, beforeTree); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						return successes, failures, true
					}
				}
				return successes, failures, false
			},
			verify: func() error {
				if err := workspace.WriteAtomic(workspaceRoot, "nested/write-target.txt", []byte("inside-write-verified\n")); err != nil {
					return fmt.Errorf("trusted workspace write: %w", err)
				}
				data, err := os.ReadFile(filepath.Join(nested, "write-target.txt"))
				if err != nil {
					return fmt.Errorf("read trusted workspace write: %w", err)
				}
				if string(data) != "inside-write-verified\n" {
					return fmt.Errorf("trusted workspace write returned %q", data)
				}
				return nil
			},
		},
		{
			id: "workspace-open-read",
			run: func() (int, int, bool) {
				successes, failures := 0, 0
				for i := 0; i < windowsWorkspaceHostileAttempts; i++ {
					file, err := workspace.OpenRead(workspaceRoot, "nested/seed.txt")
					if err != nil {
						failures++
					} else {
						data, readErr := io.ReadAll(file)
						_ = file.Close()
						switch {
						case readErr != nil:
							failures++
						case string(data) == "inside-seed\n":
							successes++
						case strings.HasPrefix(string(data), "outside"):
							return successes, failures, true
						default:
							failures++
						}
					}
					if mutated, checkErr := windowsWorkspaceHostileTreeMutated(outside, beforeTree); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						return successes, failures, true
					}
				}
				return successes, failures, false
			},
			verify: func() error {
				file, err := workspace.OpenRead(workspaceRoot, "nested/seed.txt")
				if err != nil {
					return fmt.Errorf("trusted workspace read: %w", err)
				}
				data, readErr := io.ReadAll(file)
				closeErr := file.Close()
				if readErr != nil || closeErr != nil {
					return errors.Join(readErr, closeErr)
				}
				if string(data) != "inside-seed\n" {
					return fmt.Errorf("trusted workspace read returned %q", data)
				}
				return nil
			},
		},
		{
			id: "workspace-remove",
			prep: func() error {
				return os.WriteFile(filepath.Join(nested, "remove-target.txt"), []byte("inside-remove\n"), 0o600)
			},
			run: func() (int, int, bool) {
				successes, failures := 0, 0
				for i := 0; i < windowsWorkspaceHostileAttempts; i++ {
					err := workspace.Remove(workspaceRoot, "nested/remove-target.txt")
					if err == nil {
						successes++
					} else {
						failures++
					}
					if mutated, checkErr := windowsWorkspaceHostileTreeMutated(outside, beforeTree); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						return successes, failures, true
					}
				}
				return successes, failures, false
			},
			verify: func() error {
				path := filepath.Join(nested, "remove-target.txt")
				if err := os.WriteFile(path, []byte("inside-remove-verified\n"), 0o600); err != nil {
					return fmt.Errorf("prepare trusted workspace remove: %w", err)
				}
				if err := workspace.Remove(workspaceRoot, "nested/remove-target.txt"); err != nil {
					return fmt.Errorf("trusted workspace remove: %w", err)
				}
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("trusted workspace remove target remained: %v", err)
				}
				return nil
			},
		},
	}

	operationsEvidence := make([]windowsWorkspaceHostileOperationEvidence, 0, len(operations))
	anyEscape := false
	attackerConfirmed := false
	for _, operation := range operations {
		if operation.prep != nil {
			if err := operation.prep(); err != nil {
				t.Fatalf("%s prep: %v", operation.id, err)
			}
		}
		parent := nested
		backup := nested + "-real"
		ready := filepath.Join(root, operation.id+"-ready")
		release := filepath.Join(root, operation.id+"-release")
		stop := filepath.Join(root, operation.id+"-stop")
		swapsPath := filepath.Join(root, operation.id+"-swaps")
		cmd := exec.Command(os.Args[0], "-test.run", "^TestWindowsSameUIDWorkspaceParentSwapAttackerHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			windowsWorkspaceHostileHelperEnv+"=1",
			windowsWorkspaceHostileParentEnv+"="+parent,
			windowsWorkspaceHostileBackupEnv+"="+backup,
			windowsWorkspaceHostileOutsideEnv+"="+outside,
			windowsWorkspaceHostileReadyEnv+"="+ready,
			windowsWorkspaceHostileReleaseEnv+"="+release,
			windowsWorkspaceHostileStopEnv+"="+stop,
			windowsWorkspaceHostileSwapsEnv+"="+swapsPath,
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
			restoreErr := windowsWorkspaceHostileRestoreParent(parent, backup)
			return errors.Join(waitErr, restoreErr)
		}
		cleaned := false
		defer func() {
			if !cleaned {
				_ = cleanup()
			}
		}()

		windowsWorkspaceHostileWaitForPath(t, ready)
		if err := os.WriteFile(release, []byte("go\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		windowsWorkspaceHostileWaitForPath(t, swapsPath)
		successes, failures, escaped := operation.run()
		time.Sleep(50 * time.Millisecond)
		if err := cleanup(); err != nil {
			t.Fatalf("attacker for %s failed: %v\nstdout=%q\nstderr=%q", operation.id, err, stdout.String(), stderr.String())
		}
		cleaned = true
		swaps := windowsWorkspaceHostileSwapCount(t, swapsPath)
		if swaps < 1 {
			t.Fatalf("%s observed no confirmed attacker swaps", operation.id)
		}
		attackerConfirmed = true
		if escaped {
			anyEscape = true
		}
		if operation.verify != nil {
			if err := operation.verify(); err != nil {
				t.Fatalf("%s trusted in-tree effect: %v", operation.id, err)
			}
		}
		verdict := "PASS"
		if escaped {
			verdict = "FAIL"
		}
		operationsEvidence = append(operationsEvidence, windowsWorkspaceHostileOperationEvidence{
			ID:             operation.id,
			Attempts:       windowsWorkspaceHostileAttempts,
			Successes:      successes,
			Errors:         failures,
			AttackerSwaps:  swaps,
			EscapeObserved: escaped,
			Verdict:        verdict,
		})
		if escaped {
			t.Fatalf("%s escaped into the outside workspace fixture", operation.id)
		}
	}

	afterTree, err := windowsWorkspaceHostileTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}
	if afterTree != beforeTree {
		anyEscape = true
		t.Fatalf("outside workspace tree mutated: before=%s after=%s", beforeTree, afterTree)
	}
	overall := "PASS"
	if anyEscape {
		overall = "FAIL"
	}
	if !attackerConfirmed {
		overall = "INCONCLUSIVE"
	}
	evidence := windowsWorkspaceHostileEvidence{
		Schema:                "picogent.v4.windows-workspace-parent-replacement-evidence.v1",
		CandidateSHA:          candidateSHA,
		OS:                    runtime.GOOS,
		Architecture:          runtime.GOARCH,
		Environment:           "task-owned-disposable",
		Package:               "workspace",
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
	outPath := filepath.Join(root, "windows-workspace-parent-replacement-evidence.json")
	if err := os.WriteFile(outPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if destination := strings.TrimSpace(os.Getenv(windowsWorkspaceHostileEvidenceEnv)); destination != "" {
		if err := os.WriteFile(destination, payload, 0o600); err != nil {
			t.Fatalf("retain evidence: %v", err)
		}
	}
	t.Logf("windows workspace parent-replacement evidence verdict=%s path=%s", overall, outPath)
	if overall != "PASS" {
		t.Fatalf("windows workspace parent-replacement verdict=%s", overall)
	}
}

func TestWindowsSameUIDWorkspaceParentSwapAttackerHelper(t *testing.T) {
	if os.Getenv(windowsWorkspaceHostileHelperEnv) != "1" {
		return
	}
	windowsWorkspaceHostileParentSwapHelper(t)
}

func windowsWorkspaceHostileParentSwapHelper(t *testing.T) {
	parent := os.Getenv(windowsWorkspaceHostileParentEnv)
	backup := os.Getenv(windowsWorkspaceHostileBackupEnv)
	outside := os.Getenv(windowsWorkspaceHostileOutsideEnv)
	ready := os.Getenv(windowsWorkspaceHostileReadyEnv)
	release := os.Getenv(windowsWorkspaceHostileReleaseEnv)
	stop := os.Getenv(windowsWorkspaceHostileStopEnv)
	swapsPath := os.Getenv(windowsWorkspaceHostileSwapsEnv)
	if parent == "" || backup == "" || outside == "" || ready == "" || release == "" || stop == "" || swapsPath == "" {
		t.Fatal("hostile Windows workspace helper environment is incomplete")
	}
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(stop); err == nil {
			break
		}
		if err := os.Rename(parent, backup); err != nil {
			time.Sleep(time.Millisecond)
			continue
		}
		if err := windowsWorkspaceHostileCreateJunction(parent, outside); err != nil {
			_ = os.Rename(backup, parent)
			time.Sleep(time.Millisecond)
			continue
		}
		if err := os.WriteFile(swapsPath, []byte("confirmed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(windowsWorkspaceHostilePause)
		if err := os.Remove(parent); err != nil {
			t.Fatal(fmt.Errorf("remove hostile workspace junction: %w", err))
		}
		if err := windowsWorkspaceHostileRestoreParent(parent, backup); err != nil {
			t.Fatal(fmt.Errorf("restore trusted workspace parent: %w", err))
		}
		time.Sleep(windowsWorkspaceHostilePause)
	}
	if _, err := os.Stat(parent); errors.Is(err, os.ErrNotExist) {
		if err := windowsWorkspaceHostileRestoreParent(parent, backup); err != nil {
			t.Fatal(err)
		}
	}
}

func windowsWorkspaceHostileReparseProbe(t *testing.T) error {
	t.Helper()
	root := t.TempDir()
	link := filepath.Join(root, "junction")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	if err := windowsWorkspaceHostileCreateJunction(link, target); err != nil {
		return err
	}
	return os.Remove(link)
}

func windowsWorkspaceHostileCreateJunction(link, target string) error {
	output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J %q -> %q: %w (%s)", link, target, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func windowsWorkspaceHostileRestoreParent(parent, backup string) error {
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
		return fmt.Errorf("hostile workspace parent replacement is not a directory: %v", info.Mode())
	}
	return fmt.Errorf("restore trusted workspace parent: %w", lastErr)
}

func windowsWorkspaceHostileWaitForPath(t *testing.T, path string) {
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

func windowsWorkspaceHostileSwapCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "confirmed" {
		t.Fatalf("unexpected hostile workspace swap marker %q", data)
	}
	return 1
}

func windowsWorkspaceHostileTreeMutated(root, before string) (bool, error) {
	after, err := windowsWorkspaceHostileTreeSHA256(root)
	if err != nil {
		return false, err
	}
	return after != before, nil
}

func windowsWorkspaceHostileTreeSHA256(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	digest := sha256.New()
	for _, entry := range entries {
		if entry.IsDir() {
			return "", fmt.Errorf("outside workspace evidence tree contains directory %q", entry.Name())
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

func windowsWorkspaceHostileSourceState() (string, bool) {
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
