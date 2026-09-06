package verify

import (
	"strings"
	"testing"
)

func TestEvaluateReleaseAuthorizationRequiresCompleteEvidence(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	result := EvaluateReleaseAuthorization(releaseAuthorizationFixture(candidateSHA), candidateSHA)
	if result.Status != ManifestPass || !result.Authorized {
		t.Fatalf("result = %+v, want PASS and authorized", result)
	}
	if len(result.Blockers) != 0 || result.Reason != "" {
		t.Fatalf("passing result carried blockers: %+v", result)
	}
}

func TestEvaluateReleaseAuthorizationKeepsMissingEvidenceBounded(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name   string
		mutate func(*ReleaseAuthorizationInput)
		status ManifestStatus
		want   string
	}{
		{
			name: "live provider remains unverified",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.Matrix.Claims[0].Verdict = "UNVERIFIED"
			},
			status: ManifestUnverified,
			want:   "live-provider-quality",
		},
		{
			name: "hostile result is inconclusive",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.Matrix.Claims[2].Verdict = "INCONCLUSIVE"
			},
			status: ManifestInconclusive,
			want:   "hostile-filesystem-toctou",
		},
		{
			name: "operator approval is absent",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.OperatorApproval.Approved = false
			},
			status: ManifestUnverified,
			want:   "operator approval",
		},
		{
			name: "attestation is absent",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.AttestationStatus = ManifestUnverified
			},
			status: ManifestUnverified,
			want:   "trusted release attestation",
		},
		{
			name: "matrix is absent",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.Matrix = ReleaseAuthorizationMatrix{}
			},
			status: ManifestUnverified,
			want:   "runtime-boundary matrix is missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := releaseAuthorizationFixture(candidateSHA)
			tt.mutate(&input)
			result := EvaluateReleaseAuthorization(input, candidateSHA)
			if result.Status != tt.status || result.Authorized {
				t.Fatalf("result = %+v, want status %s and unauthorized", result, tt.status)
			}
			if !strings.Contains(result.Reason, tt.want) {
				t.Fatalf("reason = %q, want substring %q; blockers = %v", result.Reason, tt.want, result.Blockers)
			}
		})
	}
}

func TestEvaluateReleaseAuthorizationFailsClosedOnContradictions(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	const otherSHA = "fedcba9876543210fedcba9876543210fedcba98"
	tests := []struct {
		name   string
		mutate func(*ReleaseAuthorizationInput)
		want   string
	}{
		{
			name: "wrong event",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.Event = "pull_request"
			},
			want: "release event",
		},
		{
			name: "dirty manifest",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.Manifest.Head.Tree = "DIRTY"
			},
			want: "worktree is DIRTY",
		},
		{
			name: "stale matrix SHA",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.Matrix.CandidateSHA = otherSHA
			},
			want: "matrix candidate SHA",
		},
		{
			name: "malformed matrix claim",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.Matrix.Claims[1].ID = ""
			},
			want: "claim identity",
		},
		{
			name: "wrong operator scope",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.OperatorApproval.Scope = "runtime-evidence"
			},
			want: "operator approval scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := releaseAuthorizationFixture(candidateSHA)
			tt.mutate(&input)
			result := EvaluateReleaseAuthorization(input, candidateSHA)
			if result.Status != ManifestFail || result.Authorized {
				t.Fatalf("result = %+v, want FAIL and unauthorized", result)
			}
			if !strings.Contains(result.Reason, tt.want) {
				t.Fatalf("reason = %q, want substring %q; blockers = %v", result.Reason, tt.want, result.Blockers)
			}
		})
	}
}

func releaseAuthorizationFixture(candidateSHA string) ReleaseAuthorizationInput {
	return ReleaseAuthorizationInput{
		Event: "push",
		Ref:   releaseAuthorizationRef,
		Manifest: Manifest{
			Schema: ManifestSchema,
			Status: ManifestPass,
			Head: HeadEvidence{
				SHA:         candidateSHA,
				ExpectedSHA: candidateSHA,
				Match:       ManifestPass,
				Tree:        "CLEAN",
			},
			Checks: []CheckEvidence{{
				Status:   ManifestPass,
				Coverage: CoverageEvidence{Status: ManifestPass},
			}},
		},
		Gates: ReleaseGateLedger{
			Schema:       ReleaseGateSchema,
			CandidateSHA: candidateSHA,
			Event:        releaseAuthorizationEvent,
			Gates: []ReleaseGateRecord{
				{CandidateSHA: candidateSHA, Event: releaseAuthorizationEvent, Job: "test", OS: "matrix", Command: "go test ./...", Status: "PASS"},
				{CandidateSHA: candidateSHA, Event: releaseAuthorizationEvent, Job: "security", OS: "ubuntu-latest", Command: "govulncheck ./...", Status: "PASS"},
			},
		},
		RequiredJobs:      []string{"test", "security"},
		AttestationStatus: ManifestPass,
		Matrix: ReleaseAuthorizationMatrix{
			Schema:       releaseAuthorizationMatrixSchema,
			CandidateSHA: candidateSHA,
			HeadMatch:    "PASS",
			Tree:         "CLEAN",
			Claims: []ReleaseAuthorizationClaim{
				{ID: "live-provider-quality", Category: "live_provider", Verdict: "PASS", Provenance: "owned provider evidence"},
				{ID: "rendered-cross-platform", Category: "rendered_platform", Verdict: "PASS", Provenance: "owned browser evidence"},
				{ID: "hostile-filesystem-toctou", Category: "hostile_runtime", Verdict: "PASS", Provenance: "hostile writer evidence"},
				{ID: "restart-steer-undo-recovery", Category: "recovery_undo", Verdict: "PASS", Provenance: "fresh-process evidence"},
				{ID: "release-authorization", Category: "release_authorization", Verdict: "INCONCLUSIVE", Provenance: "separate release predicate"},
			},
		},
		OperatorApproval: ReleaseAuthorizationApproval{
			CandidateSHA: candidateSHA,
			Approved:     true,
			Actor:        "release-manager",
			Scope:        releaseAuthorizationScope,
		},
	}
}
