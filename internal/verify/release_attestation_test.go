package verify

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestValidateReleaseAttestation(t *testing.T) {
	attestation, expected, privateKey, now := releaseAttestationFixture(t)

	if err := ValidateReleaseAttestation(attestation, expected, now); err != nil {
		t.Fatalf("ValidateReleaseAttestation() error = %v", err)
	}

	canonical, err := CanonicalReleaseAttestationPayload(attestation.Payload)
	if err != nil {
		t.Fatalf("CanonicalReleaseAttestationPayload() error = %v", err)
	}
	if !ed25519.Verify(privateKey.Public().(ed25519.PublicKey), canonical, decodeTestBase64(t, attestation.Signature)) {
		t.Fatal("fixture signature does not verify against canonical payload")
	}
}

func TestValidateReleaseAttestationRejectsMismatchedBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReleaseAttestation)
		want   string
	}{
		{
			name: "repository",
			mutate: func(attestation *ReleaseAttestation) {
				attestation.Payload.Repository = "other/picogent"
			},
			want: "repository does not match",
		},
		{
			name: "candidate SHA",
			mutate: func(attestation *ReleaseAttestation) {
				attestation.Payload.CandidateSHA = strings.Repeat("b", 40)
			},
			want: "candidate SHA does not match",
		},
		{
			name: "event",
			mutate: func(attestation *ReleaseAttestation) {
				attestation.Payload.Event = "pull_request"
			},
			want: "event does not match",
		},
		{
			name: "workflow",
			mutate: func(attestation *ReleaseAttestation) {
				attestation.Payload.Workflow = "other.yml"
			},
			want: "workflow does not match",
		},
		{
			name: "run ID",
			mutate: func(attestation *ReleaseAttestation) {
				attestation.Payload.RunID++
			},
			want: "run ID does not match",
		},
		{
			name: "release gates digest",
			mutate: func(attestation *ReleaseAttestation) {
				attestation.Payload.ReleaseGatesSHA256 = strings.Repeat("b", 64)
			},
			want: "release-gates digest does not match",
		},
		{
			name: "verification manifest digest",
			mutate: func(attestation *ReleaseAttestation) {
				attestation.Payload.VerificationManifestSHA256 = strings.Repeat("b", 64)
			},
			want: "verification-manifest digest does not match",
		},
		{
			name: "signer",
			mutate: func(attestation *ReleaseAttestation) {
				attestation.Payload.Signer = "other-signer"
			},
			want: "signer does not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attestation, expected, privateKey, now := releaseAttestationFixture(t)
			tt.mutate(&attestation)
			resignReleaseAttestation(t, &attestation, privateKey)
			if err := ValidateReleaseAttestation(attestation, expected, now); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateReleaseAttestation() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateReleaseAttestationRejectsEnvelopeAndTimeFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReleaseAttestation, time.Time)
		want   string
	}{
		{
			name: "schema",
			mutate: func(attestation *ReleaseAttestation, _ time.Time) {
				attestation.Schema = "picogent.release-attestation.v0"
			},
			want: "unsupported release attestation schema",
		},
		{
			name: "algorithm",
			mutate: func(attestation *ReleaseAttestation, _ time.Time) {
				attestation.Algorithm = "rsa"
			},
			want: "unsupported release attestation algorithm",
		},
		{
			name: "future issued-at",
			mutate: func(attestation *ReleaseAttestation, now time.Time) {
				attestation.Payload.IssuedAt = now.Add(ReleaseAttestationClockSkew + time.Second).Format(time.RFC3339Nano)
			},
			want: "issued in the future",
		},
		{
			name: "expired",
			mutate: func(attestation *ReleaseAttestation, now time.Time) {
				attestation.Payload.IssuedAt = now.Add(-2 * time.Hour).Format(time.RFC3339Nano)
				attestation.Payload.ExpiresAt = now.Add(-time.Second).Format(time.RFC3339Nano)
			},
			want: "expired",
		},
		{
			name: "validity window",
			mutate: func(attestation *ReleaseAttestation, now time.Time) {
				attestation.Payload.ExpiresAt = now.Add(MaxReleaseAttestationLifetime + time.Second).Format(time.RFC3339Nano)
			},
			want: "validity window is too long",
		},
		{
			name: "invalid signature",
			mutate: func(attestation *ReleaseAttestation, _ time.Time) {
				attestation.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
			},
			want: "signature is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attestation, expected, privateKey, now := releaseAttestationFixture(t)
			tt.mutate(&attestation, now)
			if tt.name != "schema" && tt.name != "algorithm" && tt.name != "invalid signature" && tt.name != "future issued-at" && tt.name != "expired" && tt.name != "validity window" {
				resignReleaseAttestation(t, &attestation, privateKey)
			}
			if err := ValidateReleaseAttestation(attestation, expected, now); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateReleaseAttestation() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateReleaseAttestationJSONStatesAndBounds(t *testing.T) {
	attestation, expected, _, now := releaseAttestationFixture(t)
	data, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		data       []byte
		wantStatus ManifestStatus
		wantError  string
	}{
		{name: "absent", data: nil, wantStatus: ManifestUnverified},
		{name: "whitespace absent", data: []byte(" \n\t"), wantStatus: ManifestUnverified},
		{name: "valid", data: data, wantStatus: ManifestPass},
		{name: "malformed", data: []byte("{"), wantStatus: ManifestFail, wantError: "decode release attestation"},
		{name: "trailing JSON", data: append(append([]byte(nil), data...), []byte("\nnull")...), wantStatus: ManifestFail, wantError: "trailing JSON"},
		{name: "oversized", data: bytes.Repeat([]byte(" "), MaxReleaseAttestationBytes+1), wantStatus: ManifestFail, wantError: "exceeds size limit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := ValidateReleaseAttestationJSON(tt.data, expected, now)
			if status != tt.wantStatus {
				t.Fatalf("status = %s, want %s (error = %v)", status, tt.wantStatus, err)
			}
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestDecodeReleaseAttestationRejectsUnknownFields(t *testing.T) {
	attestation, _, _, _ := releaseAttestationFixture(t)
	data, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope["extra"] = json.RawMessage(`true`)
	data, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReleaseAttestation(data); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("DecodeReleaseAttestation() error = %v, want unknown-field rejection", err)
	}
}

func TestValidateReleaseAttestationRejectsMalformedFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReleaseAttestation, *ReleaseAttestationExpectation)
		want   string
	}{
		{
			name: "negative run ID",
			mutate: func(attestation *ReleaseAttestation, _ *ReleaseAttestationExpectation) {
				attestation.Payload.RunID = -1
			},
			want: "run ID must be positive",
		},
		{
			name: "missing signer",
			mutate: func(attestation *ReleaseAttestation, _ *ReleaseAttestationExpectation) {
				attestation.Payload.Signer = ""
			},
			want: "attestation signer is required",
		},
		{
			name: "uppercase digest",
			mutate: func(attestation *ReleaseAttestation, _ *ReleaseAttestationExpectation) {
				attestation.Payload.ReleaseGatesSHA256 = strings.Repeat("A", 64)
			},
			want: "release gates digest is not SHA-256",
		},
		{
			name: "short digest",
			mutate: func(attestation *ReleaseAttestation, _ *ReleaseAttestationExpectation) {
				attestation.Payload.VerificationManifestSHA256 = "abcd"
			},
			want: "verification manifest digest is not SHA-256",
		},
		{
			name: "repository whitespace",
			mutate: func(attestation *ReleaseAttestation, _ *ReleaseAttestationExpectation) {
				attestation.Payload.Repository = " other/picogent"
			},
			want: "attestation repository has surrounding whitespace",
		},
		{
			name: "noncanonical public key",
			mutate: func(attestation *ReleaseAttestation, _ *ReleaseAttestationExpectation) {
				attestation.PublicKey = strings.TrimRight(attestation.PublicKey, "=")
			},
			want: "public key is not canonical base64",
		},
		{
			name: "noncanonical signature",
			mutate: func(attestation *ReleaseAttestation, _ *ReleaseAttestationExpectation) {
				attestation.Signature += "\n"
			},
			want: "signature has invalid length or encoding",
		},
		{
			name: "missing trusted key",
			mutate: func(_ *ReleaseAttestation, expected *ReleaseAttestationExpectation) {
				expected.PublicKey = ""
			},
			want: "expected public key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attestation, expected, _, now := releaseAttestationFixture(t)
			tt.mutate(&attestation, &expected)
			if err := ValidateReleaseAttestation(attestation, expected, now); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateReleaseAttestation() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDecodeReleaseAttestationRejectsNestedUnknownFields(t *testing.T) {
	attestation, _, _, _ := releaseAttestationFixture(t)
	data, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(envelope["payload"], &payload); err != nil {
		t.Fatal(err)
	}
	payload["unexpected"] = json.RawMessage(`true`)
	envelope["payload"], err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReleaseAttestation(data); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("DecodeReleaseAttestation() error = %v, want nested unknown-field rejection", err)
	}
}

func TestValidateReleaseAttestationRejectsUntrustedKey(t *testing.T) {
	attestation, expected, _, now := releaseAttestationFixture(t)
	_, otherPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	attestation.PublicKey = base64.StdEncoding.EncodeToString(otherPrivateKey.Public().(ed25519.PublicKey))
	if err := ValidateReleaseAttestation(attestation, expected, now); err == nil || !strings.Contains(err.Error(), "trusted key") {
		t.Fatalf("ValidateReleaseAttestation() error = %v, want trusted-key rejection", err)
	}
}

func releaseAttestationFixture(t *testing.T) (ReleaseAttestation, ReleaseAttestationExpectation, ed25519.PrivateKey, time.Time) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	payload := ReleaseAttestationPayload{
		Repository:                 "saiaathishkarthik/picogent",
		CandidateSHA:               strings.Repeat("a", 40),
		Event:                      "push",
		Workflow:                   ".github/workflows/ci.yml",
		RunID:                      33577312666,
		ReleaseGatesSHA256:         strings.Repeat("1", 64),
		VerificationManifestSHA256: strings.Repeat("2", 64),
		Signer:                     "github-actions://saiaathishkarthik/picogent/.github/workflows/ci.yml@main",
		IssuedAt:                   now.Add(-time.Minute).Format(time.RFC3339Nano),
		ExpiresAt:                  now.Add(time.Hour).Format(time.RFC3339Nano),
	}
	attestation := ReleaseAttestation{
		Schema:    ReleaseAttestationSchema,
		Algorithm: ReleaseAttestationAlgorithm,
		Payload:   payload,
		PublicKey: base64.StdEncoding.EncodeToString(publicKey),
	}
	resignReleaseAttestation(t, &attestation, privateKey)
	expected := ReleaseAttestationExpectation{
		Repository:                 payload.Repository,
		CandidateSHA:               payload.CandidateSHA,
		Event:                      payload.Event,
		Workflow:                   payload.Workflow,
		RunID:                      payload.RunID,
		ReleaseGatesSHA256:         payload.ReleaseGatesSHA256,
		VerificationManifestSHA256: payload.VerificationManifestSHA256,
		Signer:                     payload.Signer,
		PublicKey:                  attestation.PublicKey,
	}
	return attestation, expected, privateKey, now
}

func resignReleaseAttestation(t *testing.T, attestation *ReleaseAttestation, privateKey ed25519.PrivateKey) {
	t.Helper()
	canonical, err := CanonicalReleaseAttestationPayload(attestation.Payload)
	if err != nil {
		t.Fatal(err)
	}
	attestation.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
}

func decodeTestBase64(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
