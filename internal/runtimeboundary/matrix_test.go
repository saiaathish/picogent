package runtimeboundary

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectFailClosedWithoutLiveEvidence(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	sha := commitAll(t, workspace)

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		Now:          time.Unix(1700000000, 0).UTC(),
		Environ:      func(string) string { return "" },
	})
	if err != nil {
		t.Fatal(err)
	}
	live := claimByID(t, report, "live-provider-quality")
	if live.Verdict != VerdictUnverified {
		t.Fatalf("live verdict = %s", live.Verdict)
	}
	undoReload := claimByID(t, report, "rendered-recovery-undo-reload")
	if undoReload.Verdict != VerdictUnverified {
		t.Fatalf("rendered undo/reload verdict = %s", undoReload.Verdict)
	}
	if report.Summary[string(VerdictUnverified)] < 1 {
		t.Fatalf("summary = %+v", report.Summary)
	}
	release := claimByID(t, report, "release-authorization")
	if release.Verdict != VerdictInconclusive {
		t.Fatalf("release verdict = %s", release.Verdict)
	}
}

func TestCollectRejectsLiveFlagWithoutArtifact(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	sha := commitAll(t, workspace)
	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		Environ: func(key string) string {
			if key == LiveEvidenceEnv {
				return "1"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	live := claimByID(t, report, "live-provider-quality")
	if live.Verdict != VerdictFail {
		t.Fatalf("live verdict = %s reason=%s", live.Verdict, live.Reason)
	}
}

func TestCollectRejectsDirtyTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	sha := commitAll(t, workspace)
	if err := os.WriteFile(filepath.Join(workspace, "dirty.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Collect(Options{Workspace: workspace, CandidateSHA: sha})
	if err == nil || !strings.Contains(err.Error(), "clean") {
		t.Fatalf("expected dirty failure, got %v", err)
	}
}

func seedDocs(t *testing.T, workspace string) {
	t.Helper()
	write(t, workspace, "go.mod", "module example.test/runtimeboundary\n\ngo 1.25\n")
	write(t, workspace, "docs/V4-RENDERED-LONG-HORIZON-EVIDENCE.md", "# rendered\n")
	write(t, workspace, "docs/V4-RENDERED-RECOVERY-FIXTURE.md", "# recovery\n")
	write(t, workspace, "docs/V4-SECURITY-CAMPAIGN.md", "# security\n")
	write(t, workspace, "docs/V4-LONG-HORIZON-OUTCOME.md", "# long horizon\n")
	write(t, workspace, "docs/V4-RELEASE-AUDIT.md", "# audit\n")
	write(t, workspace, "docs/V4-SBOM-ARTIFACT-EVIDENCE.md", "# sbom\n")
	git(t, workspace, "init", "--quiet")
	git(t, workspace, "config", "user.name", "Picogent Test")
	git(t, workspace, "config", "user.email", "picogent@example.test")
}

func commitAll(t *testing.T, workspace string) string {
	t.Helper()
	git(t, workspace, "add", ".")
	git(t, workspace, "commit", "--quiet", "-m", "initial")
	return strings.TrimSpace(git(t, workspace, "rev-parse", "HEAD"))
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func claimByID(t *testing.T, report Report, id string) Claim {
	t.Helper()
	for _, claim := range report.Claims {
		if claim.ID == id {
			return claim
		}
	}
	t.Fatalf("missing claim %s", id)
	return Claim{}
}
