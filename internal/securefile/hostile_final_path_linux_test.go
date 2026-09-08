//go:build linux

package securefile_test

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/saiaathishkarthik/picogent/internal/securefile"
)

const (
	hostileFinalPathHelperEnv   = "PICOGENT_HOSTILE_FINAL_PATH_HELPER"
	hostileFinalPathEvidenceEnv = "PICOGENT_HOSTILE_FINAL_PATH_EVIDENCE_OUT"
	hostileFinalPathSourceEnv   = "PICOGENT_HOSTILE_FINAL_PATH_SOURCE_SHA"
	hostileFinalPathAttempts    = 250
)

type hostileFinalPathEvidence struct {
	Schema                string                       `json:"schema"`
	CandidateSHA          string                       `json:"candidate_sha"`
	OS                    string                       `json:"os"`
	Architecture          string                       `json:"architecture"`
	Environment           string                       `json:"environment"`
	Package               string                       `json:"package"`
	AttackerConfirmed     bool                         `json:"attacker_confirmed"`
	OutsideTreeBefore     string                       `json:"outside_tree_before_sha256"`
	OutsideTreeAfter      string                       `json:"outside_tree_after_sha256"`
	OutsideEscapeObserved bool                         `json:"outside_escape_observed"`
	Operations            []hostileFinalPathOpEvidence `json:"operations"`
	Verdict               string                       `json:"verdict"`
	ObservedAt            string                       `json:"observed_at"`
	BroadTOCTOUClaim      string                       `json:"broad_toctou_claim"`
	SourceTreeModified    bool                         `json:"source_tree_modified"`
}

type hostileFinalPathOpEvidence struct {
	ID             string `json:"id"`
	Attempts       int    `json:"attempts"`
	Successes      int    `json:"successes"`
	Errors         int    `json:"errors"`
	AttackerSwaps  int    `json:"attacker_swaps"`
	EscapeObserved bool   `json:"escape_observed"`
	Verdict        string `json:"verdict"`
}

func TestLinuxSameUIDFinalPathReplacementConfinement(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only same-UID final-path replacement harness")
	}
	candidateSHA, sourceTreeModified := currentSourceState(t)
	if sourceTreeModified {
		t.Fatalf("source tree must be clean before retaining hostile evidence")
	}
	if expected := strings.TrimSpace(os.Getenv(hostileFinalPathSourceEnv)); expected != "" && expected != candidateSHA {
		t.Fatalf("source SHA mismatch: expected %s, current HEAD is %s", expected, candidateSHA)
	}

	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	trusted := filepath.Join(root, "trusted")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(trusted, 0o700); err != nil {
		t.Fatal(err)
	}
	outsideTargets := map[string]string{
		"securefile-write-atomic":    "state.yaml",
		"securefile-read-file":       "state.yaml",
		"securefile-write-exclusive": "exclusive.json",
		"securefile-remove-file":     "remove-target.txt",
	}
	outsideNames := map[string]struct{}{}
	for _, name := range outsideTargets {
		outsideNames[name] = struct{}{}
	}
	for name := range outsideNames {
		if err := os.WriteFile(filepath.Join(outside, name), []byte("outside-"+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	beforeTreeDigest, err := outsideTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}

	ops := []struct {
		id     string
		name   string
		prep   func(path string) error
		run    func(path string) (successes, errors int, escape bool)
		verify func(path string) error
	}{
		{
			id:   "securefile-write-atomic",
			name: "state.yaml",
			prep: func(path string) error {
				return securefile.WriteAtomic(path, []byte("inside\n"), 0o600)
			},
			run: func(path string) (int, int, bool) {
				successes, errs := 0, 0
				for i := 0; i < hostileFinalPathAttempts; i++ {
					if err := securefile.WriteAtomic(path, []byte(fmt.Sprintf("inside-%d\n", i)), 0o600); err != nil {
						errs++
					} else {
						successes++
					}
					if mutated, checkErr := outsideTreeMutated(outside, beforeTreeDigest); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						return successes, errs, true
					}
				}
				return successes, errs, false
			},
			verify: func(path string) error {
				if err := securefile.WriteAtomic(path, []byte("inside-verified\n"), 0o600); err != nil {
					return fmt.Errorf("write trusted atomic result: %w", err)
				}
				data, err := securefile.ReadFile(path)
				if err != nil {
					return fmt.Errorf("read trusted atomic result: %w", err)
				}
				if string(data) != "inside-verified\n" {
					return fmt.Errorf("trusted atomic result has unexpected content %q", data)
				}
				return nil
			},
		},
		{
			id:   "securefile-read-file",
			name: "state.yaml",
			prep: func(path string) error {
				return securefile.WriteAtomic(path, []byte("inside\n"), 0o600)
			},
			run: func(path string) (int, int, bool) {
				successes, errs := 0, 0
				for i := 0; i < hostileFinalPathAttempts; i++ {
					data, err := securefile.ReadFile(path)
					switch {
					case err != nil:
						errs++
					case string(data) == "inside\n":
						successes++
					case strings.HasPrefix(string(data), "outside-"):
						return successes, errs, true
					default:
						errs++
					}
					if mutated, checkErr := outsideTreeMutated(outside, beforeTreeDigest); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						return successes, errs, true
					}
				}
				return successes, errs, false
			},
			verify: func(path string) error {
				data, err := securefile.ReadFile(path)
				if err != nil {
					return fmt.Errorf("read trusted read result: %w", err)
				}
				if string(data) != "inside\n" {
					return fmt.Errorf("trusted read result has unexpected content %q", data)
				}
				return nil
			},
		},
		{
			id:   "securefile-write-exclusive",
			name: "exclusive.json",
			prep: func(string) error { return nil },
			run: func(path string) (int, int, bool) {
				successes, errs := 0, 0
				for i := 0; i < hostileFinalPathAttempts; i++ {
					if err := securefile.WriteExclusive(path, []byte(fmt.Sprintf("exclusive-%d\n", i)), 0o600); err != nil {
						errs++
					} else {
						successes++
					}
					if mutated, checkErr := outsideTreeMutated(outside, beforeTreeDigest); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						return successes, errs, true
					}
				}
				return successes, errs, false
			},
			verify: func(path string) error {
				verified := filepath.Join(filepath.Dir(path), "exclusive-verified.json")
				if err := securefile.WriteExclusive(verified, []byte("exclusive-verified\n"), 0o600); err != nil {
					return fmt.Errorf("write trusted exclusive result: %w", err)
				}
				data, err := securefile.ReadFile(verified)
				if err != nil {
					return fmt.Errorf("read trusted exclusive result: %w", err)
				}
				if string(data) != "exclusive-verified\n" {
					return fmt.Errorf("trusted exclusive result has unexpected content %q", data)
				}
				return nil
			},
		},
		{
			id:   "securefile-remove-file",
			name: "remove-target.txt",
			prep: func(path string) error {
				return os.WriteFile(path, []byte("inside-remove\n"), 0o600)
			},
			run: func(path string) (int, int, bool) {
				successes, errs := 0, 0
				for i := 0; i < hostileFinalPathAttempts; i++ {
					if err := securefile.RemoveFile(path); err != nil {
						errs++
					} else {
						successes++
					}
					if mutated, checkErr := outsideTreeMutated(outside, beforeTreeDigest); checkErr != nil {
						t.Fatal(checkErr)
					} else if mutated {
						return successes, errs, true
					}
				}
				return successes, errs, false
			},
			verify: func(path string) error {
				if err := os.WriteFile(path, []byte("inside-remove-verified\n"), 0o600); err != nil {
					return fmt.Errorf("prepare trusted remove result: %w", err)
				}
				if err := securefile.RemoveFile(path); err != nil {
					return fmt.Errorf("remove trusted remove result: %w", err)
				}
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					if err == nil {
						return fmt.Errorf("trusted remove result remained")
					}
					return fmt.Errorf("stat trusted remove result: %w", err)
				}
				return nil
			},
		},
	}

	evidenceOps := make([]hostileFinalPathOpEvidence, 0, len(ops))
	anyEscape := false
	attackerConfirmed := false

	for _, op := range ops {
		path := filepath.Join(trusted, op.name)
		if err := op.prep(path); err != nil {
			t.Fatalf("%s prep: %v", op.id, err)
		}

		ready := filepath.Join(root, op.id+"-ready")
		release := filepath.Join(root, op.id+"-release")
		stop := filepath.Join(root, op.id+"-stop")
		swapsPath := filepath.Join(root, op.id+"-swaps")
		backup := filepath.Join(trusted, "."+op.name+"."+op.id+".real")
		for _, marker := range []string{ready, release, stop, swapsPath, backup} {
			_ = os.Remove(marker)
		}

		cmd := exec.Command(os.Args[0], "-test.run", "^TestLinuxSameUIDFinalPathReplacementAttackerHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			hostileFinalPathHelperEnv+"=1",
			"PICOGENT_HOSTILE_FINAL_PATH_PARENT="+trusted,
			"PICOGENT_HOSTILE_FINAL_PATH_NAME="+op.name,
			"PICOGENT_HOSTILE_FINAL_PATH_OUTSIDE="+filepath.Join(outside, outsideTargets[op.id]),
			"PICOGENT_HOSTILE_FINAL_PATH_BACKUP="+backup,
			"PICOGENT_HOSTILE_FINAL_PATH_READY="+ready,
			"PICOGENT_HOSTILE_FINAL_PATH_RELEASE="+release,
			"PICOGENT_HOSTILE_FINAL_PATH_STOP="+stop,
			"PICOGENT_HOSTILE_FINAL_PATH_SWAPS="+swapsPath,
		)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start final-path attacker for %s: %v", op.id, err)
		}
		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()
		waited := false
		var waitErr error
		waitForAttackerExit := func() error {
			if waited {
				return waitErr
			}
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			select {
			case waitErr = <-waitCh:
				waited = true
			case <-timer.C:
				_ = cmd.Process.Kill()
				waitErr = <-waitCh
				waitErr = fmt.Errorf("attacker did not stop within 5s: %w", waitErr)
				waited = true
			}
			return waitErr
		}
		restoreFinalPath := func() {
			if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(path)
			}
			if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
				if _, backupErr := os.Lstat(backup); backupErr == nil {
					_ = os.Rename(backup, path)
				}
			}
			_ = os.Remove(backup)
		}
		cleanup := func() error {
			_ = os.WriteFile(release, []byte("go\n"), 0o600)
			_ = os.WriteFile(stop, []byte("stop\n"), 0o600)
			err := waitForAttackerExit()
			restoreFinalPath()
			return err
		}
		cleaned := false
		defer func() {
			if !cleaned {
				_ = cleanup()
			}
		}()

		waitForFile(t, ready, 15*time.Second)
		if err := os.WriteFile(release, []byte("go\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		waitForConfirmedSwaps(t, swapsPath, 1, 15*time.Second)

		successes, errs, escape := op.run(path)
		time.Sleep(50 * time.Millisecond)
		if err := cleanup(); err != nil {
			t.Fatalf("attacker for %s failed: %v\nstdout=%s\nstderr=%s", op.id, err, stdout.String(), stderr.String())
		}
		cleaned = true

		swaps := readSwapCount(t, swapsPath)
		if swaps < 1 {
			t.Fatalf("%s observed no confirmed attacker swaps", op.id)
		}
		attackerConfirmed = true
		if escape {
			anyEscape = true
		}
		if err := op.verify(path); err != nil {
			t.Fatalf("%s trusted in-tree effect: %v", op.id, err)
		}
		verdict := "PASS"
		if escape {
			verdict = "FAIL"
		}
		evidenceOps = append(evidenceOps, hostileFinalPathOpEvidence{
			ID:             op.id,
			Attempts:       hostileFinalPathAttempts,
			Successes:      successes,
			Errors:         errs,
			AttackerSwaps:  swaps,
			EscapeObserved: escape,
			Verdict:        verdict,
		})
		if escape {
			t.Fatalf("%s escaped into the outside final-path target", op.id)
		}
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
	if anyEscape {
		overall = "FAIL"
	}
	if !attackerConfirmed {
		overall = "INCONCLUSIVE"
	}
	evidence := hostileFinalPathEvidence{
		Schema:                "picogent.v4.hostile-final-path-replacement-evidence.v1",
		CandidateSHA:          candidateSHA,
		OS:                    runtime.GOOS,
		Architecture:          runtime.GOARCH,
		Environment:           "task-owned-disposable",
		Package:               "securefile",
		AttackerConfirmed:     attackerConfirmed,
		OutsideTreeBefore:     beforeTreeDigest,
		OutsideTreeAfter:      afterTreeDigest,
		OutsideEscapeObserved: anyEscape,
		Operations:            evidenceOps,
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
	outPath := filepath.Join(root, "hostile-final-path-replacement-evidence.json")
	if err := os.WriteFile(outPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if dest := strings.TrimSpace(os.Getenv(hostileFinalPathEvidenceEnv)); dest != "" {
		if err := os.WriteFile(dest, payload, 0o600); err != nil {
			t.Fatalf("retain evidence: %v", err)
		}
	}
	t.Logf("hostile final-path replacement evidence verdict=%s path=%s", overall, outPath)
	if overall != "PASS" {
		t.Fatalf("hostile final-path replacement verdict=%s", overall)
	}
}

func TestLinuxSameUIDFinalPathReplacementAttackerHelper(t *testing.T) {
	if os.Getenv(hostileFinalPathHelperEnv) != "1" {
		return
	}
	parent := os.Getenv("PICOGENT_HOSTILE_FINAL_PATH_PARENT")
	name := os.Getenv("PICOGENT_HOSTILE_FINAL_PATH_NAME")
	outsideTarget := os.Getenv("PICOGENT_HOSTILE_FINAL_PATH_OUTSIDE")
	backup := os.Getenv("PICOGENT_HOSTILE_FINAL_PATH_BACKUP")
	ready := os.Getenv("PICOGENT_HOSTILE_FINAL_PATH_READY")
	release := os.Getenv("PICOGENT_HOSTILE_FINAL_PATH_RELEASE")
	stop := os.Getenv("PICOGENT_HOSTILE_FINAL_PATH_STOP")
	swapsPath := os.Getenv("PICOGENT_HOSTILE_FINAL_PATH_SWAPS")
	if parent == "" || name == "" || outsideTarget == "" || backup == "" || ready == "" || release == "" || stop == "" || swapsPath == "" {
		t.Fatal("final-path attacker helper missing required environment")
	}
	target := filepath.Join(parent, name)
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(release); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for final-path release")
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
		_ = os.Remove(backup)
		moved := false
		if err := os.Rename(target, backup); err == nil {
			moved = true
		} else if !errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := os.Symlink(outsideTarget, target); err != nil {
			if moved {
				_ = os.Rename(backup, target)
			}
			continue
		}
		swaps++
		_ = os.WriteFile(swapsPath, []byte(strconv.Itoa(swaps)+"\n"), 0o600)
		for i := 0; i < 8; i++ {
			runtime.Gosched()
		}
		_ = os.Remove(target)
		if moved {
			if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
				_ = os.Rename(backup, target)
			}
		}
	}
	if err := os.WriteFile(swapsPath, []byte(strconv.Itoa(swaps)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
