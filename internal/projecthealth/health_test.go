package projecthealth

import (
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/repomap"
)

func TestFromSnapshotRanksUnverifiedWorkBeforeInformationalDirtyState(t *testing.T) {
	report := FromSnapshot(repomap.Snapshot{
		Summary: repomap.Map{
			Languages:      []string{"Go"},
			Manifests:      []string{"go.mod", "tools.go"},
			Commands:       repomap.Commands{Build: []string{"go build ./..."}, Test: []string{"go test ./..."}, Lint: []string{"go vet ./..."}},
			InventoryFiles: 12,
		},
		Head:       strings.Repeat("a", 40),
		HeadKnown:  true,
		DirtyKnown: true,
		DirtyPaths: []string{"README.md"},
	})

	if report.Status != StateAttention {
		t.Fatalf("status = %q, want ATTENTION", report.Status)
	}
	if len(report.Findings) < 4 {
		t.Fatalf("findings = %#v, want build/test/lint/dirty observations", report.Findings)
	}
	for i := 1; i < len(report.Findings); i++ {
		if report.Findings[i-1].Priority < report.Findings[i].Priority {
			t.Fatalf("findings not ranked: %#v", report.Findings)
		}
	}
	if report.Findings[len(report.Findings)-1].ID != "uncommitted-work" {
		t.Fatalf("lowest-priority finding = %q, want uncommitted-work", report.Findings[len(report.Findings)-1].ID)
	}
	if report.Dimensions[0].State != StateUnverified || report.Dimensions[1].State != StateUnverified {
		t.Fatalf("runtime checks were presented as proven: %#v", report.Dimensions[:2])
	}
}

func TestFromSnapshotPreservesUnknownProvenanceAndProjectShape(t *testing.T) {
	report := FromSnapshot(repomap.Snapshot{Summary: repomap.Map{InventoryFiles: 1}})

	if report.Provenance.HeadKnown || report.Provenance.DirtyKnown {
		t.Fatalf("unknown provenance became known: %#v", report.Provenance)
	}
	if report.Dimensions[7].Name != "environment" || report.Dimensions[7].State != StateObserved {
		t.Fatalf("environment dimension = %#v", report.Dimensions[7])
	}
	if report.Findings[0].ID != "project-shape-unknown" {
		t.Fatalf("first finding = %#v", report.Findings[0])
	}
	if report.Findings[0].Priority < report.Findings[len(report.Findings)-1].Priority {
		t.Fatalf("findings are not deterministic: %#v", report.Findings)
	}
	for _, dimension := range report.Dimensions {
		if dimension.State == StateObserved && strings.Contains(strings.ToLower(dimension.Summary), "healthy") {
			t.Fatalf("static observation claimed health: %#v", dimension)
		}
	}
}

func TestFormatIsBoundedAndDoesNotIncludeRawCommandOutput(t *testing.T) {
	report := Report{
		Schema: Schema,
		Status: StateAttention,
		Shape:  Shape{Commands: repomap.Commands{Test: []string{"test --secret=do-not-copy"}}},
		Findings: []Finding{{
			ID:         "x",
			Dimension:  "tests",
			Priority:   1,
			Severity:   SeverityLow,
			Confidence: "high",
			Title:      strings.Repeat("title ", 200),
			Evidence:   strings.Repeat("evidence ", 200),
			NextAction: strings.Repeat("action ", 200),
		}},
	}
	formatted := Format(report)
	if len(formatted) > MaxOutputBytes {
		t.Fatalf("formatted report is %d bytes", len(formatted))
	}
	if strings.Contains(formatted, "do-not-copy") {
		t.Fatal("raw command detail leaked into diagnosis output")
	}
	if !strings.Contains(formatted, `"schema": "picogent.project-health.v1"`) {
		t.Fatalf("schema missing from %s", formatted)
	}
}

func TestFromSnapshotDoesNotMutateSnapshotSlices(t *testing.T) {
	languages := []string{"Go"}
	dirty := []string{"main.go"}
	snapshot := repomap.Snapshot{
		Summary:    repomap.Map{Languages: languages, Manifests: []string{"go.mod"}},
		DirtyKnown: true,
		HeadKnown:  true,
		DirtyPaths: dirty,
		Head:       strings.Repeat("b", 40),
	}
	_ = FromSnapshot(snapshot)
	if len(languages) != 1 || languages[0] != "Go" || len(dirty) != 1 || dirty[0] != "main.go" {
		t.Fatalf("snapshot slices changed: languages=%#v dirty=%#v", languages, dirty)
	}
}
