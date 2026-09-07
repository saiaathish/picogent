//go:build linux

package checkpoint_test

import (
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

	"github.com/saiaathish/picogent/internal/checkpoint"
)

const (
	hostileCheckpointSwapHelperEnv   = "PICOGENT_HOSTILE_CHECKPOINT_SWAP_HELPER"
	hostileCheckpointSwapEvidenceEnv = "PICOGENT_HOSTILE_PARENT_SWAP_EVIDENCE_OUT"
	hostileCheckpointSwapSourceEnv   = "PICOGENT_HOSTILE_PARENT_SWAP_SOURCE_SHA"
	hostileCheckpointSwapAttempts    = 200
)

type hostileCheckpointSwapEvidence struct {
	Schema                string                            `json:"schema"`
	CandidateSHA          string                            `json:"candidate_sha"`
	OS                    string                            `json:"os"`
	Architecture          string                            `json:"architecture"`
	Environment           string                            `json:"environment"`
	Package               string                            `json:"package"`
	AttackerConfirmed     bool                              `json:"attacker_confirmed"`
	OutsideSentinelBefore string                            `json:"outside_sentinel_before_sha256"`
	OutsideSentinelAfter  string                            `json:"outside_sentinel_after_sha256"`
	OutsideTreeBefore     string                            `json:"outside_tree_before_sha256"`
	OutsideTreeAfter      string                            `json:"outside_tree_after_sha256"`
	OutsideEscapeObserved bool                              `json:"outside_escape_observed"`
	Operations            []hostileCheckpointSwapOpEvidence `json:"operations"`
	Verdict               string                            `json:"verdict"`
	ObservedAt            string                            `json:"observed_at"`
	BroadTOCTOUClaim      string                            `json:"broad_toctou_claim"`
}

type hostileCheckpointSwapOpEvidence struct {
	ID             string `json:"id"`
	Attempts       int    `json:"attempts"`
	Successes      int    `json:"successes"`
	Errors         int    `json:"errors"`
	AttackerSwaps  int    `json:"attacker_swaps"`
	EscapeObserved bool   `json:"escape_observed"`
	Verdict        string `json:"verdict"`
}

func TestLinuxSameUIDCheckpointParentSwapConfinement(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only same-UID checkpoint parent-swap harness")
	}

	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	workspaceRoot := filepath.Join(root, "workspace")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinelPath := filepath.Join(outside, "sentinel.txt")
	sentinel := []byte("checkpoint-outside-sentinel\n")
	if err := os.WriteFile(sentinelPath, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "marker.txt"), []byte("outside-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeSentinel := sha256HexCheckpoint(sentinel)

	nested := filepath.Join(workspaceRoot, "nested")
	backup := nested + "-real"
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < hostileCheckpointSwapAttempts; i++ {
		name := fmt.Sprintf("remove-%d.txt", i)
		if err := os.WriteFile(filepath.Join(outside, name), []byte("outside-remove\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	beforeTree, err := checkpointOutsideTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}

	ops := []struct {
		id     string
		prep   func() error
		seal   func(t *testing.T) []*checkpoint.Checkpoint
		restore func(t *testing.T, sealed []*checkpoint.Checkpoint) (successes, errs int, escape bool)
	}{
		{
			id: "checkpoint-restore-existing",
			prep: func() error {
				return nil
			},
			seal: func(t *testing.T) []*checkpoint.Checkpoint {
				t.Helper()
				sealed := make([]*checkpoint.Checkpoint, 0, hostileCheckpointSwapAttempts)
				for i := 0; i < hostileCheckpointSwapAttempts; i++ {
					rel := fmt.Sprintf("nested/existing-%d.txt", i)
					if err := os.WriteFile(filepath.Join(nested, filepath.Base(rel)), []byte("before\n"), 0o600); err != nil {
						t.Fatal(err)
					}
					cp, err := checkpoint.Capture(workspaceRoot, []string{rel})
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(nested, filepath.Base(rel)), []byte("after\n"), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := cp.Seal(); err != nil {
						t.Fatal(err)
					}
					sealed = append(sealed, cp)
				}
				return sealed
			},
			restore: func(t *testing.T, sealed []*checkpoint.Checkpoint) (int, int, bool) {
				t.Helper()
				successes, errs, escape := 0, 0, false
				for _, cp := range sealed {
					result, err := cp.Restore()
					switch {
					case err == nil && result.Complete:
						successes++
					default:
						errs++
					}
					if mutated, checkErr := checkpointOutsideTreeMutated(outside, beforeTree); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						escape = true
						break
					}
				}
				return successes, errs, escape
			},
		},
		{
			id: "checkpoint-restore-remove-created",
			prep: func() error {
				return nil
			},
			seal: func(t *testing.T) []*checkpoint.Checkpoint {
				t.Helper()
				sealed := make([]*checkpoint.Checkpoint, 0, hostileCheckpointSwapAttempts)
				for i := 0; i < hostileCheckpointSwapAttempts; i++ {
					rel := fmt.Sprintf("nested/created-%d.txt", i)
					_ = os.Remove(filepath.Join(nested, fmt.Sprintf("created-%d.txt", i)))
					cp, err := checkpoint.Capture(workspaceRoot, []string{rel})
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(nested, fmt.Sprintf("created-%d.txt", i)), []byte("created\n"), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := cp.Seal(); err != nil {
						t.Fatal(err)
					}
					sealed = append(sealed, cp)
				}
				return sealed
			},
			restore: func(t *testing.T, sealed []*checkpoint.Checkpoint) (int, int, bool) {
				t.Helper()
				successes, errs, escape := 0, 0, false
				for _, cp := range sealed {
					result, err := cp.Restore()
					switch {
					case err == nil && result.Complete:
						successes++
					default:
						errs++
					}
					if mutated, checkErr := checkpointOutsideTreeMutated(outside, beforeTree); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						escape = true
						break
					}
				}
				return successes, errs, escape
			},
		},
	}

	evidenceOps := make([]hostileCheckpointSwapOpEvidence, 0, len(ops))
	anyEscape := false
	attackerConfirmed := false

	for _, op := range ops {
		if err := op.prep(); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s prep: %v", op.id, err)
		}
		sealed := op.seal(t)

		ready := filepath.Join(root, op.id+"-ready")
		release := filepath.Join(root, op.id+"-release")
		stop := filepath.Join(root, op.id+"-stop")
		swapsPath := filepath.Join(root, op.id+"-swaps")
		_ = os.Remove(ready)
		_ = os.Remove(release)
		_ = os.Remove(stop)
		_ = os.Remove(swapsPath)

		cmd := exec.Command(os.Args[0], "-test.run", "^TestLinuxSameUIDCheckpointParentSwapAttackerHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			hostileCheckpointSwapHelperEnv+"=1",
			"PICOGENT_HOSTILE_PARENT="+nested,
			"PICOGENT_HOSTILE_BACKUP="+backup,
			"PICOGENT_HOSTILE_OUTSIDE="+outside,
			"PICOGENT_HOSTILE_READY="+ready,
			"PICOGENT_HOSTILE_RELEASE="+release,
			"PICOGENT_HOSTILE_STOP="+stop,
			"PICOGENT_HOSTILE_SWAPS="+swapsPath,
		)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start attacker for %s: %v", op.id, err)
		}
		waitForCheckpointFile(t, ready, 15*time.Second)
		if err := os.WriteFile(release, []byte("go\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		waitForCheckpointSwaps(t, swapsPath, 1, 30*time.Second)

		successes, errs, escape := op.restore(t, sealed)
		time.Sleep(50 * time.Millisecond)
		if err := os.WriteFile(stop, []byte("stop\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if waitErr := cmd.Wait(); waitErr != nil {
			t.Fatalf("attacker for %s failed: %v\nstdout=%s\nstderr=%s", op.id, waitErr, stdout.String(), stderr.String())
		}

		swaps := readCheckpointSwapCount(t, swapsPath)
		if swaps < 1 {
			t.Fatalf("%s observed no confirmed attacker swaps", op.id)
		}
		attackerConfirmed = true
		if escape {
			anyEscape = true
		}
		verdict := "PASS"
		if escape {
			verdict = "FAIL"
		}
		evidenceOps = append(evidenceOps, hostileCheckpointSwapOpEvidence{
			ID:             op.id,
			Attempts:       hostileCheckpointSwapAttempts,
			Successes:      successes,
			Errors:         errs,
			AttackerSwaps:  swaps,
			EscapeObserved: escape,
			Verdict:        verdict,
		})
		if escape {
			t.Fatalf("%s escaped into outside sentinel directory", op.id)
		}

		if info, err := os.Lstat(nested); err == nil && info.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(nested)
		}
		if _, err := os.Lstat(nested); errors.Is(err, os.ErrNotExist) {
			_ = os.Rename(backup, nested)
		}
		_ = os.RemoveAll(backup)
		if err := os.RemoveAll(nested); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(nested, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	after, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatal(err)
	}
	afterSentinel := sha256HexCheckpoint(after)
	if afterSentinel != beforeSentinel {
		anyEscape = true
		t.Fatalf("outside sentinel mutated: before=%s after=%s", beforeSentinel, afterSentinel)
	}
	afterTree, err := checkpointOutsideTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}
	if afterTree != beforeTree {
		anyEscape = true
		t.Fatalf("outside tree mutated: before=%s after=%s", beforeTree, afterTree)
	}

	overall := "PASS"
	for _, op := range evidenceOps {
		if op.Verdict != "PASS" {
			overall = op.Verdict
			break
		}
	}
	if anyEscape {
		overall = "FAIL"
	}
	if !attackerConfirmed {
		overall = "INCONCLUSIVE"
	}

	sourceSHA := strings.TrimSpace(os.Getenv(hostileCheckpointSwapSourceEnv))
	if sourceSHA == "" {
		sourceSHA = "UNRECORDED"
	}
	evidence := hostileCheckpointSwapEvidence{
		Schema:                "picogent.v4.hostile-parent-swap-checkpoint-evidence.v1",
		CandidateSHA:          sourceSHA,
		OS:                    runtime.GOOS,
		Architecture:          runtime.GOARCH,
		Environment:           "task-owned-disposable",
		Package:               "checkpoint",
		AttackerConfirmed:     attackerConfirmed,
		OutsideSentinelBefore: beforeSentinel,
		OutsideSentinelAfter:  afterSentinel,
		OutsideTreeBefore:     beforeTree,
		OutsideTreeAfter:      afterTree,
		OutsideEscapeObserved: anyEscape,
		Operations:            evidenceOps,
		Verdict:               overall,
		ObservedAt:            time.Now().UTC().Format(time.RFC3339),
		BroadTOCTOUClaim:      "UNVERIFIED",
	}
	payload, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	evidencePath := filepath.Join(root, "hostile-parent-swap-checkpoint-evidence.json")
	if err := os.WriteFile(evidencePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if dest := strings.TrimSpace(os.Getenv(hostileCheckpointSwapEvidenceEnv)); dest != "" {
		if err := os.WriteFile(dest, payload, 0o600); err != nil {
			t.Fatalf("retain evidence: %v", err)
		}
	}
	t.Logf("checkpoint parent-swap evidence verdict=%s path=%s ops=%d", overall, evidencePath, len(evidenceOps))
	if overall != "PASS" {
		t.Fatalf("checkpoint parent-swap confinement verdict=%s", overall)
	}
}

func TestLinuxSameUIDCheckpointParentSwapAttackerHelper(t *testing.T) {
	if os.Getenv(hostileCheckpointSwapHelperEnv) != "1" {
		return
	}
	parent := os.Getenv("PICOGENT_HOSTILE_PARENT")
	backup := os.Getenv("PICOGENT_HOSTILE_BACKUP")
	outside := os.Getenv("PICOGENT_HOSTILE_OUTSIDE")
	ready := os.Getenv("PICOGENT_HOSTILE_READY")
	release := os.Getenv("PICOGENT_HOSTILE_RELEASE")
	stop := os.Getenv("PICOGENT_HOSTILE_STOP")
	swapsPath := os.Getenv("PICOGENT_HOSTILE_SWAPS")
	if parent == "" || backup == "" || outside == "" || ready == "" || release == "" || stop == "" || swapsPath == "" {
		t.Fatal("checkpoint attacker helper missing required environment")
	}
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(release); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for parent release")
		}
		time.Sleep(5 * time.Millisecond)
	}

	swaps := 0
	stopDeadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := os.Stat(stop); err == nil {
			break
		}
		if time.Now().After(stopDeadline) {
			break
		}
		if err := os.Rename(parent, backup); err != nil {
			continue
		}
		if err := os.Symlink(outside, parent); err != nil {
			_ = os.Rename(backup, parent)
			continue
		}
		swaps++
		_ = os.WriteFile(swapsPath, []byte(strconv.Itoa(swaps)+"\n"), 0o600)
		_ = os.Remove(parent)
		_ = os.Rename(backup, parent)
	}
	if err := os.WriteFile(swapsPath, []byte(strconv.Itoa(swaps)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitForCheckpointFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForCheckpointSwaps(t *testing.T, path string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if data, err := os.ReadFile(path); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && n >= want {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for >=%d attacker swaps in %s", want, path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func readCheckpointSwapCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read swap count: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse swap count %q: %v", data, err)
	}
	return n
}

func checkpointOutsideTreeMutated(path, beforeDigest string) (bool, error) {
	afterDigest, err := checkpointOutsideTreeSHA256(path)
	if err != nil {
		return true, err
	}
	return afterDigest != beforeDigest, nil
}

func checkpointOutsideTreeSHA256(path string) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	hash := sha256.New()
	for _, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())
		info, err := os.Lstat(entryPath)
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00", entry.Name(), info.Mode().String())
		switch {
		case info.Mode().IsRegular():
			data, err := os.ReadFile(entryPath)
			if err != nil {
				return "", err
			}
			_, _ = hash.Write(data)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(entryPath)
			if err != nil {
				return "", err
			}
			_, _ = fmt.Fprint(hash, target)
		}
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sha256HexCheckpoint(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
