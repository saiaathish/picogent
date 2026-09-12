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
	if release.Verdict != VerdictUnverified {
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
	connectivity := claimByID(t, report, "live-provider-connectivity")
	if connectivity.Verdict != VerdictFail {
		t.Fatalf("connectivity verdict = %s reason=%s", connectivity.Verdict, connectivity.Reason)
	}
	quality := claimByID(t, report, "live-provider-quality")
	if quality.Verdict != VerdictUnverified {
		t.Fatalf("quality verdict = %s reason=%s", quality.Verdict, quality.Reason)
	}
}

func TestCollectPassesBoundedHostileFilesystemWithCurrentDocProvenance(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	behaviorSHA := commitAll(t, workspace)
	write(t, workspace, "docs/V4-HOSTILE-RUNTIME-EVIDENCE.md", "Status: `PASS`\n## Source identity\nsource: "+behaviorSHA+"\nBroadTOCTOUClaim: UNVERIFIED\n")
	sha := commitAll(t, workspace)

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		BehaviorSHA:  behaviorSHA,
		Now:          time.Unix(1700000000, 0).UTC(),
		Environ:      func(string) string { return "" },
	})
	if err != nil {
		t.Fatal(err)
	}
	deterministic := claimByID(t, report, "hostile-filesystem-deterministic")
	if deterministic.Verdict != VerdictPass {
		t.Fatalf("deterministic hostile-filesystem verdict = %s reason=%s", deterministic.Verdict, deterministic.Reason)
	}
	if !strings.Contains(deterministic.Reason, "same-UID filesystem TOCTOU") {
		t.Fatalf("deterministic hostile-filesystem reason = %q", deterministic.Reason)
	}
	parentSwap := claimByID(t, report, "hostile-parent-swap-confinement")
	if parentSwap.Verdict != VerdictUnverified {
		t.Fatalf("parent-swap verdict = %s reason=%s", parentSwap.Verdict, parentSwap.Reason)
	}
	toctou := claimByID(t, report, "hostile-filesystem-toctou")
	if toctou.Verdict != VerdictUnverified {
		t.Fatalf("TOCTOU verdict = %s reason=%s", toctou.Verdict, toctou.Reason)
	}
}

func TestCollectPassesBoundedParentSwapConfinementWithoutBroadTOCTOU(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	behaviorSHA := commitAll(t, workspace)
	write(t, workspace, "docs/V4-HOSTILE-PARENT-SWAP-DARWIN.md", "Status: bounded Darwin-only `PASS`\nsource: "+behaviorSHA+"\n")
	write(t, workspace, "docs/V4-HOSTILE-PARENT-SWAP-LINUX.md", "Status: bounded Linux-only `PASS`\nsource: "+behaviorSHA+"\n")
	sha := commitAll(t, workspace)

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		BehaviorSHA:  behaviorSHA,
		Now:          time.Unix(1700000000, 0).UTC(),
		Environ:      func(string) string { return "" },
	})
	if err != nil {
		t.Fatal(err)
	}
	parentSwap := claimByID(t, report, "hostile-parent-swap-confinement")
	if parentSwap.Verdict != VerdictPass {
		t.Fatalf("parent-swap verdict = %s reason=%s", parentSwap.Verdict, parentSwap.Reason)
	}
	if !strings.Contains(parentSwap.Reason, "arbitrary same-UID TOCTOU remains outside this claim") {
		t.Fatalf("parent-swap reason = %q", parentSwap.Reason)
	}
	toctou := claimByID(t, report, "hostile-filesystem-toctou")
	if toctou.Verdict != VerdictUnverified {
		t.Fatalf("TOCTOU verdict = %s reason=%s", toctou.Verdict, toctou.Reason)
	}
}

func TestCollectMarksPartialParentSwapConfinementInconclusive(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	behaviorSHA := commitAll(t, workspace)
	write(t, workspace, "docs/V4-HOSTILE-PARENT-SWAP-DARWIN.md", "Status: bounded Darwin-only `PASS`\nsource: "+behaviorSHA+"\n")
	sha := commitAll(t, workspace)

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		BehaviorSHA:  behaviorSHA,
		Now:          time.Unix(1700000000, 0).UTC(),
		Environ:      func(string) string { return "" },
	})
	if err != nil {
		t.Fatal(err)
	}
	parentSwap := claimByID(t, report, "hostile-parent-swap-confinement")
	if parentSwap.Verdict != VerdictInconclusive {
		t.Fatalf("parent-swap verdict = %s reason=%s", parentSwap.Verdict, parentSwap.Reason)
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

func TestCollectAcceptsBehaviorArtifactsAtDocsOnlyDescendant(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	behaviorSHA := commitAll(t, workspace)
	liveArtifact := filepath.Join(t.TempDir(), "live-provider.json")
	qualityArtifact := filepath.Join(t.TempDir(), "live-provider-quality.json")
	renderedArtifact := filepath.Join(t.TempDir(), "rendered-platform.json")
	renderedCrossArtifact := filepath.Join(t.TempDir(), "rendered-cross-platform.json")
	writeLiveEvidence(t, liveArtifact, validLiveEvidence(behaviorSHA))
	writeLiveProviderQualityEvidence(t, qualityArtifact, validLiveProviderQualityEvidence(behaviorSHA))
	writeRenderedEvidence(t, renderedArtifact, validRenderedEvidence(behaviorSHA))
	writeRenderedCrossEvidence(t, renderedCrossArtifact, validRenderedCrossEvidence(behaviorSHA, VerdictPass))
	write(t, workspace, "docs/V4-TIP-EVIDENCE.md", "# evidence only\n")
	candidateSHA := commitAll(t, workspace)

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: candidateSHA,
		BehaviorSHA:  behaviorSHA,
		Environ: func(key string) string {
			switch key {
			case LiveEvidenceEnv, LiveQualityEvidenceEnv, RenderedEvidenceEnv, RenderedCrossEvidenceEnv:
				return "1"
			case LiveArtifactEnv:
				return liveArtifact
			case LiveQualityArtifactEnv:
				return qualityArtifact
			case RenderedArtifactEnv:
				return renderedArtifact
			case RenderedCrossArtifactEnv:
				return renderedCrossArtifact
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.BehaviorProvenance != BehaviorProvenanceDocsOnlyDescendant {
		t.Fatalf("behavior provenance = %q", report.BehaviorProvenance)
	}
	if got := claimByID(t, report, "live-provider-connectivity").Verdict; got != VerdictPass {
		t.Fatalf("live-provider-connectivity = %s", got)
	}
	if got := claimByID(t, report, "live-provider-quality").Verdict; got != VerdictPass {
		t.Fatalf("live-provider-quality = %s", got)
	}
	if got := claimByID(t, report, "rendered-platform-local").Verdict; got != VerdictPass {
		t.Fatalf("rendered-platform-local = %s", got)
	}
	if got := claimByID(t, report, "rendered-cross-platform").Verdict; got != VerdictFail {
		t.Fatalf("rendered-cross-platform = %s", got)
	}
	if got := claimByID(t, report, "release-authorization").Verdict; got != VerdictUnverified {
		t.Fatalf("release-authorization = %s", got)
	}
}

func TestCollectRejectsNonDocsChangeAfterBehaviorSHA(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	behaviorSHA := commitAll(t, workspace)
	write(t, workspace, "main.go", "package main\n")
	candidateSHA := commitAll(t, workspace)

	_, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: candidateSHA,
		BehaviorSHA:  behaviorSHA,
	})
	if err == nil || !strings.Contains(err.Error(), "non-docs path") {
		t.Fatalf("expected non-docs provenance failure, got %v", err)
	}
}

func TestCollectRejectsRevertedNonDocsHistoryAfterBehaviorSHA(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	behaviorSHA := commitAll(t, workspace)
	write(t, workspace, "main.go", "package main\n")
	_ = commitAll(t, workspace)
	git(t, workspace, "rm", "--quiet", "main.go")
	git(t, workspace, "commit", "--quiet", "-m", "revert code")
	write(t, workspace, "docs/V4-TIP-EVIDENCE.md", "# evidence only\n")
	candidateSHA := commitAll(t, workspace)

	_, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: candidateSHA,
		BehaviorSHA:  behaviorSHA,
	})
	if err == nil || !strings.Contains(err.Error(), "non-docs path") {
		t.Fatalf("expected reverted non-docs history failure, got %v", err)
	}
}

func TestCollectRejectsArtifactBoundToWrongSHAAtDocsOnlyDescendant(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	behaviorSHA := commitAll(t, workspace)
	write(t, workspace, "docs/V4-TIP-EVIDENCE.md", "# evidence only\n")
	candidateSHA := commitAll(t, workspace)
	artifact := filepath.Join(t.TempDir(), "live-provider.json")
	writeLiveEvidence(t, artifact, validLiveEvidence(candidateSHA))

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: candidateSHA,
		BehaviorSHA:  behaviorSHA,
		Environ: func(key string) string {
			switch key {
			case LiveEvidenceEnv:
				return "1"
			case LiveArtifactEnv:
				return artifact
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	connectivity := claimByID(t, report, "live-provider-connectivity")
	if connectivity.Verdict != VerdictFail || !strings.Contains(connectivity.Reason, "candidate_sha") {
		t.Fatalf("expected wrong artifact SHA failure, got %+v", connectivity)
	}
}

func TestCollectRejectsWrongBehaviorSHA(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	candidateSHA := commitAll(t, workspace)

	_, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: candidateSHA,
		BehaviorSHA:  strings.Repeat("b", 40),
	})
	if err == nil || !strings.Contains(err.Error(), "not a proven ancestor") {
		t.Fatalf("expected wrong behavior SHA failure, got %v", err)
	}
}

func seedDocs(t *testing.T, workspace string) {
	t.Helper()
	write(t, workspace, "go.mod", "module example.test/runtimeboundary\n\ngo 1.25\n")
	write(t, workspace, "docs/V4-RENDERED-LONG-HORIZON-EVIDENCE.md", "# rendered\n")
	write(t, workspace, "docs/V4-RENDERED-RECOVERY-FIXTURE.md", "# recovery\n")
	write(t, workspace, "docs/V4-RENDERED-RECOVERY-API-BOUNDARY.md", "# api-boundary\n")
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
