package verify

import (
	"strings"
	"testing"
)

func TestValidateReleaseGateLedger(t *testing.T) {
	const candidateSHA = "0123456789abcdef0123456789abcdef01234567"
	required := []string{"test", "security"}

	tests := []struct {
		name    string
		mutate  func(*ReleaseGateLedger)
		wantErr string
	}{
		{name: "pass"},
		{
			name: "missing required gate",
			mutate: func(ledger *ReleaseGateLedger) {
				ledger.Gates = ledger.Gates[:1]
			},
			wantErr: "missing required gate security",
		},
		{
			name: "failed gate",
			mutate: func(ledger *ReleaseGateLedger) {
				ledger.Gates[0].Status = "FAIL"
			},
			wantErr: "gate test is FAIL",
		},
		{
			name: "mismatched sha",
			mutate: func(ledger *ReleaseGateLedger) {
				ledger.Gates[0].CandidateSHA = strings.Repeat("a", 40)
			},
			wantErr: "gate test candidate SHA does not match",
		},
		{
			name: "nonzero exit",
			mutate: func(ledger *ReleaseGateLedger) {
				ledger.Gates[0].ExitCode = 1
			},
			wantErr: "gate test exit code is 1",
		},
		{
			name: "duplicate gate",
			mutate: func(ledger *ReleaseGateLedger) {
				ledger.Gates = append(ledger.Gates, ledger.Gates[0])
			},
			wantErr: "duplicate gate test",
		},
		{
			name: "truncated evidence",
			mutate: func(ledger *ReleaseGateLedger) {
				ledger.Gates[1].Truncated = true
			},
			wantErr: "gate security evidence is truncated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ledger := releaseGateFixture(candidateSHA)
			if tt.mutate != nil {
				tt.mutate(&ledger)
			}
			err := ValidateReleaseGateLedger(ledger, candidateSHA, "pull_request", required)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateReleaseGateLedger() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateReleaseGateLedger() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func releaseGateFixture(candidateSHA string) ReleaseGateLedger {
	return ReleaseGateLedger{
		Schema:       ReleaseGateSchema,
		CandidateSHA: candidateSHA,
		Event:        "pull_request",
		Gates: []ReleaseGateRecord{
			{
				CandidateSHA: candidateSHA,
				Event:        "pull_request",
				Job:          "test",
				OS:           "matrix",
				Command:      "go test ./...",
				Status:       "PASS",
				ExitCode:     0,
			},
			{
				CandidateSHA: candidateSHA,
				Event:        "pull_request",
				Job:          "security",
				OS:           "ubuntu-latest",
				Command:      "govulncheck ./...",
				Status:       "PASS",
				ExitCode:     0,
			},
		},
	}
}
