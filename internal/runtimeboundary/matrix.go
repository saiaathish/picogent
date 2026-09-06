// Package runtimeboundary records a bounded evidence matrix for remaining
// release-readiness runtime claims. It does not authorize a release and never
// upgrades missing live-provider or unsupported-platform evidence to PASS.
package runtimeboundary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/verify"
)

const (
	Schema                 = "picogent.v4.runtime-boundary-matrix.v1"
	MaxReportBytes         = 48 << 10
	MaxClaims              = 32
	MaxTextBytes           = 512
	LiveEvidenceEnv        = "PICOGENT_LIVE_PROVIDER_EVIDENCE"
	LiveArtifactEnv        = "PICOGENT_LIVE_PROVIDER_ARTIFACT"
	LiveQualityEvidenceEnv = LiveProviderQualityEvidenceEnv
	LiveQualityArtifactEnv = LiveProviderQualityArtifactEnv
)

// Verdict is the fail-closed observation vocabulary for one claim.
type Verdict string

const (
	VerdictPass         Verdict = "PASS"
	VerdictFail         Verdict = "FAIL"
	VerdictInconclusive Verdict = "INCONCLUSIVE"
	VerdictUnverified   Verdict = "UNVERIFIED"
)

func (v Verdict) valid() bool {
	switch v {
	case VerdictPass, VerdictFail, VerdictInconclusive, VerdictUnverified:
		return true
	default:
		return false
	}
}

// Category groups related runtime claims.
type Category string

const (
	CategoryLiveProvider Category = "live_provider"
	CategoryRendered     Category = "rendered_platform"
	CategoryHostile      Category = "hostile_runtime"
	CategoryRecovery     Category = "recovery_undo"
	CategoryRelease      Category = "release_authorization"
)

// Claim is one matrix row with observable setup, artifact, and provenance.
type Claim struct {
	ID         string   `json:"id"`
	Category   Category `json:"category"`
	Title      string   `json:"title"`
	Setup      string   `json:"setup"`
	Artifact   string   `json:"artifact"`
	Verdict    Verdict  `json:"verdict"`
	Provenance string   `json:"provenance"`
	Reason     string   `json:"reason,omitempty"`
	ObservedAt string   `json:"observed_at,omitempty"`
}

// Report is the exact-SHA-bound matrix artifact.
type Report struct {
	Schema       string         `json:"schema"`
	CandidateSHA string         `json:"candidate_sha"`
	HeadMatch    string         `json:"head_match"`
	Tree         string         `json:"tree"`
	HostOS       string         `json:"host_os"`
	HostArch     string         `json:"host_arch"`
	GoVersion    string         `json:"go_version"`
	GeneratedAt  string         `json:"generated_at"`
	Claims       []Claim        `json:"claims"`
	Summary      map[string]int `json:"summary"`
	Unverified   []string       `json:"unverified,omitempty"`
	Reason       string         `json:"reason,omitempty"`
}

// Options configures one matrix collection.
type Options struct {
	Workspace    string
	CandidateSHA string
	Now          time.Time
	// Lookup lets tests stub filesystem existence checks.
	Lookup func(path string) (bool, error)
	// Environ lets tests stub environment lookups.
	Environ func(key string) string
}

// Collect builds the bounded matrix for the workspace at the expected SHA.
func Collect(opts Options) (Report, error) {
	report := Report{
		Schema:       Schema,
		CandidateSHA: strings.TrimSpace(opts.CandidateSHA),
		HostOS:       runtime.GOOS,
		HostArch:     runtime.GOARCH,
		GoVersion:    runtime.Version(),
		Summary:      map[string]int{},
	}
	if !validCommitSHA(report.CandidateSHA) {
		return report, errors.New("candidate_sha must be a full commit id")
	}
	workspace, err := filepath.Abs(strings.TrimSpace(opts.Workspace))
	if err != nil {
		return report, fmt.Errorf("resolve workspace: %w", err)
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	report.GeneratedAt = now.UTC().Format(time.RFC3339)

	provenance := verify.CollectProvenance(context.Background(), workspace, report.CandidateSHA)
	report.HeadMatch = string(provenance.Match)
	report.Tree = provenance.Tree
	if provenance.Match != verify.ManifestPass {
		report.Reason = firstNonEmpty(provenance.Reason, "HEAD provenance is unverified")
		return report, fmt.Errorf("candidate provenance: %s", report.Reason)
	}
	if provenance.Tree != "CLEAN" {
		report.Reason = "workspace is not clean"
		return report, errors.New(report.Reason)
	}

	lookup := opts.Lookup
	if lookup == nil {
		lookup = func(path string) (bool, error) {
			_, err := os.Stat(path)
			if err == nil {
				return true, nil
			}
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			return false, err
		}
	}
	environ := opts.Environ
	if environ == nil {
		environ = os.Getenv
	}

	claims := defaultClaims(workspace, report.CandidateSHA, now, lookup, environ)
	if len(claims) > MaxClaims {
		return report, errors.New("runtime boundary matrix exceeds claim cap")
	}
	for i := range claims {
		claims[i] = boundClaim(claims[i])
		if !claims[i].Verdict.valid() {
			return report, fmt.Errorf("claim %s has invalid verdict", claims[i].ID)
		}
		report.Summary[string(claims[i].Verdict)]++
		if claims[i].Verdict == VerdictUnverified {
			report.Unverified = append(report.Unverified, claims[i].ID)
		}
	}
	sort.Slice(claims, func(i, j int) bool { return claims[i].ID < claims[j].ID })
	report.Claims = claims
	sort.Strings(report.Unverified)
	return report, nil
}

func defaultClaims(workspace, sha string, now time.Time, lookup func(string) (bool, error), environ func(string) string) []Claim {
	observed := now.UTC().Format(time.RFC3339)
	doc := func(rel string) string { return filepath.Join(workspace, filepath.FromSlash(rel)) }

	liveConnectivity := Claim{
		ID:         "live-provider-connectivity",
		Category:   CategoryLiveProvider,
		Title:      "Direct connectivity to a real authenticated provider",
		Setup:      "Task-owned disposable state/workspace with a fixed no-tool prompt and secret-free digests.",
		Artifact:   "live-provider evidence JSON referenced by " + LiveArtifactEnv,
		Verdict:    VerdictUnverified,
		Reason:     "no live-provider connectivity evidence artifact was supplied",
		ObservedAt: observed,
	}
	live := Claim{
		ID:         "live-provider-quality",
		Category:   CategoryLiveProvider,
		Title:      "Bounded live-provider quality observation (self-reported artifact)",
		Setup:      "Task-owned live provider session running the fixed no-tool campaign with canonical prompt digests and digest-only evidence; provider identity and raw result semantics are not independently attested.",
		Artifact:   "live-provider quality evidence JSON referenced by " + LiveQualityArtifactEnv,
		Verdict:    VerdictUnverified,
		Reason:     "no live-provider quality evidence artifact was supplied",
		ObservedAt: observed,
	}
	if strings.TrimSpace(environ(LiveEvidenceEnv)) == "1" {
		artifact := strings.TrimSpace(environ(LiveArtifactEnv))
		if artifact == "" {
			liveConnectivity.Verdict = VerdictFail
			liveConnectivity.Reason = "live evidence requested without " + LiveArtifactEnv
		} else if ok, err := lookup(artifact); err != nil {
			liveConnectivity.Verdict = VerdictInconclusive
			liveConnectivity.Reason = "live evidence artifact lookup failed"
		} else if !ok {
			liveConnectivity.Verdict = VerdictFail
			liveConnectivity.Reason = "live evidence artifact is missing"
			liveConnectivity.Artifact = artifact
		} else {
			evidence, digest, err := loadLiveProviderEvidence(workspace, artifact, sha)
			if err != nil {
				liveConnectivity.Verdict = VerdictFail
				liveConnectivity.Reason = "live evidence validation failed: " + err.Error()
				liveConnectivity.Artifact = artifact
			} else {
				liveConnectivity.Verdict = VerdictPass
				liveConnectivity.Artifact = artifact
				liveConnectivity.Provenance = "env:" + LiveEvidenceEnv + "+sha256:" + digest
				liveConnectivity.ObservedAt = evidence.ObservedAt
				liveConnectivity.Reason = "direct fixed-prompt provider response was recorded without tools or mutation"
			}
		}
	} else {
		liveConnectivity.Provenance = "fail-closed default without " + LiveEvidenceEnv
	}
	if strings.TrimSpace(environ(LiveQualityEvidenceEnv)) == "1" {
		artifact := strings.TrimSpace(environ(LiveQualityArtifactEnv))
		if artifact == "" {
			live.Verdict = VerdictFail
			live.Reason = "live-provider quality evidence requested without " + LiveQualityArtifactEnv
		} else if ok, err := lookup(artifact); err != nil {
			live.Verdict = VerdictInconclusive
			live.Artifact = artifact
			live.Reason = "live-provider quality evidence artifact lookup failed"
		} else if !ok {
			live.Verdict = VerdictFail
			live.Artifact = artifact
			live.Reason = "live-provider quality evidence artifact is missing"
		} else {
			evidence, digest, err := loadLiveProviderQualityEvidence(workspace, artifact, sha)
			if err != nil {
				live.Verdict = VerdictFail
				live.Artifact = artifact
				live.Reason = "live-provider quality evidence validation failed: " + err.Error()
			} else {
				live.Verdict = evidence.Verdict
				live.Artifact = artifact
				live.Provenance = "env:" + LiveQualityEvidenceEnv + "+sha256:" + digest
				live.ObservedAt = evidence.ObservedAt
				switch evidence.Verdict {
				case VerdictPass:
					live.Reason = "self-reported fixed no-tool quality artifact passed canonical prompt-digest and bounded case checks; provider identity and raw result semantics are not independently attested"
				case VerdictFail:
					live.Reason = "fixed no-tool quality campaign recorded a failed case or safety assertion"
				case VerdictInconclusive:
					live.Reason = "fixed no-tool quality campaign is inconclusive because provider availability or case coverage is incomplete"
				default:
					live.Reason = "fixed no-tool quality campaign remains unverified"
				}
			}
		}
	} else {
		live.Provenance = "fail-closed default without " + LiveQualityEvidenceEnv
	}

	renderedPlatform, renderedArchitecture := currentRenderedPlatform()
	renderedLocal := Claim{
		ID:         "rendered-platform-local",
		Category:   CategoryRendered,
		Title:      "Task-owned rendered evidence on the current platform",
		Setup:      "Task-owned disposable rendered fixture with exact source SHA and digest-only observation references.",
		Artifact:   "rendered-platform evidence JSON referenced by " + RenderedArtifactEnv,
		Verdict:    VerdictUnverified,
		Reason:     "no rendered-platform evidence artifact was supplied",
		ObservedAt: observed,
	}
	if strings.TrimSpace(environ(RenderedEvidenceEnv)) == "1" {
		artifact := strings.TrimSpace(environ(RenderedArtifactEnv))
		if artifact == "" {
			renderedLocal.Verdict = VerdictFail
			renderedLocal.Reason = "rendered evidence requested without " + RenderedArtifactEnv
		} else if ok, err := lookup(artifact); err != nil {
			renderedLocal.Verdict = VerdictInconclusive
			renderedLocal.Reason = "rendered evidence artifact lookup failed"
		} else if !ok {
			renderedLocal.Verdict = VerdictFail
			renderedLocal.Artifact = artifact
			renderedLocal.Reason = "rendered evidence artifact is missing"
		} else {
			evidence, digest, loadErr := loadRenderedPlatformEvidence(workspace, artifact, sha, renderedPlatform, renderedArchitecture)
			if loadErr != nil {
				renderedLocal.Verdict = VerdictFail
				renderedLocal.Artifact = artifact
				renderedLocal.Reason = "rendered evidence validation failed: " + loadErr.Error()
			} else {
				renderedLocal.Verdict = evidence.Verdict
				renderedLocal.Artifact = artifact
				renderedLocal.Provenance = "env:" + RenderedEvidenceEnv + "+sha256:" + digest
				renderedLocal.ObservedAt = evidence.ObservedAt
				renderedLocal.Reason = "direct task-owned rendered observation recorded for " + renderedPlatform + "/" + renderedArchitecture
			}
		}
	} else {
		renderedLocal.Provenance = "fail-closed default without " + RenderedEvidenceEnv
	}

	renderedExists, _ := lookup(doc("docs/V4-RENDERED-LONG-HORIZON-EVIDENCE.md"))
	rendered := Claim{
		ID:         "rendered-long-horizon-local",
		Category:   CategoryRendered,
		Title:      "Rendered long-horizon fixture evidence on one owned browser session",
		Setup:      "Task-owned rendered_fixture + owned browser observation recorded in docs.",
		Artifact:   "docs/V4-RENDERED-LONG-HORIZON-EVIDENCE.md",
		Provenance: "docs + fixture contract",
		ObservedAt: observed,
	}
	if renderedExists {
		rendered.Verdict = VerdictPass
		rendered.Reason = "local rendered evidence doc is present; unsupported platforms remain outside this claim"
	} else {
		rendered.Verdict = VerdictUnverified
		rendered.Reason = "rendered long-horizon evidence doc is missing"
	}

	renderedCross := Claim{
		ID:         "rendered-cross-platform",
		Category:   CategoryRendered,
		Title:      "Rendered behavior across all supported desktop platforms",
		Setup:      "Repeat the rendered fixture on macOS, Windows, and Linux owned browsers.",
		Artifact:   "per-platform rendered evidence records",
		Verdict:    VerdictUnverified,
		Provenance: "no multi-platform rendered matrix artifact",
		Reason:     "only a single local rendered evidence path is catalogued",
		ObservedAt: observed,
	}

	recoveryFixtureDoc, _ := lookup(doc("docs/V4-RENDERED-RECOVERY-FIXTURE.md"))
	apiBoundaryDoc, _ := lookup(doc("docs/V4-RENDERED-RECOVERY-API-BOUNDARY.md"))
	renderedUndoReload := Claim{
		ID:         "rendered-recovery-undo-reload",
		Category:   CategoryRendered,
		Title:      "Rendered permission, undo, and fresh-process reload API boundary",
		Setup:      "Deterministic rendered_recovery fixture: allow mutation, undo, then reload.",
		Artifact:   "docs/V4-RENDERED-RECOVERY-API-BOUNDARY.md",
		Provenance: "automated API-boundary fixture test",
		ObservedAt: observed,
	}
	switch {
	case apiBoundaryDoc && recoveryFixtureDoc:
		renderedUndoReload.Verdict = VerdictPass
		renderedUndoReload.Reason = "deterministic allow→undo→reload API-boundary evidence is retained; browser DOM and live-provider remain outside this claim"
	case recoveryFixtureDoc:
		renderedUndoReload.Verdict = VerdictUnverified
		renderedUndoReload.Reason = "recovery fixture runbook exists, but automated allow→undo→reload API-boundary evidence is missing"
		renderedUndoReload.Provenance = "runbook only"
	default:
		renderedUndoReload.Verdict = VerdictUnverified
		renderedUndoReload.Reason = "rendered recovery fixture documentation is missing"
		renderedUndoReload.Provenance = "missing docs"
	}

	hostileChildEnvExists, _ := lookup(doc("docs/V4-SECURITY-CAMPAIGN.md"))
	hostile := Claim{
		ID:         "hostile-child-env-sanitization",
		Category:   CategoryHostile,
		Title:      "Non-interactive child environment sanitization",
		Setup:      "Deterministic procenv sanitization + MCP/env hostile tests.",
		Artifact:   "docs/V4-SECURITY-CAMPAIGN.md and internal/procenv coverage",
		Provenance: "merged security lanes",
		ObservedAt: observed,
	}
	if hostileChildEnvExists {
		hostile.Verdict = VerdictPass
		hostile.Reason = "bounded child-env sanitization evidence is documented; arbitrary same-UID TOCTOU remains outside this claim"
	} else {
		hostile.Verdict = VerdictUnverified
		hostile.Reason = "security campaign evidence doc is missing"
	}

	hostileFilesystemExists, _ := lookup(doc("docs/V4-HOSTILE-RUNTIME-EVIDENCE.md"))
	hostileFilesystem := Claim{
		ID:         "hostile-filesystem-deterministic",
		Category:   CategoryHostile,
		Title:      "Deterministic hostile-filesystem boundary",
		Setup:      "Bounded securefile, procenv, and workspace tests for path substitution, atomic cleanup identity, and child-environment sanitization.",
		Artifact:   "docs/V4-HOSTILE-RUNTIME-EVIDENCE.md and deterministic package tests",
		Provenance: "exact-head bounded hostile-runtime evidence",
		ObservedAt: observed,
	}
	if hostileFilesystemExists {
		hostileFilesystem.Verdict = VerdictPass
		hostileFilesystem.Reason = "bounded deterministic hostile-runtime evidence is documented; arbitrary same-UID filesystem TOCTOU remains outside this claim"
	} else {
		hostileFilesystem.Verdict = VerdictUnverified
		hostileFilesystem.Reason = "bounded hostile-runtime evidence doc is missing"
	}

	hostileTOCTOU := Claim{
		ID:         "hostile-filesystem-toctou",
		Category:   CategoryHostile,
		Title:      "Arbitrary same-UID filesystem TOCTOU races",
		Setup:      "Hostile writer between final check and mutate/exec across surfaces.",
		Artifact:   "cross-surface hostile TOCTOU stress evidence",
		Verdict:    VerdictUnverified,
		Provenance: "explicit audit boundary",
		Reason:     "broader filesystem race proof is not claimed by current deterministic coverage",
		ObservedAt: observed,
	}

	steeringDoc, _ := lookup(doc("docs/V4-LONG-HORIZON-OUTCOME.md"))
	recovery := Claim{
		ID:         "restart-steer-undo-recovery",
		Category:   CategoryRecovery,
		Title:      "Restart, steering, undo, and recovery deterministic contracts",
		Setup:      "Provider-independent long-horizon and undo/recovery fixtures.",
		Artifact:   "docs/V4-LONG-HORIZON-OUTCOME.md and undo/recovery contracts",
		Provenance: "deterministic fixtures",
		ObservedAt: observed,
	}
	if steeringDoc {
		recovery.Verdict = VerdictPass
		recovery.Reason = "deterministic restart/steer/undo contracts are documented; live-provider recovery remains outside this claim"
	} else {
		recovery.Verdict = VerdictUnverified
		recovery.Reason = "long-horizon outcome evidence doc is missing"
	}

	sbomDoc, _ := lookup(doc("docs/V4-SBOM-ARTIFACT-EVIDENCE.md"))
	release := Claim{
		ID:         "release-authorization",
		Category:   CategoryRelease,
		Title:      "Overall release authorization from exact-head evidence",
		Setup:      "Independent release audit + verification manifest + production artifacts.",
		Artifact:   "docs/V4-RELEASE-AUDIT.md",
		Provenance: "exact-head audit",
		ObservedAt: observed,
	}
	auditExists, _ := lookup(doc("docs/V4-RELEASE-AUDIT.md"))
	switch {
	case !auditExists:
		release.Verdict = VerdictUnverified
		release.Reason = "release audit document is missing"
	case sbomDoc:
		release.Verdict = VerdictInconclusive
		release.Reason = "production artifacts and audits exist, but overall release authorization remains inconclusive while live/rendered/hostile gaps persist"
		release.Provenance = "audit+sbom lane at " + sha[:12]
	default:
		release.Verdict = VerdictInconclusive
		release.Reason = "release audit exists without claiming authorization"
	}

	return []Claim{liveConnectivity, live, renderedLocal, rendered, renderedCross, renderedUndoReload, hostile, hostileFilesystem, hostileTOCTOU, recovery, release}
}

func boundClaim(claim Claim) Claim {
	claim.ID = boundText(claim.ID)
	claim.Title = boundText(claim.Title)
	claim.Setup = boundText(claim.Setup)
	claim.Artifact = boundText(claim.Artifact)
	claim.Provenance = boundText(claim.Provenance)
	claim.Reason = boundText(claim.Reason)
	claim.ObservedAt = boundText(claim.ObservedAt)
	return claim
}

func boundText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= MaxTextBytes {
		return value
	}
	return value[:MaxTextBytes]
}

// WriteJSON writes the size-bounded matrix report.
func WriteJSON(w io.Writer, report Report) error {
	if w == nil {
		return errors.New("runtime boundary writer is nil")
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > MaxReportBytes {
		return errors.New("runtime boundary matrix exceeds size limit")
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func validCommitSHA(sha string) bool {
	if len(sha) != 40 {
		return false
	}
	for _, r := range sha {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
