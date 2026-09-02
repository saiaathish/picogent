package verify

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
)

const (
	ReleaseAttestationSchema      = "picogent.release-attestation.v1"
	ReleaseAttestationAlgorithm   = "ed25519"
	MaxReleaseAttestationBytes    = 16 << 10
	MaxReleaseAttestationText     = 512
	MaxReleaseAttestationLifetime = 7 * 24 * time.Hour
	ReleaseAttestationClockSkew   = 5 * time.Minute
)

// ReleaseAttestationPayload is the signed, canonical portion of a release
// evidence attestation. It is intentionally separate from task completion and
// does not authorize a release by itself.
type ReleaseAttestationPayload struct {
	Repository                 string `json:"repository"`
	CandidateSHA               string `json:"candidate_sha"`
	Event                      string `json:"event"`
	Workflow                   string `json:"workflow"`
	RunID                      int64  `json:"run_id"`
	ReleaseGatesSHA256         string `json:"release_gates_sha256"`
	VerificationManifestSHA256 string `json:"verification_manifest_sha256"`
	Signer                     string `json:"signer"`
	IssuedAt                   string `json:"issued_at"`
	ExpiresAt                  string `json:"expires_at"`
}

// ReleaseAttestation is a detached signature envelope for the two bounded
// release-evidence artifacts produced by CI. PublicKey is checked against a
// caller-provided trust expectation; a self-signed envelope is not accepted.
type ReleaseAttestation struct {
	Schema    string                    `json:"schema"`
	Algorithm string                    `json:"algorithm"`
	Payload   ReleaseAttestationPayload `json:"payload"`
	PublicKey string                    `json:"public_key"`
	Signature string                    `json:"signature"`
}

// ReleaseAttestationExpectation identifies the exact evidence a consumer
// expects. All fields are required so an omitted expectation cannot turn an
// arbitrary valid signature into a passing release result.
type ReleaseAttestationExpectation struct {
	Repository                 string
	CandidateSHA               string
	Event                      string
	Workflow                   string
	RunID                      int64
	ReleaseGatesSHA256         string
	VerificationManifestSHA256 string
	Signer                     string
	PublicKey                  string
}

// CanonicalReleaseAttestationPayload returns the exact bytes covered by the
// Ed25519 signature. The domain prefix prevents a signature from being reused
// as evidence for a different protocol.
func CanonicalReleaseAttestationPayload(payload ReleaseAttestationPayload) ([]byte, error) {
	if _, _, err := validateReleaseAttestationPayload(payload); err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal release attestation payload: %w", err)
	}
	return append([]byte(ReleaseAttestationSchema+"\n"), body...), nil
}

// DecodeReleaseAttestation decodes one bounded JSON envelope and rejects
// unknown fields or trailing JSON values. Empty input is handled by
// ValidateReleaseAttestationJSON so absence can remain explicitly
// UNVERIFIED.
func DecodeReleaseAttestation(data []byte) (ReleaseAttestation, error) {
	if len(data) > MaxReleaseAttestationBytes {
		return ReleaseAttestation{}, errors.New("release attestation exceeds size limit")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return ReleaseAttestation{}, errors.New("release attestation is empty")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var attestation ReleaseAttestation
	if err := decoder.Decode(&attestation); err != nil {
		return ReleaseAttestation{}, fmt.Errorf("decode release attestation: %w", err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ReleaseAttestation{}, errors.New("release attestation has trailing JSON")
		}
		return ReleaseAttestation{}, fmt.Errorf("decode trailing release attestation data: %w", err)
	}
	return attestation, nil
}

// ValidateReleaseAttestation checks the envelope, exact expected identity,
// validity window, trusted public key, and signature. It is a release-evidence
// check only; it never changes task or goal completion state.
func ValidateReleaseAttestation(attestation ReleaseAttestation, expected ReleaseAttestationExpectation, now time.Time) error {
	if attestation.Schema != ReleaseAttestationSchema {
		return fmt.Errorf("unsupported release attestation schema %q", attestation.Schema)
	}
	if attestation.Algorithm != ReleaseAttestationAlgorithm {
		return fmt.Errorf("unsupported release attestation algorithm %q", attestation.Algorithm)
	}
	if err := validateReleaseAttestationExpectation(expected); err != nil {
		return err
	}
	issuedAt, expiresAt, err := validateReleaseAttestationPayload(attestation.Payload)
	if err != nil {
		return err
	}
	if err := compareReleaseAttestationPayload(attestation.Payload, expected); err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	if issuedAt.After(now.Add(ReleaseAttestationClockSkew)) {
		return errors.New("release attestation is issued in the future")
	}
	if !expiresAt.After(now) {
		return errors.New("release attestation is expired")
	}

	publicKey, err := decodeAttestationBase64(attestation.PublicKey, ed25519.PublicKeySize, "public key")
	if err != nil {
		return err
	}
	if attestation.PublicKey != expected.PublicKey {
		return errors.New("release attestation public key does not match trusted key")
	}
	signature, err := decodeAttestationBase64(attestation.Signature, ed25519.SignatureSize, "signature")
	if err != nil {
		return err
	}
	signedPayload, err := CanonicalReleaseAttestationPayload(attestation.Payload)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), signedPayload, signature) {
		return errors.New("release attestation signature is invalid")
	}
	return nil
}

// ValidateReleaseAttestationJSON maps the absence of an attestation to the
// explicit release-evidence state UNVERIFIED. Present malformed or mismatched
// evidence is FAIL; only a complete trusted signature is PASS.
func ValidateReleaseAttestationJSON(data []byte, expected ReleaseAttestationExpectation, now time.Time) (ManifestStatus, error) {
	if len(data) > MaxReleaseAttestationBytes {
		return ManifestFail, errors.New("release attestation exceeds size limit")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return ManifestUnverified, nil
	}
	attestation, err := DecodeReleaseAttestation(data)
	if err != nil {
		return ManifestFail, err
	}
	if err := ValidateReleaseAttestation(attestation, expected, now); err != nil {
		return ManifestFail, err
	}
	return ManifestPass, nil
}

func validateReleaseAttestationExpectation(expected ReleaseAttestationExpectation) error {
	if err := validateReleaseAttestationIdentity(expected.Repository, "expected repository"); err != nil {
		return err
	}
	if !validManifestCommitID(expected.CandidateSHA) {
		return errors.New("expected attestation candidate SHA is not a full commit ID")
	}
	if err := validateReleaseAttestationIdentity(expected.Event, "expected event"); err != nil {
		return err
	}
	if err := validateReleaseAttestationIdentity(expected.Workflow, "expected workflow"); err != nil {
		return err
	}
	if expected.RunID <= 0 {
		return errors.New("expected attestation run ID must be positive")
	}
	if !validSHA256Digest(expected.ReleaseGatesSHA256) {
		return errors.New("expected release gates digest is not SHA-256")
	}
	if !validSHA256Digest(expected.VerificationManifestSHA256) {
		return errors.New("expected verification manifest digest is not SHA-256")
	}
	if err := validateReleaseAttestationIdentity(expected.Signer, "expected signer"); err != nil {
		return err
	}
	if _, err := decodeAttestationBase64(expected.PublicKey, ed25519.PublicKeySize, "expected public key"); err != nil {
		return err
	}
	return nil
}

func validateReleaseAttestationPayload(payload ReleaseAttestationPayload) (time.Time, time.Time, error) {
	if err := validateReleaseAttestationIdentity(payload.Repository, "attestation repository"); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !validManifestCommitID(payload.CandidateSHA) {
		return time.Time{}, time.Time{}, errors.New("attestation candidate SHA is not a full commit ID")
	}
	if err := validateReleaseAttestationIdentity(payload.Event, "attestation event"); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if err := validateReleaseAttestationIdentity(payload.Workflow, "attestation workflow"); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if payload.RunID <= 0 {
		return time.Time{}, time.Time{}, errors.New("attestation run ID must be positive")
	}
	if !validSHA256Digest(payload.ReleaseGatesSHA256) {
		return time.Time{}, time.Time{}, errors.New("release gates digest is not SHA-256")
	}
	if !validSHA256Digest(payload.VerificationManifestSHA256) {
		return time.Time{}, time.Time{}, errors.New("verification manifest digest is not SHA-256")
	}
	if err := validateReleaseAttestationIdentity(payload.Signer, "attestation signer"); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if err := validateReleaseAttestationIdentity(payload.IssuedAt, "attestation issued-at timestamp"); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if err := validateReleaseAttestationIdentity(payload.ExpiresAt, "attestation expires-at timestamp"); err != nil {
		return time.Time{}, time.Time{}, err
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, payload.IssuedAt)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("attestation issued-at timestamp is invalid: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, payload.ExpiresAt)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("attestation expires-at timestamp is invalid: %w", err)
	}
	if !expiresAt.After(issuedAt) {
		return time.Time{}, time.Time{}, errors.New("attestation expires-at timestamp must be after issued-at")
	}
	if expiresAt.Sub(issuedAt) > MaxReleaseAttestationLifetime {
		return time.Time{}, time.Time{}, errors.New("attestation validity window is too long")
	}
	return issuedAt, expiresAt, nil
}

func compareReleaseAttestationPayload(payload ReleaseAttestationPayload, expected ReleaseAttestationExpectation) error {
	if payload.Repository != expected.Repository {
		return errors.New("release attestation repository does not match expected repository")
	}
	if !strings.EqualFold(payload.CandidateSHA, expected.CandidateSHA) {
		return errors.New("release attestation candidate SHA does not match expected SHA")
	}
	if payload.Event != expected.Event {
		return errors.New("release attestation event does not match expected event")
	}
	if payload.Workflow != expected.Workflow {
		return errors.New("release attestation workflow does not match expected workflow")
	}
	if payload.RunID != expected.RunID {
		return errors.New("release attestation run ID does not match expected run ID")
	}
	if payload.ReleaseGatesSHA256 != expected.ReleaseGatesSHA256 {
		return errors.New("release attestation release-gates digest does not match expected digest")
	}
	if payload.VerificationManifestSHA256 != expected.VerificationManifestSHA256 {
		return errors.New("release attestation verification-manifest digest does not match expected digest")
	}
	if payload.Signer != expected.Signer {
		return errors.New("release attestation signer does not match expected signer")
	}
	return nil
}

func validateReleaseAttestationIdentity(value, field string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s has surrounding whitespace", field)
	}
	if len(value) > MaxReleaseAttestationText {
		return fmt.Errorf("%s is too long", field)
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s contains a control character", field)
	}
	return nil
}

func validSHA256Digest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func decodeAttestationBase64(value string, expectedLength int, label string) ([]byte, error) {
	if value == "" {
		return nil, fmt.Errorf("release attestation %s is required", label)
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("release attestation %s is not canonical base64: %w", label, err)
	}
	if len(decoded) != expectedLength || base64.StdEncoding.EncodeToString(decoded) != value {
		return nil, fmt.Errorf("release attestation %s has invalid length or encoding", label)
	}
	return decoded, nil
}
