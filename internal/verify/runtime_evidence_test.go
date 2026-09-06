package verify

import (
	"bytes"
	"strings"
	"testing"
)

const runtimeEvidenceTestSHA = "2ca217142cf49366feddf881552c29c5044168ab"

func validRuntimeEvidenceRecordForTest() RuntimeEvidenceRecord {
	return RuntimeEvidenceRecord{
		Schema:      RuntimeEvidenceSchema,
		Claim:       "the rendered fixture preserves a blocked task after reload",
		Category:    RuntimeEvidenceRenderedPlatform,
		Verdict:     RuntimeEvidenceInconclusive,
		Repository:  "github.com/saiaathish/picogent",
		SourceSHA:   runtimeEvidenceTestSHA,
		ObservedAt:  "2026-09-06T00:00:00Z",
		Environment: RuntimeEvidenceEnvironment{OS: "darwin", Arch: "arm64", Runtime: "go1.25.0"},
		Setup:       "task-owned rendered fixture with a disposable home and workspace",
		References: []RuntimeEvidenceReference{{
			Kind:   "browser-session",
			Value:  "codex/rendered-evidence/task-127",
			SHA256: strings.Repeat("a", 64),
		}},
		Limitations: []string{
			"deterministic fixture only; no live-provider behavior was exercised",
			"screenshot persistence was not exposed by the browser API",
		},
	}
}

func TestValidateRuntimeEvidenceRecord(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RuntimeEvidenceRecord)
		want   string
	}{
		{name: "valid", mutate: func(*RuntimeEvidenceRecord) {}, want: ""},
		{name: "schema", mutate: func(record *RuntimeEvidenceRecord) { record.Schema = "picogent.runtime-evidence.v0" }, want: "schema"},
		{name: "category", mutate: func(record *RuntimeEvidenceRecord) { record.Category = "provider" }, want: "category"},
		{name: "verdict", mutate: func(record *RuntimeEvidenceRecord) { record.Verdict = "SKIPPED" }, want: "verdict"},
		{name: "source sha", mutate: func(record *RuntimeEvidenceRecord) { record.SourceSHA = "deadbeef" }, want: "source_sha"},
		{name: "timestamp", mutate: func(record *RuntimeEvidenceRecord) { record.ObservedAt = "yesterday" }, want: "observed_at"},
		{name: "reference", mutate: func(record *RuntimeEvidenceRecord) { record.References = nil }, want: "reference"},
		{name: "limitation", mutate: func(record *RuntimeEvidenceRecord) { record.Limitations = nil }, want: "limitation"},
		{name: "digest", mutate: func(record *RuntimeEvidenceRecord) { record.References[0].SHA256 = "not-a-digest" }, want: "sha256"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := validRuntimeEvidenceRecordForTest()
			test.mutate(&record)
			err := ValidateRuntimeEvidenceRecord(record)
			if test.want == "" {
				if err != nil {
					t.Fatalf("ValidateRuntimeEvidenceRecord() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateRuntimeEvidenceRecord() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestRuntimeEvidenceJSONRoundTrip(t *testing.T) {
	original := validRuntimeEvidenceRecordForTest()
	var encoded bytes.Buffer
	if err := WriteRuntimeEvidenceRecord(&encoded, original); err != nil {
		t.Fatalf("WriteRuntimeEvidenceRecord() error = %v", err)
	}
	decoded, err := DecodeRuntimeEvidenceRecord(encoded.Bytes())
	if err != nil {
		t.Fatalf("DecodeRuntimeEvidenceRecord() error = %v", err)
	}
	if decoded.Schema != original.Schema || decoded.SourceSHA != original.SourceSHA || decoded.Verdict != original.Verdict {
		t.Fatalf("decoded record = %#v, want key fields from %#v", decoded, original)
	}
}

func TestDecodeRuntimeEvidenceRecordRejectsUnknownAndTrailingJSON(t *testing.T) {
	valid := `{"schema":"picogent.runtime-evidence.v1","claim":"claim","category":"hostile_runtime","verdict":"UNVERIFIED","repository":"github.com/saiaathish/picogent","source_sha":"2ca217142cf49366feddf881552c29c5044168ab","observed_at":"2026-09-06T00:00:00Z","environment":{"os":"darwin","arch":"arm64","runtime":"go1.25.0"},"setup":"fixture","references":[{"kind":"log","value":"UNRECORDED"}],"limitations":["not observed"]}`
	if _, err := DecodeRuntimeEvidenceRecord([]byte(strings.Replace(valid, `"limitations"`, `"extra":true,"limitations"`, 1))); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	if _, err := DecodeRuntimeEvidenceRecord([]byte(valid + "\n{}")); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("trailing JSON error = %v", err)
	}
}

func TestWriteRuntimeEvidenceRecordRejectsNilWriter(t *testing.T) {
	if err := WriteRuntimeEvidenceRecord(nil, validRuntimeEvidenceRecordForTest()); err == nil {
		t.Fatal("WriteRuntimeEvidenceRecord(nil, ...) unexpectedly succeeded")
	}
}
