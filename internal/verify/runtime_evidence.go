package verify

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	RuntimeEvidenceSchema        = "picogent.runtime-evidence.v1"
	MaxRuntimeEvidenceBytes      = 32 << 10
	maxRuntimeEvidenceText       = 512
	maxRuntimeEvidenceReferences = 16
	maxRuntimeEvidenceLimits     = 16
)

// RuntimeEvidenceVerdict is intentionally narrower than ManifestStatus. A
// runtime observation is either directly supported, directly contradicted,
// not decisive, or not observed; SKIPPED is not a claim about runtime state.
type RuntimeEvidenceVerdict string

const (
	RuntimeEvidencePass         RuntimeEvidenceVerdict = "PASS"
	RuntimeEvidenceFail         RuntimeEvidenceVerdict = "FAIL"
	RuntimeEvidenceInconclusive RuntimeEvidenceVerdict = "INCONCLUSIVE"
	RuntimeEvidenceUnverified   RuntimeEvidenceVerdict = "UNVERIFIED"
)

type RuntimeEvidenceCategory string

const (
	RuntimeEvidenceLiveProvider         RuntimeEvidenceCategory = "live_provider"
	RuntimeEvidenceRenderedPlatform     RuntimeEvidenceCategory = "rendered_platform"
	RuntimeEvidenceHostileRuntime       RuntimeEvidenceCategory = "hostile_runtime"
	RuntimeEvidenceRecoveryUndo         RuntimeEvidenceCategory = "recovery_undo"
	RuntimeEvidenceReleaseAuthorization RuntimeEvidenceCategory = "release_authorization"
)

type RuntimeEvidenceEnvironment struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Runtime string `json:"runtime"`
	Host    string `json:"host,omitempty"`
}

// RuntimeEvidenceReference identifies the retained proof or observation used
// by a record. Value may be a path, URL, task-owned session identifier, or an
// explicit UNRECORDED marker; the limitations field must explain unsupported
// or unavailable capture.
type RuntimeEvidenceReference struct {
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	SHA256 string `json:"sha256,omitempty"`
}

// RuntimeEvidenceRecord is the smallest shared unit for the v4 runtime
// evidence matrix. It is an evidence boundary, not a release authorization.
type RuntimeEvidenceRecord struct {
	Schema      string                     `json:"schema"`
	Claim       string                     `json:"claim"`
	Category    RuntimeEvidenceCategory    `json:"category"`
	Verdict     RuntimeEvidenceVerdict     `json:"verdict"`
	Repository  string                     `json:"repository"`
	SourceSHA   string                     `json:"source_sha"`
	ObservedAt  string                     `json:"observed_at"`
	Environment RuntimeEvidenceEnvironment `json:"environment"`
	Setup       string                     `json:"setup"`
	References  []RuntimeEvidenceReference `json:"references"`
	Limitations []string                   `json:"limitations"`
}

// ValidateRuntimeEvidenceRecord rejects incomplete or lookalike evidence.
// Callers must retain the returned error instead of converting it to PASS.
func ValidateRuntimeEvidenceRecord(record RuntimeEvidenceRecord) error {
	if record.Schema != RuntimeEvidenceSchema {
		return fmt.Errorf("runtime evidence schema must be %q", RuntimeEvidenceSchema)
	}
	if err := requireRuntimeEvidenceText("claim", record.Claim); err != nil {
		return err
	}
	if !validRuntimeEvidenceCategory(record.Category) {
		return fmt.Errorf("runtime evidence category %q is unsupported", record.Category)
	}
	if !validRuntimeEvidenceVerdict(record.Verdict) {
		return fmt.Errorf("runtime evidence verdict %q is unsupported", record.Verdict)
	}
	if err := requireRuntimeEvidenceText("repository", record.Repository); err != nil {
		return err
	}
	if !validManifestCommitID(strings.TrimSpace(record.SourceSHA)) {
		return errors.New("runtime evidence source_sha must be a full commit ID")
	}
	if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(record.ObservedAt)); err != nil {
		return fmt.Errorf("runtime evidence observed_at must be RFC3339: %w", err)
	}
	if err := requireRuntimeEvidenceText("environment.os", record.Environment.OS); err != nil {
		return err
	}
	if err := requireRuntimeEvidenceText("environment.arch", record.Environment.Arch); err != nil {
		return err
	}
	if err := requireRuntimeEvidenceText("environment.runtime", record.Environment.Runtime); err != nil {
		return err
	}
	if err := requireRuntimeEvidenceText("setup", record.Setup); err != nil {
		return err
	}
	if len(record.References) == 0 {
		return errors.New("runtime evidence requires at least one reference")
	}
	if len(record.References) > maxRuntimeEvidenceReferences {
		return fmt.Errorf("runtime evidence has too many references: %d", len(record.References))
	}
	for index, reference := range record.References {
		if err := requireRuntimeEvidenceText(fmt.Sprintf("references[%d].kind", index), reference.Kind); err != nil {
			return err
		}
		if err := requireRuntimeEvidenceText(fmt.Sprintf("references[%d].value", index), reference.Value); err != nil {
			return err
		}
		if reference.SHA256 != "" && !validSHA256(reference.SHA256) {
			return fmt.Errorf("references[%d].sha256 must be 64 hexadecimal characters", index)
		}
	}
	if len(record.Limitations) == 0 {
		return errors.New("runtime evidence requires at least one limitation")
	}
	if len(record.Limitations) > maxRuntimeEvidenceLimits {
		return fmt.Errorf("runtime evidence has too many limitations: %d", len(record.Limitations))
	}
	for index, limitation := range record.Limitations {
		if err := requireRuntimeEvidenceText(fmt.Sprintf("limitations[%d]", index), limitation); err != nil {
			return err
		}
	}
	return nil
}

// DecodeRuntimeEvidenceRecord parses exactly one bounded, strict record and
// validates it before returning it to a caller.
func DecodeRuntimeEvidenceRecord(data []byte) (RuntimeEvidenceRecord, error) {
	if len(data) == 0 {
		return RuntimeEvidenceRecord{}, errors.New("runtime evidence is empty")
	}
	if len(data) > MaxRuntimeEvidenceBytes {
		return RuntimeEvidenceRecord{}, fmt.Errorf("runtime evidence exceeds %d bytes", MaxRuntimeEvidenceBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record RuntimeEvidenceRecord
	if err := decoder.Decode(&record); err != nil {
		return RuntimeEvidenceRecord{}, fmt.Errorf("decode runtime evidence: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return RuntimeEvidenceRecord{}, errors.New("runtime evidence contains multiple JSON values")
		}
		return RuntimeEvidenceRecord{}, fmt.Errorf("decode runtime evidence trailer: %w", err)
	}
	if err := ValidateRuntimeEvidenceRecord(record); err != nil {
		return RuntimeEvidenceRecord{}, err
	}
	return record, nil
}

// WriteRuntimeEvidenceRecord emits a validated, bounded JSON record.
func WriteRuntimeEvidenceRecord(w io.Writer, record RuntimeEvidenceRecord) error {
	if w == nil {
		return errors.New("runtime evidence writer is nil")
	}
	if err := ValidateRuntimeEvidenceRecord(record); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal runtime evidence: %w", err)
	}
	if len(data)+1 > MaxRuntimeEvidenceBytes {
		return fmt.Errorf("runtime evidence exceeds %d bytes", MaxRuntimeEvidenceBytes)
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func validRuntimeEvidenceCategory(category RuntimeEvidenceCategory) bool {
	switch category {
	case RuntimeEvidenceLiveProvider, RuntimeEvidenceRenderedPlatform,
		RuntimeEvidenceHostileRuntime, RuntimeEvidenceRecoveryUndo,
		RuntimeEvidenceReleaseAuthorization:
		return true
	default:
		return false
	}
}

func validRuntimeEvidenceVerdict(verdict RuntimeEvidenceVerdict) bool {
	switch verdict {
	case RuntimeEvidencePass, RuntimeEvidenceFail, RuntimeEvidenceInconclusive, RuntimeEvidenceUnverified:
		return true
	default:
		return false
	}
}

func requireRuntimeEvidenceText(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("runtime evidence %s is required", field)
	}
	if len(value) > maxRuntimeEvidenceText {
		return fmt.Errorf("runtime evidence %s exceeds %d bytes", field, maxRuntimeEvidenceText)
	}
	if strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("runtime evidence %s contains NUL", field)
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
