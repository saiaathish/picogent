//go:build linux

package securefile_test

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

	"github.com/saiaathish/picogent/internal/securefile"
)

const (
	hostileParentSwapHelperEnv   = "PICOGENT_HOSTILE_PARENT_SWAP_HELPER"
	hostileParentSwapEvidenceEnv = "PICOGENT_HOSTILE_PARENT_SWAP_EVIDENCE_OUT"
	hostileParentSwapAttempts    = 250
)

type hostileParentSwapEvidence struct {
	Schema                string                        `json:"schema"`
	CandidateSHA          string                        `json:"candidate_sha"`
	OS                    string                        `json:"os"`
	Architecture          string                        `json:"architecture"`
	Environment           string                        `json:"environment"`
	Package               string                        `json:"package"`
	AttackerConfirmed     bool                          `json:"attacker_confirmed"`
	OutsideSentinelBefore string                        `json:"outside_sentinel_before_sha256"`
	OutsideSentinelAfter  string                        `json:"outside_sentinel_after_sha256"`
	OutsideTreeBefore     string                        `json:"outside_tree_before_sha256"`
	OutsideTreeAfter      string                        `json:"outside_tree_after_sha256"`
	OutsideEscapeObserved bool                          `json:"outside_escape_observed"`
	Operations            []hostileParentSwapOpEvidence `json:"operations"`
	Verdict               string                        `json:"verdict"`
	ObservedAt            string                        `json:"observed_at"`
	BroadTOCTOUClaim      string                        `json:"broad_toctou_claim"`
	SourceTreeModified    bool                          `json:"source_tree_modified"`
}

type hostileParentSwapOpEvidence struct {
	ID             string `json:"id"`
	Attempts       int    `json:"attempts"`
	Successes      int    `json:"successes"`
	Errors         int    `json:"errors"`
	AttackerSwaps  int    `json:"attacker_swaps"`
	EscapeObserved bool   `json:"escape_observed"`
	Verdict        string `json:"verdict"`
}

func TestLinuxSameUIDParentSwapConfinement(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only same-UID parent-swap harness")
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
	sentinelPath := filepath.Join(outside, "sentinel.txt")
	sentinelContent := []byte("outside-sentinel-v1\n")
	if err := os.WriteFile(sentinelPath, sentinelContent, 0o600); err != nil {
		t.Fatal(err)
	}
	// Same-name markers make a redirected read observable. The full outside
	// tree digest below also catches escaped creation, replacement, and
	// deletion even when the named sentinel itself is untouched.
	if err := os.WriteFile(filepath.Join(outside, "state.yaml"), []byte("outside-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "remove-target.txt"), []byte("outside-remove\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeDigest := sha256Hex(sentinelContent)
	beforeTreeDigest, err := outsideTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}

	ops := []struct {
		id   string
		run  func(parent string) (successes, errors int, escape bool)
		prep func(parent string) error
	}{
		{
			id: "securefile-write-atomic",
			prep: func(parent string) error {
				return nil
			},
			run: func(parent string) (int, int, bool) {
				successes, errs := 0, 0
				escape := false
				for i := 0; i < hostileParentSwapAttempts; i++ {
					err := securefile.WriteAtomic(filepath.Join(parent, "state.yaml"), []byte(fmt.Sprintf("inside-%d\n", i)), 0o600)
					if err == nil {
						successes++
					} else {
						errs++
					}
					if mutated, checkErr := outsideTreeMutated(outside, beforeTreeDigest); checkErr != nil {
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
			id: "securefile-read-file",
			prep: func(parent string) error {
				return securefile.WriteAtomic(filepath.Join(parent, "state.yaml"), []byte("inside\n"), 0o600)
			},
			run: func(parent string) (int, int, bool) {
				successes, errs := 0, 0
				escape := false
				for i := 0; i < hostileParentSwapAttempts; i++ {
					data, err := securefile.ReadFile(filepath.Join(parent, "state.yaml"))
					switch {
					case err != nil:
						errs++
					case string(data) == "inside\n":
						successes++
					case string(data) == string(sentinelContent) || strings.HasPrefix(string(data), "outside"):
						escape = true
					default:
						errs++
					}
					if escape {
						break
					}
					if mutated, checkErr := outsideTreeMutated(outside, beforeTreeDigest); checkErr != nil {
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
			id: "securefile-write-exclusive",
			prep: func(parent string) error {
				return nil
			},
			run: func(parent string) (int, int, bool) {
				successes, errs := 0, 0
				escape := false
				for i := 0; i < hostileParentSwapAttempts; i++ {
					name := filepath.Join(parent, fmt.Sprintf("exclusive-%d.json", i))
					err := securefile.WriteExclusive(name, []byte(fmt.Sprintf("exclusive-%d\n", i)), 0o600)
					if err == nil {
						successes++
					} else {
						errs++
					}
					if mutated, checkErr := outsideTreeMutated(outside, beforeTreeDigest); checkErr != nil {
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
			id: "securefile-remove-file",
			prep: func(parent string) error {
				return os.WriteFile(filepath.Join(parent, "remove-target.txt"), []byte("inside-remove\n"), 0o600)
			},
			run: func(parent string) (int, int, bool) {
				successes, errs := 0, 0
				escape := false
				for i := 0; i < hostileParentSwapAttempts; i++ {
					name := filepath.Join(parent, "remove-target.txt")
					err := securefile.RemoveFile(name)
					if err == nil {
						successes++
					} else {
						errs++
					}
					if mutated, checkErr := outsideTreeMutated(outside, beforeTreeDigest); checkErr != nil {
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

	evidenceOps := make([]hostileParentSwapOpEvidence, 0, len(ops))
	anyEscape := false
	attackerConfirmed := false

	for _, op := range ops {
		parent := filepath.Join(trustedRoot, op.id)
		backup := parent + "-real"
		if err := os.MkdirAll(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := op.prep(parent); err != nil {
			t.Fatalf("%s prep: %v", op.id, err)
		}

		ready := filepath.Join(root, op.id+"-ready")
		release := filepath.Join(root, op.id+"-release")
		stop := filepath.Join(root, op.id+"-stop")
		swapsPath := filepath.Join(root, op.id+"-swaps")
		_ = os.Remove(ready)
		_ = os.Remove(release)
		_ = os.Remove(stop)
		_ = os.Remove(swapsPath)

		cmd := exec.Command(os.Args[0], "-test.run", "^TestLinuxSameUIDParentSwapAttackerHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			hostileParentSwapHelperEnv+"=1",
			"PICOGENT_HOSTILE_PARENT="+parent,
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
		waitForFile(t, ready, 15*time.Second)
		if err := os.WriteFile(release, []byte("go\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		// Require confirmed attacker activity before measuring confinement.
		waitForConfirmedSwaps(t, swapsPath, 1, 15*time.Second)

		successes, errs, escape := op.run(parent)
		// Keep the attacker live briefly so the final swap counter is flushed
		// after the last operation rather than racing the stop file.
		time.Sleep(50 * time.Millisecond)
		if err := os.WriteFile(stop, []byte("stop\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		waitErr := cmd.Wait()
		if waitErr != nil {
			t.Fatalf("attacker for %s failed: %v\nstdout=%s\nstderr=%s", op.id, waitErr, stdout.String(), stderr.String())
		}

		swaps := readSwapCount(t, swapsPath)
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
		evidenceOps = append(evidenceOps, hostileParentSwapOpEvidence{
			ID:             op.id,
			Attempts:       hostileParentSwapAttempts,
			Successes:      successes,
			Errors:         errs,
			AttackerSwaps:  swaps,
			EscapeObserved: escape,
			Verdict:        verdict,
		})
		if escape {
			t.Fatalf("%s escaped into outside sentinel directory", op.id)
		}

		// Restore trusted parent if left as symlink.
		if info, err := os.Lstat(parent); err == nil && info.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(parent)
		}
		if _, err := os.Lstat(parent); errors.Is(err, os.ErrNotExist) {
			_ = os.Rename(backup, parent)
		}
	}

	after, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatal(err)
	}
	afterDigest := sha256Hex(after)
	if afterDigest != beforeDigest {
		anyEscape = true
		t.Fatalf("outside sentinel mutated: before=%s after=%s", beforeDigest, afterDigest)
	}
	afterTreeDigest, err := outsideTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}
	if afterTreeDigest != beforeTreeDigest {
		anyEscape = true
		t.Fatalf("outside tree mutated: before=%s after=%s", beforeTreeDigest, afterTreeDigest)
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

	sha := strings.TrimSpace(os.Getenv("PICOGENT_HOSTILE_PARENT_SWAP_SOURCE_SHA"))
	if sha == "" {
		sha = "UNRECORDED"
	}
	evidence := hostileParentSwapEvidence{
		Schema:                "picogent.v4.hostile-parent-swap-evidence.v1",
		CandidateSHA:          sha,
		OS:                    runtime.GOOS,
		Architecture:          runtime.GOARCH,
		Environment:           "task-owned-disposable",
		Package:               "securefile",
		AttackerConfirmed:     attackerConfirmed,
		OutsideSentinelBefore: beforeDigest,
		OutsideSentinelAfter:  afterDigest,
		OutsideTreeBefore:     beforeTreeDigest,
		OutsideTreeAfter:      afterTreeDigest,
		OutsideEscapeObserved: anyEscape,
		Operations:            evidenceOps,
		Verdict:               overall,
		ObservedAt:            time.Now().UTC().Format(time.RFC3339),
		BroadTOCTOUClaim:      "UNVERIFIED",
		SourceTreeModified:    false,
	}
	payload, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	outPath := filepath.Join(root, "hostile-parent-swap-evidence.json")
	if err := os.WriteFile(outPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if dest := strings.TrimSpace(os.Getenv(hostileParentSwapEvidenceEnv)); dest != "" {
		if err := os.WriteFile(dest, payload, 0o600); err != nil {
			t.Fatalf("retain evidence: %v", err)
		}
	}
	t.Logf("hostile parent-swap evidence verdict=%s path=%s", overall, outPath)
	if overall != "PASS" {
		t.Fatalf("hostile parent-swap confinement verdict=%s", overall)
	}
}

func TestLinuxSameUIDParentSwapAttackerHelper(t *testing.T) {
	if os.Getenv(hostileParentSwapHelperEnv) != "1" {
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
		t.Fatal("attacker helper missing required environment")
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

func waitForFile(t *testing.T, path string, timeout time.Duration) {
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

func waitForConfirmedSwaps(t *testing.T, path string, want int, timeout time.Duration) {
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

func readSwapCount(t *testing.T, path string) int {
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

func outsideTreeMutated(path, beforeDigest string) (bool, error) {
	afterDigest, err := outsideTreeSHA256(path)
	if err != nil {
		return true, err
	}
	return afterDigest != beforeDigest, nil
}

func outsideTreeSHA256(path string) (string, error) {
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

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
