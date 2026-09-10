//go:build windows

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
	"testing"
	"time"
)

const (
	hostileWindowsRetainLoadHelperEnv   = "PICOGENT_HOSTILE_RETAIN_WINDOWS_LOAD_HELPER"
	hostileWindowsRetainLoadParentEnv   = "PICOGENT_HOSTILE_RETAIN_WINDOWS_LOAD_PARENT"
	hostileWindowsRetainLoadBackupEnv   = "PICOGENT_HOSTILE_RETAIN_WINDOWS_LOAD_BACKUP"
	hostileWindowsRetainLoadOutsideEnv  = "PICOGENT_HOSTILE_RETAIN_WINDOWS_LOAD_OUTSIDE"
	hostileWindowsRetainLoadReadyEnv    = "PICOGENT_HOSTILE_RETAIN_WINDOWS_LOAD_READY"
	hostileWindowsRetainLoadStopEnv     = "PICOGENT_HOSTILE_RETAIN_WINDOWS_LOAD_STOP"
	hostileWindowsRetainLoadSwapsEnv    = "PICOGENT_HOSTILE_RETAIN_WINDOWS_LOAD_SWAPS"
	hostileWindowsRetainLoadSourceEnv   = "PICOGENT_HOSTILE_RETAIN_WINDOWS_LOAD_SOURCE_SHA"
	hostileWindowsRetainLoadEvidenceEnv = "PICOGENT_HOSTILE_RETAIN_WINDOWS_LOAD_EVIDENCE_OUT"
	hostileWindowsRetainLoadArtifact    = "runtime-boundary-matrix.json"
	hostileWindowsRetainLoadAttempts    = 256
	hostileWindowsRetainLoadPause       = 250 * time.Microsecond
)

type hostileWindowsRetainLoadEvidence struct {
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

// TestWindowsSameUIDRetainedArtifactReadConfinement exercises the Windows
// retained-artifact read boundary while an independent same-UID helper swaps
// the trusted parent for a junction to an outside directory. A successful
// LoadReport must never follow the junction to the outside marker. This is a
// bounded reparse-point observation, not a universal TOCTOU claim.
func TestWindowsSameUIDRetainedArtifactReadConfinement(t *testing.T) {
	if err := windowsRetainLoadJunctionProbe(t); err != nil {
		if strings.TrimSpace(os.Getenv(hostileWindowsRetainLoadEvidenceEnv)) != "" {
			t.Fatalf("Windows junction support is required for evidence mode: %v", err)
		}
		t.Skipf("Windows junction support unavailable: %v", err)
	}
	if os.Getenv(hostileWindowsRetainLoadHelperEnv) == "1" {
		windowsRetainLoadParentSwapHelper(t)
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
	artifact := filepath.Join(parent, hostileWindowsRetainLoadArtifact)
	if err := RetainReport(workspace, artifact, sampleReport(sha)); err != nil {
		t.Fatal(err)
	}
	outsideReport := sampleReport(sha)
	outsideReport.Reason = "outside-marker"
	outsideData, err := encodeReport(outsideReport)
	if err != nil {
		t.Fatal(err)
	}
	outsideArtifact := filepath.Join(outside, hostileWindowsRetainLoadArtifact)
	if err := os.WriteFile(outsideArtifact, outsideData, 0o600); err != nil {
		t.Fatal(err)
	}
	outsideBefore, err := hostileWindowsRetainLoadSHA256(outsideArtifact)
	if err != nil {
		t.Fatal(err)
	}

	ready := filepath.Join(root, "ready")
	stop := filepath.Join(root, "stop")
	swaps := filepath.Join(root, "swaps")
	cmd := exec.Command(os.Args[0], "-test.run", "^TestWindowsSameUIDRetainedArtifactReadConfinement$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		hostileWindowsRetainLoadHelperEnv+"=1",
		hostileWindowsRetainLoadParentEnv+"="+parent,
		hostileWindowsRetainLoadBackupEnv+"="+backup,
		hostileWindowsRetainLoadOutsideEnv+"="+outside,
		hostileWindowsRetainLoadReadyEnv+"="+ready,
		hostileWindowsRetainLoadStopEnv+"="+stop,
		hostileWindowsRetainLoadSwapsEnv+"="+swaps,
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

	hostileWindowsRetainLoadWaitForPath(t, ready)
	hostileWindowsRetainLoadWaitForPath(t, swaps)

	successfulLoads := 0
	outsideMarkerObserved := false
	for i := 0; i < hostileWindowsRetainLoadAttempts; i++ {
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

	outsideAfter, err := hostileWindowsRetainLoadSHA256(outsideArtifact)
	if err != nil {
		t.Fatal(err)
	}
	if outsideMarkerObserved {
		t.Fatal("LoadReport accepted the outside artifact through a Windows junction")
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

	sourceSHA, sourceTreeModified := hostileWindowsRetainLoadSourceState()
	if expected := strings.TrimSpace(os.Getenv(hostileWindowsRetainLoadSourceEnv)); expected != "" && expected != sourceSHA {
		t.Fatalf("source SHA mismatch: expected %s, got %s", expected, sourceSHA)
	}
	if evidencePath := strings.TrimSpace(os.Getenv(hostileWindowsRetainLoadEvidenceEnv)); evidencePath != "" {
		if sourceTreeModified {
			t.Fatal("source tree must be clean before retaining hostile Windows read evidence")
		}
		if err := hostileWindowsRetainLoadWriteEvidence(evidencePath, hostileWindowsRetainLoadEvidence{
			Schema:                 "picogent.v4.hostile-retained-artifact-read-evidence.v1",
			CandidateSHA:           sourceSHA,
			HostOS:                 runtime.GOOS,
			HostArch:               runtime.GOARCH,
			Attempts:               hostileWindowsRetainLoadAttempts,
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
		t.Logf("Windows retained-artifact hostile read evidence path=%s source=%s", evidencePath, sourceSHA)
	}
}

func windowsRetainLoadJunctionProbe(t *testing.T) error {
	t.Helper()
	root := t.TempDir()
	link := filepath.Join(root, "junction")
	target := t.TempDir()
	if err := hostileWindowsRetainLoadCreateJunction(link, target); err != nil {
		return err
	}
	if err := os.Remove(link); err != nil {
		return fmt.Errorf("remove junction probe: %w", err)
	}
	return nil
}

func windowsRetainLoadParentSwapHelper(t *testing.T) {
	parent := os.Getenv(hostileWindowsRetainLoadParentEnv)
	backup := os.Getenv(hostileWindowsRetainLoadBackupEnv)
	outside := os.Getenv(hostileWindowsRetainLoadOutsideEnv)
	ready := os.Getenv(hostileWindowsRetainLoadReadyEnv)
	stop := os.Getenv(hostileWindowsRetainLoadStopEnv)
	swapsPath := os.Getenv(hostileWindowsRetainLoadSwapsEnv)
	if parent == "" || backup == "" || outside == "" || ready == "" || stop == "" || swapsPath == "" {
		t.Fatal("hostile Windows retained-artifact helper environment is incomplete")
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
		if err := hostileWindowsRetainLoadCreateJunction(parent, outside); err != nil {
			_ = os.Rename(backup, parent)
			time.Sleep(time.Millisecond)
			continue
		}
		confirmed = true
		if err := os.WriteFile(swapsPath, []byte("confirmed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(hostileWindowsRetainLoadPause)
		if err := os.Remove(parent); err != nil {
			t.Fatal(fmt.Errorf("remove hostile junction: %w", err))
		}
		if err := os.Rename(backup, parent); err != nil {
			t.Fatal(fmt.Errorf("restore trusted parent: %w", err))
		}
		time.Sleep(hostileWindowsRetainLoadPause)
	}
	if _, err := os.Stat(parent); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(backup, parent); err != nil {
			t.Fatal(err)
		}
	}
	if !confirmed {
		t.Fatal("hostile Windows helper did not confirm a junction replacement")
	}
}

func hostileWindowsRetainLoadCreateJunction(link, target string) error {
	// Pass the built-in command and paths as separate arguments. The Windows
	// runner's temp paths are short and contain no spaces, so this avoids
	// nested /c quoting while preserving backslashes as path separators.
	output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J %q -> %q: %w (%s)", link, target, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func hostileWindowsRetainLoadWaitForPath(t *testing.T, path string) {
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

func hostileWindowsRetainLoadSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func hostileWindowsRetainLoadSourceState() (string, bool) {
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

func hostileWindowsRetainLoadWriteEvidence(path string, evidence hostileWindowsRetainLoadEvidence) error {
	if !filepath.IsAbs(path) {
		return errors.New("hostile Windows retained-artifact evidence path must be absolute")
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
