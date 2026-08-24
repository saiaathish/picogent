package verify

import "strings"

// StatusFromEvidence parses the status token emitted by Format or a tool.
// Unknown, missing, and lookalike tokens are inconclusive; only a complete
// "verify PASS" token can authorize completion or learned reflection.
func StatusFromEvidence(evidence string) Status {
	rest := strings.TrimSpace(strings.ToUpper(evidence))
	if !strings.HasPrefix(rest, "VERIFY ") {
		return StatusInconclusive
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, "VERIFY "))
	end := 0
	for end < len(rest) && rest[end] >= 'A' && rest[end] <= 'Z' {
		end++
	}
	if end == 0 {
		return StatusInconclusive
	}
	switch rest[:end] {
	case string(StatusPass):
		return StatusPass
	case string(StatusFail):
		return StatusFail
	case string(StatusInconclusive):
		return StatusInconclusive
	case string(StatusSkipped):
		return StatusSkipped
	default:
		return StatusInconclusive
	}
}
