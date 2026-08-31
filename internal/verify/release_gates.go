package verify

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ReleaseGateSchema   = "picogent.release-gates.v1"
	MaxReleaseGateBytes = 16 << 10
	maxReleaseGateJobs  = 32
	maxReleaseGateText  = 512
)

// ReleaseGateRecord is the bounded result of one required CI job. It carries
// provenance separately from the verification manifest so a release decision
// cannot silently treat an uploaded artifact as proof that its producer ran.
type ReleaseGateRecord struct {
	CandidateSHA string `json:"candidate_sha"`
	Event        string `json:"event"`
	Job          string `json:"job"`
	OS           string `json:"os"`
	Command      string `json:"command"`
	Status       string `json:"status"`
	ExitCode     int    `json:"exit_code"`
	Truncated    bool   `json:"truncated,omitempty"`
}

// ReleaseGateLedger binds required job results to one candidate commit and
// event. The ledger is intentionally independent of task completion state.
type ReleaseGateLedger struct {
	Schema       string              `json:"schema"`
	CandidateSHA string              `json:"candidate_sha"`
	Event        string              `json:"event"`
	Gates        []ReleaseGateRecord `json:"gates"`
}

// ValidateReleaseGateLedger fails closed unless every required job has exactly
// one complete PASS record for the expected candidate and event. Optional
// records are also checked, so malformed extra evidence cannot be ignored.
func ValidateReleaseGateLedger(ledger ReleaseGateLedger, expectedSHA, expectedEvent string, requiredJobs []string) error {
	if ledger.Schema != ReleaseGateSchema {
		return fmt.Errorf("unsupported release gate schema %q", ledger.Schema)
	}
	candidate := strings.TrimSpace(ledger.CandidateSHA)
	if !validManifestCommitID(candidate) {
		return errors.New("release gate candidate SHA is not a full commit ID")
	}
	expected := strings.TrimSpace(expectedSHA)
	if !validManifestCommitID(expected) {
		return errors.New("expected release gate SHA is not a full commit ID")
	}
	if !strings.EqualFold(candidate, expected) {
		return errors.New("release gate candidate SHA does not match expected SHA")
	}
	event := strings.TrimSpace(expectedEvent)
	if event == "" {
		return errors.New("expected release gate event is required")
	}
	if strings.TrimSpace(ledger.Event) != event {
		return errors.New("release gate event does not match expected event")
	}
	if len(ledger.Gates) == 0 {
		return errors.New("release gate ledger has no gates")
	}
	if len(ledger.Gates) > maxReleaseGateJobs {
		return fmt.Errorf("release gate ledger has too many gates: %d", len(ledger.Gates))
	}

	required := make(map[string]struct{}, len(requiredJobs))
	for _, raw := range requiredJobs {
		job := strings.TrimSpace(raw)
		if job == "" {
			return errors.New("required release gate job is empty")
		}
		if len(job) > maxReleaseGateText {
			return errors.New("required release gate job is too long")
		}
		if _, exists := required[job]; exists {
			return fmt.Errorf("duplicate required release gate %s", job)
		}
		required[job] = struct{}{}
	}
	if len(required) == 0 {
		return errors.New("no required release gate jobs were configured")
	}

	seen := make(map[string]struct{}, len(ledger.Gates))
	for _, gate := range ledger.Gates {
		job := strings.TrimSpace(gate.Job)
		if job == "" {
			return errors.New("release gate job is empty")
		}
		if len(job) > maxReleaseGateText {
			return fmt.Errorf("release gate %s job is too long", job)
		}
		if _, exists := seen[job]; exists {
			return fmt.Errorf("duplicate gate %s", job)
		}
		seen[job] = struct{}{}
		if !strings.EqualFold(strings.TrimSpace(gate.CandidateSHA), candidate) {
			return fmt.Errorf("gate %s candidate SHA does not match ledger", job)
		}
		if strings.TrimSpace(gate.Event) != event {
			return fmt.Errorf("gate %s event does not match ledger", job)
		}
		if strings.TrimSpace(gate.OS) == "" || len(strings.TrimSpace(gate.OS)) > maxReleaseGateText {
			return fmt.Errorf("gate %s OS is empty or too long", job)
		}
		if strings.TrimSpace(gate.Command) == "" || len(strings.TrimSpace(gate.Command)) > maxReleaseGateText {
			return fmt.Errorf("gate %s command is empty or too long", job)
		}
		status := strings.ToUpper(strings.TrimSpace(gate.Status))
		if status != "PASS" {
			return fmt.Errorf("gate %s is %s", job, statusOrUnknown(status))
		}
		if gate.ExitCode != 0 {
			return fmt.Errorf("gate %s exit code is %d", job, gate.ExitCode)
		}
		if gate.Truncated {
			return fmt.Errorf("gate %s evidence is truncated", job)
		}
	}
	for job := range required {
		if _, exists := seen[job]; !exists {
			return fmt.Errorf("missing required gate %s", job)
		}
	}
	return nil
}

func statusOrUnknown(status string) string {
	if status == "" {
		return "UNKNOWN"
	}
	return status
}
