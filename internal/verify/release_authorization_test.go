package verify

import (
	"strings"
	"testing"
)

func TestEvaluateReleaseAuthorizationAcceptsTargetedOnlyCoverage(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	input := releaseAuthorizationFixture(candidateSHA)
	input.Manifest.Checks = []CheckEvidence{
		{
			Scope:    ScopeTargeted,
			Status:   ManifestPass,
			Coverage: CoverageEvidence{Status: ManifestPass},
		},
		{
			Scope:    ScopeBroader,
			Status:   ManifestPass,
			Coverage: CoverageEvidence{Status: ManifestUnverified, Reason: "coverage not collected"},
		},
	}
	result := EvaluateReleaseAuthorization(input, candidateSHA)
	if result.Status != ManifestPass || !result.Authorized {
		t.Fatalf("result = %+v, want PASS under targeted-only coverage", result)
	}
}

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

func TestEvaluateReleaseAuthorizationRequiresTargetedCoverage(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	input := releaseAuthorizationFixture(candidateSHA)
	input.Manifest.Checks = []CheckEvidence{
		{
			Scope:    ScopeTargeted,
			Status:   ManifestPass,
			Coverage: CoverageEvidence{Status: ManifestUnverified, Reason: "coverage not collected"},
		},
		{
			Scope:    ScopeBroader,
			Status:   ManifestPass,
			Coverage: CoverageEvidence{Status: ManifestUnverified, Reason: "coverage not collected"},
		},
	}
	result := EvaluateReleaseAuthorization(input, candidateSHA)
	if result.Status != ManifestUnverified || result.Authorized {
		t.Fatalf("result = %+v, want UNVERIFIED without targeted coverage", result)
	}
	if !strings.Contains(result.Reason, "coverage") {
		t.Fatalf("reason = %q, want coverage blocker", result.Reason)
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
				setReleaseClaimVerdict(t, input, "live-provider-quality", "UNVERIFIED")
			},
			status: ManifestUnverified,
			want:   "live-provider-quality",
		},
		{
			name: "hostile parent-swap is inconclusive",
			mutate: func(input *ReleaseAuthorizationInput) {
				setReleaseClaimVerdict(t, input, "hostile-parent-swap-confinement", "INCONCLUSIVE")
			},
			status: ManifestInconclusive,
			want:   "hostile-parent-swap-confinement",
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

func TestEvaluateReleaseAuthorizationIgnoresResidualBroadTOCTOU(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	input := releaseAuthorizationFixture(candidateSHA)
	input.Matrix.Claims = append(input.Matrix.Claims, ReleaseAuthorizationClaim{
		ID:         "hostile-filesystem-toctou",
		Category:   "hostile_runtime",
		Verdict:    "UNVERIFIED",
		Provenance: "explicit residual audit boundary",
	})
	result := EvaluateReleaseAuthorization(input, candidateSHA)
	if result.Status != ManifestPass || !result.Authorized {
		t.Fatalf("result = %+v, want PASS and authorized with residual TOCTOU UNVERIFIED", result)
	}
}

func TestEvaluateReleaseAuthorizationRequiresCanonicalMatrixClaims(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	required := []string{
		"live-provider-connectivity",
		"live-provider-quality",
		"rendered-cross-platform",
		"hostile-child-env-sanitization",
		"hostile-filesystem-deterministic",
		"hostile-parent-swap-confinement",
		"restart-steer-undo-recovery",
	}
	for _, id := range required {
		t.Run("missing "+id, func(t *testing.T) {
			input := releaseAuthorizationFixture(candidateSHA)
			removeReleaseClaim(t, &input, id)
			result := EvaluateReleaseAuthorization(input, candidateSHA)
			if result.Status != ManifestUnverified || result.Authorized {
				t.Fatalf("result = %+v, want UNVERIFIED and unauthorized", result)
			}
			if !strings.Contains(result.Reason, id) {
				t.Fatalf("reason = %q, want missing claim %q; blockers = %v", result.Reason, id, result.Blockers)
			}
		})
	}

	t.Run("substituted claim does not satisfy canonical identity", func(t *testing.T) {
		input := releaseAuthorizationFixture(candidateSHA)
		setReleaseClaimID(t, &input, "live-provider-connectivity", "invented-live-provider")
		result := EvaluateReleaseAuthorization(input, candidateSHA)
		if result.Status != ManifestFail || result.Authorized {
			t.Fatalf("result = %+v, want FAIL and unauthorized", result)
		}
		if !strings.Contains(result.Reason, "unsupported") {
			t.Fatalf("reason = %q, want unsupported claim; blockers = %v", result.Reason, result.Blockers)
		}
	})

	t.Run("wrong category does not satisfy canonical identity", func(t *testing.T) {
		input := releaseAuthorizationFixture(candidateSHA)
		setReleaseClaimCategory(t, &input, "live-provider-connectivity", "recovery_undo")
		result := EvaluateReleaseAuthorization(input, candidateSHA)
		if result.Status != ManifestFail || result.Authorized {
			t.Fatalf("result = %+v, want FAIL and unauthorized", result)
		}
		if !strings.Contains(result.Reason, "category") {
			t.Fatalf("reason = %q, want category contradiction; blockers = %v", result.Reason, result.Blockers)
		}
	})
}

func TestEvaluateReleaseAuthorizationValidatesOptionalMatrixRows(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"

	t.Run("residual unverified remains an explicit accepted boundary", func(t *testing.T) {
		input := releaseAuthorizationFixture(candidateSHA)
		input.Matrix.Claims = append(input.Matrix.Claims, ReleaseAuthorizationClaim{
			ID:         "hostile-filesystem-toctou",
			Category:   "hostile_runtime",
			Verdict:    "UNVERIFIED",
			Provenance: "explicit residual audit boundary",
		})
		result := EvaluateReleaseAuthorization(input, candidateSHA)
		if result.Status != ManifestPass || !result.Authorized {
			t.Fatalf("result = %+v, want PASS and authorized", result)
		}
	})

	tests := []struct {
		name   string
		mutate func(*ReleaseAuthorizationInput)
		want   string
	}{
		{
			name: "residual failure",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.Matrix.Claims = append(input.Matrix.Claims, ReleaseAuthorizationClaim{
					ID: "hostile-filesystem-toctou", Category: "hostile_runtime", Verdict: "FAIL", Provenance: "hostile observation",
				})
			},
			want: "hostile-filesystem-toctou",
		},
		{
			name: "unknown verdict",
			mutate: func(input *ReleaseAuthorizationInput) {
				input.Matrix.Claims = append(input.Matrix.Claims, ReleaseAuthorizationClaim{
					ID: "hostile-filesystem-toctou", Category: "hostile_runtime", Verdict: "SKIPPED", Provenance: "explicit residual audit boundary",
				})
			},
			want: "invalid verdict",
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
				setReleaseClaimID(t, input, "live-provider-quality", "")
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
				{ID: "live-provider-connectivity", Category: "live_provider", Verdict: "PASS", Provenance: "owned provider evidence"},
				{ID: "live-provider-quality", Category: "live_provider", Verdict: "PASS", Provenance: "owned provider evidence"},
				{ID: "rendered-cross-platform", Category: "rendered_platform", Verdict: "PASS", Provenance: "owned browser evidence"},
				{ID: "hostile-child-env-sanitization", Category: "hostile_runtime", Verdict: "PASS", Provenance: "merged security evidence"},
				{ID: "hostile-filesystem-deterministic", Category: "hostile_runtime", Verdict: "PASS", Provenance: "bounded hostile evidence"},
				{ID: "hostile-parent-swap-confinement", Category: "hostile_runtime", Verdict: "PASS", Provenance: "bounded parent-swap evidence"},
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

func setReleaseClaimVerdict(t *testing.T, input *ReleaseAuthorizationInput, id, verdict string) {
	t.Helper()
	for i := range input.Matrix.Claims {
		if input.Matrix.Claims[i].ID == id {
			input.Matrix.Claims[i].Verdict = verdict
			return
		}
	}
	t.Fatalf("matrix claim %q not found", id)
}

func setReleaseClaimID(t *testing.T, input *ReleaseAuthorizationInput, id, replacement string) {
	t.Helper()
	for i := range input.Matrix.Claims {
		if input.Matrix.Claims[i].ID == id {
			input.Matrix.Claims[i].ID = replacement
			return
		}
	}
	t.Fatalf("matrix claim %q not found", id)
}

func setReleaseClaimCategory(t *testing.T, input *ReleaseAuthorizationInput, id, category string) {
	t.Helper()
	for i := range input.Matrix.Claims {
		if input.Matrix.Claims[i].ID == id {
			input.Matrix.Claims[i].Category = category
			return
		}
	}
	t.Fatalf("matrix claim %q not found", id)
}

func removeReleaseClaim(t *testing.T, input *ReleaseAuthorizationInput, id string) {
	t.Helper()
	for i := range input.Matrix.Claims {
		if input.Matrix.Claims[i].ID == id {
			input.Matrix.Claims = append(input.Matrix.Claims[:i], input.Matrix.Claims[i+1:]...)
			return
		}
	}
	t.Fatalf("matrix claim %q not found", id)
}
