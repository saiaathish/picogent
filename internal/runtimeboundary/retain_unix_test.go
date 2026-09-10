//go:build unix

package runtimeboundary

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
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRetainReportParentSwapNeverEscapesDescriptor(t *testing.T) {
	workspace := t.TempDir()
	root := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(root, "artifacts")
	backup := filepath.Join(root, "artifacts-real")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "sentinel.txt"), []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("a", 40)
	report := sampleReport(sha)
	if err := RetainReport(workspace, filepath.Join(parent, "baseline.json"), report); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var swaps sync.WaitGroup
	swaps.Add(1)
	go func() {
		defer swaps.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := os.Rename(parent, backup); err != nil {
				continue
			}
			if err := os.Symlink(outside, parent); err != nil {
				_ = os.Rename(backup, parent)
				continue
			}
			_ = os.Remove(parent)
			_ = os.Rename(backup, parent)
		}
	}()

	for i := 0; i < 400; i++ {
		// A failure during the hostile interval is acceptable; a successful
		// operation must never create an entry in the outside directory.
		_ = RetainReport(workspace, filepath.Join(parent, fmt.Sprintf("matrix-%03d.json", i)), report)
	}
	close(stop)
	swaps.Wait()
	_ = os.RemoveAll(parent)
	_ = os.Rename(backup, parent)

	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "sentinel.txt" {
		t.Fatalf("hostile parent swap created outside entries: %+v", entries)
	}
}

const (
	hostileRetainLoadHelperEnv    = "PICOGENT_HOSTILE_RETAIN_LOAD_HELPER"
	hostileRetainLoadParentEnv    = "PICOGENT_HOSTILE_RETAIN_LOAD_PARENT"
	hostileRetainLoadBackupEnv    = "PICOGENT_HOSTILE_RETAIN_LOAD_BACKUP"
	hostileRetainLoadOutsideEnv   = "PICOGENT_HOSTILE_RETAIN_LOAD_OUTSIDE"
	hostileRetainLoadReadyEnv     = "PICOGENT_HOSTILE_RETAIN_LOAD_READY"
	hostileRetainLoadStopEnv      = "PICOGENT_HOSTILE_RETAIN_LOAD_STOP"
	hostileRetainLoadSwapsEnv     = "PICOGENT_HOSTILE_RETAIN_LOAD_SWAPS"
	hostileRetainLoadSourceEnv    = "PICOGENT_HOSTILE_RETAIN_LOAD_SOURCE_SHA"
	hostileRetainLoadEvidenceEnv  = "PICOGENT_HOSTILE_RETAIN_LOAD_EVIDENCE_OUT"
	hostileRetainLoadArtifactName = "runtime-boundary-matrix.json"
	hostileRetainLoadAttempts     = 400
)

type hostileRetainLoadEvidence struct {
	Schema                 string `json:"schema"`
	CandidateSHA           string `json:"candidate_sha"`
	HostOS                 string `json:"host_os"`
	HostArch               string `json:"host_arch"`
	Attempts               int    `json:"attempts"`
	SuccessfulLoads        int    `json:"successful_loads"`
	ConfirmedAttackerSwaps bool   `json:"confirmed_attacker_swaps"`
	OutsideMarkerObserved  bool   `json:"outside_marker_observed"`
	OutsideBeforeSHA256    string `json:"outside_before_sha256"`
	OutsideAfterSHA256     string `json:"outside_after_sha256"`
	SourceTreeModified     bool   `json:"source_tree_modified"`
	Verdict                string `json:"verdict"`
	BroadTOCTOUClaim       string `json:"broad_toctou_claim"`
}

// TestLoadReportParentSwapNeverReadsOutside exercises the retained-artifact
// read boundary with an independent same-UID process. The outside directory
// contains a valid artifact with a marker that would be observable if a
// pathname replacement escaped the descriptor-anchored parent. This is a
// bounded read-confinement observation, not a universal TOCTOU claim.
func TestLoadReportParentSwapNeverReadsOutside(t *testing.T) {
	if os.Getenv(hostileRetainLoadHelperEnv) == "1" {
		retainLoadParentSwapHelper(t)
		return
	}

	workspace := t.TempDir()
	root := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(root, "artifacts")
	backup := filepath.Join(root, "artifacts-real")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}

	sha := strings.Repeat("a", 40)
	artifact := filepath.Join(parent, hostileRetainLoadArtifactName)
	if err := RetainReport(workspace, artifact, sampleReport(sha)); err != nil {
		t.Fatal(err)
	}
	outsideReport := sampleReport(sha)
	outsideReport.Reason = "outside-marker"
	outsideData, err := encodeReport(outsideReport)
	if err != nil {
		t.Fatal(err)
	}
	outsideArtifact := filepath.Join(outside, hostileRetainLoadArtifactName)
	if err := os.WriteFile(outsideArtifact, outsideData, 0o600); err != nil {
		t.Fatal(err)
	}
	outsideBefore, err := retainLoadSHA256(outsideArtifact)
	if err != nil {
		t.Fatal(err)
	}

	ready := filepath.Join(root, "ready")
	stop := filepath.Join(root, "stop")
	swaps := filepath.Join(root, "swaps")
	cmd := exec.Command(os.Args[0], "-test.run", "^TestLoadReportParentSwapNeverReadsOutside$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		hostileRetainLoadHelperEnv+"=1",
		hostileRetainLoadParentEnv+"="+parent,
		hostileRetainLoadBackupEnv+"="+backup,
		hostileRetainLoadOutsideEnv+"="+outside,
		hostileRetainLoadReadyEnv+"="+ready,
		hostileRetainLoadStopEnv+"="+stop,
		hostileRetainLoadSwapsEnv+"="+swaps,
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

	waitForRetainLoadPath(t, ready)
	waitForRetainLoadPath(t, swaps)

	successfulLoads := 0
	outsideMarkerObserved := false
	for i := 0; i < hostileRetainLoadAttempts; i++ {
		loaded, loadErr := LoadReport(artifact, sha)
		if loadErr != nil {
			continue
		}
		if loaded.Reason == outsideReport.Reason {
			outsideMarkerObserved = true
			break
		}
		if loaded.CandidateSHA == sha {
			successfulLoads++
		}
	}
	if err := os.WriteFile(stop, []byte("stop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("hostile retained-artifact helper failed: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}

	outsideAfter, err := retainLoadSHA256(outsideArtifact)
	if err != nil {
		t.Fatal(err)
	}
	if outsideMarkerObserved {
		t.Fatal("LoadReport accepted the outside artifact during parent replacement")
	}
	if successfulLoads == 0 {
		t.Fatal("hostile campaign did not observe an in-tree successful load")
	}
	if outsideBefore != outsideAfter {
		t.Fatalf("outside artifact changed: before=%s after=%s", outsideBefore, outsideAfter)
	}
	loaded, err := LoadReport(artifact, sha)
	if err != nil {
		t.Fatalf("load after hostile campaign: %v", err)
	}
	if loaded.Reason == outsideReport.Reason {
		t.Fatal("post-campaign load returned the outside marker")
	}

	sourceSHA, sourceTreeModified := retainLoadSourceState()
	if expected := strings.TrimSpace(os.Getenv(hostileRetainLoadSourceEnv)); expected != "" && expected != sourceSHA {
		t.Fatalf("source SHA mismatch: expected %s, got %s", expected, sourceSHA)
	}
	if evidencePath := strings.TrimSpace(os.Getenv(hostileRetainLoadEvidenceEnv)); evidencePath != "" {
		if sourceTreeModified {
			t.Fatal("source tree must be clean before retaining hostile read evidence")
		}
		if err := writeRetainLoadEvidence(evidencePath, hostileRetainLoadEvidence{
			Schema:                 "picogent.v4.hostile-retained-artifact-read-evidence.v1",
			CandidateSHA:           sourceSHA,
			HostOS:                 runtime.GOOS,
			HostArch:               runtime.GOARCH,
			Attempts:               hostileRetainLoadAttempts,
			SuccessfulLoads:        successfulLoads,
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
		t.Logf("retained-artifact hostile read evidence path=%s source=%s", evidencePath, sourceSHA)
	}
}

func retainLoadParentSwapHelper(t *testing.T) {
	parent := os.Getenv(hostileRetainLoadParentEnv)
	backup := os.Getenv(hostileRetainLoadBackupEnv)
	outside := os.Getenv(hostileRetainLoadOutsideEnv)
	ready := os.Getenv(hostileRetainLoadReadyEnv)
	stop := os.Getenv(hostileRetainLoadStopEnv)
	swapsPath := os.Getenv(hostileRetainLoadSwapsEnv)
	if parent == "" || backup == "" || outside == "" || ready == "" || stop == "" || swapsPath == "" {
		t.Fatal("hostile retained-artifact helper environment is incomplete")
	}
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	confirmed := false
	for {
		if _, err := os.Stat(stop); err == nil {
			break
		}
		if err := os.Rename(parent, backup); err != nil {
			time.Sleep(time.Millisecond)
			continue
		}
		if err := os.Symlink(outside, parent); err != nil {
			_ = os.Rename(backup, parent)
			continue
		}
		confirmed = true
		if err := os.WriteFile(swapsPath, []byte("confirmed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(250 * time.Microsecond)
		_ = os.Remove(parent)
		if err := os.Rename(backup, parent); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Lstat(parent)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(backup, parent); err != nil {
			t.Fatal(err)
		}
	} else if err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(parent); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(backup, parent); err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	if !confirmed {
		t.Fatal("hostile helper did not confirm a parent replacement")
	}
}

func waitForRetainLoadPath(t *testing.T, path string) {
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

func retainLoadSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func retainLoadSourceState() (string, bool) {
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

func writeRetainLoadEvidence(path string, evidence hostileRetainLoadEvidence) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("hostile retained-artifact evidence path must be absolute")
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
