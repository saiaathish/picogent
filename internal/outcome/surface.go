package outcome

import "fmt"

// BoundTurnContract returns the canonical, bounded surface projection. It
// preserves a runtime-derived contradiction witness while downgrading
// caller-created or reloaded reports to advisory evidence.
func BoundTurnContract(contract TurnContract) TurnContract {
	return boundTurnContract(contract)
}

// SurfaceSummary returns one safe, categorical status for a user-facing
// surface. It deliberately omits origins, summaries, commands, and other
// evidence text; taskstate completion remains the only retirement authority.
func SurfaceSummary(contract TurnContract) string {
	return SurfaceContradictionSummary(contract.Contradictions)
}

// SurfaceContradictionSummary formats the bounded contradiction state for a
// user-facing surface. A caller-shaped CONFIRMED report is downgraded before
// formatting, so reloaded or untrusted data cannot become an instruction.
func SurfaceContradictionSummary(report ContradictionReport) string {
	report = boundContradictionReport(report)
	if report.State == ContradictionNone || len(report.Signals) == 0 {
		return ""
	}

	label := "signal"
	if len(report.Signals) != 1 {
		label = "signals"
	}
	truncated := ""
	if report.Truncated {
		truncated = "; some signals omitted"
	}
	switch report.State {
	case ContradictionConfirmed:
		return fmt.Sprintf("Contradictory evidence confirmed (%d %s%s); diagnose and recheck before continuing", len(report.Signals), label, truncated)
	case ContradictionAdvisory:
		return fmt.Sprintf("Contradictory evidence is unverified (%d %s%s); it cannot select an action", len(report.Signals), label, truncated)
	default:
		return ""
	}
}
