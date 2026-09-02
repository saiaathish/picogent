package verify

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateReleaseEvidenceDirectory(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(filepath.Dir(workspace), "picogent-release-evidence")

	tests := []struct {
		name       string
		workspace  string
		evidence   string
		wantSubstr string
	}{
		{name: "runner temporary sibling", workspace: workspace, evidence: outside},
		{name: "sibling with shared prefix", workspace: workspace, evidence: workspace + "-evidence"},
		{name: "same directory", workspace: workspace, evidence: workspace, wantSubstr: "inside workspace"},
		{name: "nested directory", workspace: workspace, evidence: filepath.Join(workspace, "artifacts"), wantSubstr: "inside workspace"},
		{name: "relative evidence", workspace: workspace, evidence: "artifacts", wantSubstr: "must be absolute"},
		{name: "relative workspace", workspace: "workspace", evidence: outside, wantSubstr: "workspace must be absolute"},
		{name: "missing evidence", workspace: workspace, wantSubstr: "directory is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateReleaseEvidenceDirectory(tt.workspace, tt.evidence)
			if tt.wantSubstr == "" {
				if err != nil {
					t.Fatalf("ValidateReleaseEvidenceDirectory() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("ValidateReleaseEvidenceDirectory() error = %v, want substring %q", err, tt.wantSubstr)
			}
		})
	}
}
