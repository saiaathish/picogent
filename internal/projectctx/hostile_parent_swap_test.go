package projectctx

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
	projectRuleHostileHelperEnv   = "PICOGENT_PROJECT_RULE_HOSTILE_HELPER"
	projectRuleHostileParentEnv   = "PICOGENT_PROJECT_RULE_HOSTILE_PARENT"
	projectRuleHostileBackupEnv   = "PICOGENT_PROJECT_RULE_HOSTILE_BACKUP"
	projectRuleHostileOutsideEnv  = "PICOGENT_PROJECT_RULE_HOSTILE_OUTSIDE"
	projectRuleHostileReadyEnv    = "PICOGENT_PROJECT_RULE_HOSTILE_READY"
	projectRuleHostileStopEnv     = "PICOGENT_PROJECT_RULE_HOSTILE_STOP"
	projectRuleHostileSwapsEnv    = "PICOGENT_PROJECT_RULE_HOSTILE_SWAPS"
	projectRuleHostileSourceEnv   = "PICOGENT_PROJECT_RULE_HOSTILE_SOURCE_SHA"
	projectRuleHostileEvidenceEnv = "PICOGENT_PROJECT_RULE_HOSTILE_EVIDENCE_OUT"
	projectRuleHostileAttempts    = 256
	projectRuleHostilePause       = 250 * time.Microsecond
)

type projectRuleHostileEvidence struct {
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

// TestProjectRuleHostileParentSwapConfinement exercises the project-rule
// loader while an independent same-UID process swaps .picogent for a symlink
// or Windows junction to an outside directory. A successful Load must never
// return the outside marker. This is a bounded project-rule-read observation,
// not a universal filesystem TOCTOU claim.
func TestProjectRuleHostileParentSwapConfinement(t *testing.T) {
	if err := projectRuleReparseProbe(t); err != nil {
		if strings.TrimSpace(os.Getenv(projectRuleHostileEvidenceEnv)) != "" {
			t.Fatalf("reparse support is required for evidence mode: %v", err)
		}
		t.Skipf("reparse support unavailable: %v", err)
	}

	if os.Getenv(projectRuleHostileHelperEnv) == "1" {
		projectRuleHostileParentSwapHelper(t)
		return
	}

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	parent := filepath.Join(workspace, ".picogent")
	backup := filepath.Join(workspace, ".picogent-real")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "rules.md"), []byte("inside-project-rule-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideRules := filepath.Join(outside, "rules.md")
	if err := os.WriteFile(outsideRules, []byte("outside-project-rule-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideBefore, err := projectRuleSHA256(outsideRules)
	if err != nil {
		t.Fatal(err)
	}

	ready := filepath.Join(root, "ready")
	stop := filepath.Join(root, "stop")
	swaps := filepath.Join(root, "swaps")
	cmd := exec.Command(os.Args[0], "-test.run", "^TestProjectRuleHostileParentSwapHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		projectRuleHostileHelperEnv+"=1",
		projectRuleHostileParentEnv+"="+parent,
		projectRuleHostileBackupEnv+"="+backup,
		projectRuleHostileOutsideEnv+"="+outside,
		projectRuleHostileReadyEnv+"="+ready,
		projectRuleHostileStopEnv+"="+stop,
		projectRuleHostileSwapsEnv+"="+swaps,
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

	projectRuleHostileWaitForPath(t, ready)
	projectRuleHostileWaitForPath(t, swaps)

	successfulLoads := 0
	outsideMarkerObserved := false
	for i := 0; i < projectRuleHostileAttempts; i++ {
		loaded := Load(workspace)
		switch {
		case strings.Contains(loaded, "outside-project-rule-marker"):
			outsideMarkerObserved = true
		case strings.Contains(loaded, "inside-project-rule-marker"):
			successfulLoads++
		}
	}
	if err := os.WriteFile(stop, []byte("stop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("hostile project-rule helper failed: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}

	outsideAfter, err := projectRuleSHA256(outsideRules)
	if err != nil {
		t.Fatal(err)
	}
	if outsideMarkerObserved {
		t.Fatal("Load returned project rules through an outside reparse point")
	}
	if successfulLoads == 0 {
		t.Fatal("hostile project-rule campaign did not observe an in-tree load")
	}
	if outsideBefore != outsideAfter {
		t.Fatalf("outside project-rule file changed: before=%s after=%s", outsideBefore, outsideAfter)
	}
	if loaded := Load(workspace); !strings.Contains(loaded, "inside-project-rule-marker") {
		t.Fatalf("post-campaign trusted project-rule load failed: %q", loaded)
	}

	sourceSHA, sourceTreeModified := projectRuleSourceState()
	if expected := strings.TrimSpace(os.Getenv(projectRuleHostileSourceEnv)); expected != "" && expected != sourceSHA {
		t.Fatalf("source SHA mismatch: expected %s, got %s", expected, sourceSHA)
	}
	if evidencePath := strings.TrimSpace(os.Getenv(projectRuleHostileEvidenceEnv)); evidencePath != "" {
		if sourceTreeModified {
			t.Fatal("source tree must be clean before retaining hostile project-rule evidence")
		}
		if err := projectRuleWriteEvidence(evidencePath, projectRuleHostileEvidence{
			Schema:                 "picogent.v4.project-rule-read-evidence.v1",
			CandidateSHA:           sourceSHA,
			HostOS:                 runtime.GOOS,
			HostArch:               runtime.GOARCH,
			Attempts:               projectRuleHostileAttempts,
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
		t.Logf("project-rule hostile evidence path=%s source=%s", evidencePath, sourceSHA)
	}
}

func TestProjectRuleHostileParentSwapHelper(t *testing.T) {
	if os.Getenv(projectRuleHostileHelperEnv) != "1" {
		t.Skip("hostile helper subprocess only")
	}
	projectRuleHostileParentSwapHelper(t)
}

func projectRuleReparseProbe(t *testing.T) error {
	t.Helper()
	root := t.TempDir()
	link := filepath.Join(root, "reparse")
	target := t.TempDir()
	if err := projectRuleCreateReparse(link, target); err != nil {
		return err
	}
	if err := os.Remove(link); err != nil {
		return fmt.Errorf("remove reparse probe: %w", err)
	}
	return nil
}

func projectRuleCreateReparse(link, target string) error {
	if runtime.GOOS != "windows" {
		return os.Symlink(target, link)
	}
	output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J %q -> %q: %w (%s)", link, target, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func projectRuleHostileParentSwapHelper(t *testing.T) {
	parent := os.Getenv(projectRuleHostileParentEnv)
	backup := os.Getenv(projectRuleHostileBackupEnv)
	outside := os.Getenv(projectRuleHostileOutsideEnv)
	ready := os.Getenv(projectRuleHostileReadyEnv)
	stop := os.Getenv(projectRuleHostileStopEnv)
	swapsPath := os.Getenv(projectRuleHostileSwapsEnv)
	if parent == "" || backup == "" || outside == "" || ready == "" || stop == "" || swapsPath == "" {
		t.Fatal("hostile project-rule helper environment is incomplete")
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
		if err := projectRuleCreateReparse(parent, outside); err != nil {
			_ = os.Rename(backup, parent)
			time.Sleep(time.Millisecond)
			continue
		}
		confirmed = true
		if err := os.WriteFile(swapsPath, []byte("confirmed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(projectRuleHostilePause)
		if err := os.Remove(parent); err != nil {
			t.Fatal(fmt.Errorf("remove hostile project-rule parent: %w", err))
		}
		if err := os.Rename(backup, parent); err != nil {
			t.Fatal(fmt.Errorf("restore trusted project-rule parent: %w", err))
		}
		time.Sleep(projectRuleHostilePause)
	}
	if _, err := os.Stat(parent); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(backup, parent); err != nil {
			t.Fatal(err)
		}
	}
	if !confirmed {
		t.Fatal("hostile project-rule helper did not confirm a reparse replacement")
	}
}

func projectRuleHostileWaitForPath(t *testing.T, path string) {
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

func projectRuleSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func projectRuleSourceState() (string, bool) {
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

func projectRuleWriteEvidence(path string, evidence projectRuleHostileEvidence) error {
	if !filepath.IsAbs(path) {
		return errors.New("hostile project-rule evidence path must be absolute")
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
