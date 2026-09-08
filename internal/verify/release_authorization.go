package verify

import (
	"fmt"
	"sort"
	"strings"
)

const (
	ReleaseAuthorizationSchema       = "picogent.release-authorization.v1"
	releaseAuthorizationMatrixSchema = "picogent.v4.runtime-boundary-matrix.v1"
	releaseAuthorizationEvent        = "push"
	releaseAuthorizationRef          = "refs/heads/main"
	releaseAuthorizationScope        = "v4-release"
)

const (
	releaseCategoryLiveProvider  = "live_provider"
	releaseCategoryRendered      = "rendered_platform"
	releaseCategoryHostile       = "hostile_runtime"
	releaseCategoryRecovery      = "recovery_undo"
	releaseCategoryAuthorization = "release_authorization"
)

type releaseAuthorizationClaimRule struct {
	category         string
	residual         bool
	authorizationRow bool
}

// The matrix schema is intentionally closed for release consumption. The
// runtime collector may expose optional rows, but a category-only check would
// let an invented PASS row stand in for the evidence row that the predicate
// actually requires.
var releaseAuthorizationClaimRules = map[string]releaseAuthorizationClaimRule{
	"live-provider-connectivity":       {category: releaseCategoryLiveProvider},
	"live-provider-quality":            {category: releaseCategoryLiveProvider},
	"rendered-platform-local":          {category: releaseCategoryRendered},
	"rendered-long-horizon-local":      {category: releaseCategoryRendered},
	"rendered-cross-platform":          {category: releaseCategoryRendered},
	"rendered-recovery-undo-reload":    {category: releaseCategoryRendered},
	"hostile-child-env-sanitization":   {category: releaseCategoryHostile},
	"hostile-filesystem-deterministic": {category: releaseCategoryHostile},
	"hostile-parent-swap-confinement":  {category: releaseCategoryHostile},
	"hostile-filesystem-toctou":        {category: releaseCategoryHostile, residual: true},
	"restart-steer-undo-recovery":      {category: releaseCategoryRecovery},
	"release-authorization":            {category: releaseCategoryAuthorization, authorizationRow: true},
}

var requiredReleaseAuthorizationClaims = []string{
	"live-provider-connectivity",
	"live-provider-quality",
	"rendered-cross-platform",
	"hostile-child-env-sanitization",
	"hostile-filesystem-deterministic",
	"hostile-parent-swap-confinement",
	"restart-steer-undo-recovery",
}

// ReleaseAuthorizationClaim is the bounded, package-local projection of one
// runtime-boundary matrix row. It intentionally avoids importing
// internal/runtimeboundary, which already depends on this package.
type ReleaseAuthorizationClaim struct {
	ID         string `json:"id"`
	Category   string `json:"category"`
	Verdict    string `json:"verdict"`
	Provenance string `json:"provenance"`
}

// ReleaseAuthorizationMatrix is the minimum matrix evidence needed by the
// release predicate. The release-authority row itself is excluded from the
// required categories because this predicate is its separate consumer.
type ReleaseAuthorizationMatrix struct {
	Schema       string                      `json:"schema"`
	CandidateSHA string                      `json:"candidate_sha"`
	HeadMatch    string                      `json:"head_match"`
	Tree         string                      `json:"tree"`
	Claims       []ReleaseAuthorizationClaim `json:"claims"`
}

// ReleaseAuthorizationApproval is an explicit operator decision input. The
// predicate records the decision but does not authenticate the actor; a
// trusted workflow must establish that provenance before passing it here.
type ReleaseAuthorizationApproval struct {
	CandidateSHA string `json:"candidate_sha"`
	Approved     bool   `json:"approved"`
	Actor        string `json:"actor"`
	Scope        string `json:"scope"`
}

// ReleaseAuthorizationInput combines independent evidence sources. A green
// CI ledger, runtime matrix, signed attestation, and operator approval are all
// required; none is allowed to substitute for another.
type ReleaseAuthorizationInput struct {
	Event             string                       `json:"event"`
	Ref               string                       `json:"ref"`
	Manifest          Manifest                     `json:"manifest"`
	Gates             ReleaseGateLedger            `json:"gates"`
	RequiredJobs      []string                     `json:"required_jobs"`
	Matrix            ReleaseAuthorizationMatrix   `json:"matrix"`
	AttestationStatus ManifestStatus               `json:"attestation_status"`
	OperatorApproval  ReleaseAuthorizationApproval `json:"operator_approval"`
}

// ReleaseAuthorizationResult is an eligibility result, not a publish or
// deployment action. Authorized is true only for a complete PASS result.
type ReleaseAuthorizationResult struct {
	Schema     string         `json:"schema"`
	Status     ManifestStatus `json:"status"`
	Authorized bool           `json:"authorized"`
	Reason     string         `json:"reason,omitempty"`
	Blockers   []string       `json:"blockers,omitempty"`
}

type releaseAuthorizationBlocker struct {
	status ManifestStatus
	reason string
}

// EvaluateReleaseAuthorization evaluates the independent release gates for
// one exact candidate SHA. Missing evidence stays UNVERIFIED, inconclusive
// evidence stays INCONCLUSIVE, and malformed or contradictory evidence is
// FAIL. No result authorizes a release unless every required input is PASS.
func EvaluateReleaseAuthorization(input ReleaseAuthorizationInput, expectedSHA string) ReleaseAuthorizationResult {
	result := ReleaseAuthorizationResult{
		Schema: ReleaseAuthorizationSchema,
		Status: ManifestPass,
	}
	blockers := make([]releaseAuthorizationBlocker, 0, 8)
	add := func(status ManifestStatus, reason string) {
		if strings.TrimSpace(reason) == "" {
			return
		}
		blockers = append(blockers, releaseAuthorizationBlocker{status: status, reason: reason})
	}

	expectedSHA = strings.TrimSpace(expectedSHA)
	if !validManifestCommitID(expectedSHA) {
		add(ManifestFail, "expected release candidate SHA is not a full commit ID")
	}

	switch strings.TrimSpace(input.Event) {
	case "":
		add(ManifestUnverified, "release event is missing")
	case releaseAuthorizationEvent:
	default:
		add(ManifestFail, fmt.Sprintf("release event %q is not %q", input.Event, releaseAuthorizationEvent))
	}
	switch strings.TrimSpace(input.Ref) {
	case "":
		add(ManifestUnverified, "release ref is missing")
	case releaseAuthorizationRef:
	default:
		add(ManifestFail, fmt.Sprintf("release ref %q is not %q", input.Ref, releaseAuthorizationRef))
	}

	manifestStatus, manifestReason := releaseAuthorizationManifest(input.Manifest, expectedSHA)
	add(manifestStatus, manifestReason)

	if len(input.Gates.Gates) == 0 {
		add(ManifestUnverified, "release gate ledger is missing")
	} else if err := ValidateReleaseGateLedger(input.Gates, expectedSHA, releaseAuthorizationEvent, input.RequiredJobs); err != nil {
		add(ManifestFail, "release gates: "+err.Error())
	}

	matrixStatus, matrixReason := releaseAuthorizationMatrix(input.Matrix, expectedSHA)
	add(matrixStatus, matrixReason)

	switch input.AttestationStatus {
	case ManifestPass:
	case ManifestFail:
		add(ManifestFail, "release attestation failed")
	case ManifestInconclusive:
		add(ManifestInconclusive, "release attestation is inconclusive")
	case ManifestUnverified, ManifestSkipped, "":
		add(ManifestUnverified, "trusted release attestation is missing")
	default:
		add(ManifestFail, "release attestation has an invalid status")
	}

	approval := input.OperatorApproval
	switch {
	case !approval.Approved:
		add(ManifestUnverified, "explicit operator approval is missing")
	case !validManifestCommitID(strings.TrimSpace(approval.CandidateSHA)):
		add(ManifestUnverified, "operator approval candidate SHA is missing")
	case !strings.EqualFold(strings.TrimSpace(approval.CandidateSHA), expectedSHA):
		add(ManifestFail, "operator approval candidate SHA does not match expected SHA")
	case strings.TrimSpace(approval.Actor) == "":
		add(ManifestUnverified, "operator approval actor is missing")
	case strings.TrimSpace(approval.Scope) != releaseAuthorizationScope:
		add(ManifestFail, fmt.Sprintf("operator approval scope %q is not %q", approval.Scope, releaseAuthorizationScope))
	}

	if len(blockers) == 0 {
		result.Authorized = true
		return result
	}

	result.Blockers = make([]string, 0, len(blockers))
	statusRank := map[ManifestStatus]int{
		ManifestUnverified:   1,
		ManifestInconclusive: 2,
		ManifestFail:         3,
	}
	for _, blocker := range blockers {
		result.Blockers = append(result.Blockers, blocker.reason)
		if statusRank[blocker.status] > statusRank[result.Status] {
			result.Status = blocker.status
		}
	}
	sort.Strings(result.Blockers)
	result.Reason = result.Blockers[0]
	return result
}

func releaseAuthorizationManifest(manifest Manifest, expectedSHA string) (ManifestStatus, string) {
	if manifest.Schema != ManifestSchema {
		return ManifestFail, fmt.Sprintf("verification manifest schema %q is unsupported", manifest.Schema)
	}
	if !validManifestCommitID(expectedSHA) {
		return ManifestFail, "verification manifest cannot bind to an invalid expected SHA"
	}
	if !validManifestCommitID(manifest.Head.SHA) || !validManifestCommitID(manifest.Head.ExpectedSHA) {
		return ManifestUnverified, "verification manifest exact-head provenance is missing"
	}
	if !strings.EqualFold(manifest.Head.SHA, expectedSHA) || !strings.EqualFold(manifest.Head.ExpectedSHA, expectedSHA) {
		return ManifestFail, "verification manifest exact-head SHA does not match expected SHA"
	}
	if manifest.Head.Match != ManifestPass {
		return manifestEvidenceStatus(manifest.Head.Match, "verification manifest HEAD provenance is not PASS")
	}
	if manifest.Head.Tree != "CLEAN" {
		if manifest.Head.Tree == "DIRTY" {
			return ManifestFail, "verification manifest worktree is DIRTY"
		}
		return ManifestUnverified, "verification manifest worktree cleanliness is unverified"
	}
	switch manifest.Status {
	case ManifestPass:
	case ManifestFail:
		return ManifestFail, "verification manifest failed"
	case ManifestInconclusive:
		return ManifestInconclusive, "verification manifest is inconclusive"
	case ManifestSkipped:
		return ManifestUnverified, "verification manifest was skipped"
	default:
		return ManifestUnverified, "verification manifest is unverified"
	}
	if manifest.ChecksTruncated {
		return ManifestUnverified, "verification manifest checks are truncated"
	}
	if len(manifest.Checks) == 0 {
		return ManifestUnverified, "verification manifest has no checks"
	}
	for _, check := range manifest.Checks {
		if status, reason := manifestEvidenceStatus(check.Status, "verification check is not PASS"); status != ManifestPass {
			return status, reason
		}
		if check.OutputTruncated {
			return ManifestUnverified, "verification check output is truncated"
		}
		if !coverageRequiredForManifest(check) {
			continue
		}
		if status, reason := manifestEvidenceStatus(check.Coverage.Status, "verification check coverage is not PASS"); status != ManifestPass {
			return status, reason
		}
	}
	return ManifestPass, ""
}

func releaseAuthorizationMatrix(matrix ReleaseAuthorizationMatrix, expectedSHA string) (ManifestStatus, string) {
	if matrix.Schema == "" && len(matrix.Claims) == 0 {
		return ManifestUnverified, "runtime-boundary matrix is missing"
	}
	if matrix.Schema != releaseAuthorizationMatrixSchema {
		return ManifestFail, fmt.Sprintf("runtime-boundary matrix schema %q is unsupported", matrix.Schema)
	}
	if !validManifestCommitID(matrix.CandidateSHA) {
		return ManifestUnverified, "runtime-boundary matrix candidate SHA is missing"
	}
	if !strings.EqualFold(matrix.CandidateSHA, expectedSHA) {
		return ManifestFail, "runtime-boundary matrix candidate SHA does not match expected SHA"
	}
	if status, reason := manifestEvidenceStatus(ManifestStatus(matrix.HeadMatch), "runtime-boundary matrix HEAD provenance is not PASS"); status != ManifestPass {
		return status, reason
	}
	if matrix.Tree != "CLEAN" {
		if matrix.Tree == "DIRTY" {
			return ManifestFail, "runtime-boundary matrix worktree is DIRTY"
		}
		return ManifestUnverified, "runtime-boundary matrix worktree cleanliness is unverified"
	}
	if len(matrix.Claims) == 0 {
		return ManifestUnverified, "runtime-boundary matrix has no claims"
	}
	seen := make(map[string]struct{}, len(matrix.Claims))
	for _, claim := range matrix.Claims {
		id := strings.TrimSpace(claim.ID)
		category := strings.TrimSpace(claim.Category)
		if id == "" || category == "" {
			return ManifestFail, "runtime-boundary matrix claim identity is incomplete"
		}
		if _, exists := seen[id]; exists {
			return ManifestFail, fmt.Sprintf("runtime-boundary matrix claim %q is duplicated", id)
		}
		seen[id] = struct{}{}
		rule, known := releaseAuthorizationClaimRules[id]
		if !known {
			return ManifestFail, fmt.Sprintf("runtime-boundary matrix claim %q is unsupported", id)
		}
		if category != rule.category {
			return ManifestFail, fmt.Sprintf("runtime-boundary matrix claim %q has category %q, want %q", id, category, rule.category)
		}
		if strings.TrimSpace(claim.Provenance) == "" {
			return ManifestFail, fmt.Sprintf("runtime-boundary matrix claim %q has no provenance", id)
		}
		verdict := ManifestStatus(strings.TrimSpace(claim.Verdict))
		if verdict != ManifestPass && verdict != ManifestFail && verdict != ManifestInconclusive && verdict != ManifestUnverified {
			return ManifestFail, fmt.Sprintf("runtime-boundary matrix claim %q has invalid verdict %q", id, claim.Verdict)
		}
		if rule.authorizationRow {
			// This row is intentionally consumed by this separate predicate;
			// requiring it to already be PASS would make the contract recursive.
			if verdict == ManifestFail {
				return ManifestFail, "runtime-boundary matrix release-authorization claim is FAIL"
			}
			continue
		}
		if rule.residual {
			// Residual broad TOCTOU stays visible but is not an unreachable
			// universal PASS gate. A recorded FAIL is still contradictory;
			// UNVERIFIED and INCONCLUSIVE remain explicit accepted boundaries.
			if verdict == ManifestFail {
				return ManifestFail, "runtime-boundary matrix claim \"hostile-filesystem-toctou\" is FAIL"
			}
			continue
		}
		if status, reason := manifestEvidenceStatus(verdict, fmt.Sprintf("runtime-boundary matrix claim %q in category %q is not PASS", id, category)); status != ManifestPass {
			return status, reason
		}
	}
	for _, id := range requiredReleaseAuthorizationClaims {
		if _, exists := seen[id]; !exists {
			return ManifestUnverified, fmt.Sprintf("runtime-boundary matrix claim %q is missing", id)
		}
	}
	return ManifestPass, ""
}

func manifestEvidenceStatus(status ManifestStatus, reason string) (ManifestStatus, string) {
	switch status {
	case ManifestPass:
		return ManifestPass, ""
	case ManifestFail:
		return ManifestFail, reason
	case ManifestInconclusive:
		return ManifestInconclusive, reason
	case ManifestSkipped, ManifestUnverified, "":
		return ManifestUnverified, reason
	default:
		return ManifestFail, reason
	}
}
