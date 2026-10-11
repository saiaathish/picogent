// Package inventionlab contains bounded, deterministic v4 invention
// prototypes. Nothing in this package is imported by Picogent runtime code.
package inventionlab

import (
	"path"
	"sort"
	"strings"
)

type metricSample struct {
	name  string
	value int64
}

type parityAudit struct {
	comparable bool
	findings   []string
}

// auditMetricParity treats a present zero as measured and missing or duplicate
// observations as incomparable. It deliberately does not compare quality.
func auditMetricParity(required []string, baseline, candidate []metricSample) parityAudit {
	var findings []string
	for _, variant := range []struct {
		name string
		rows []metricSample
	}{
		{name: "baseline", rows: baseline},
		{name: "candidate", rows: candidate},
	} {
		counts := make(map[string]int, len(variant.rows))
		for _, row := range variant.rows {
			counts[row.name]++
		}
		wanted := make(map[string]struct{}, len(required))
		for _, name := range required {
			wanted[name] = struct{}{}
			switch counts[name] {
			case 0:
				findings = append(findings, "missing "+variant.name+"."+name)
			case 1:
				// Zero is a valid measurement when the observation is present.
			default:
				findings = append(findings, "duplicate "+variant.name+"."+name)
			}
		}
		for name := range counts {
			if _, ok := wanted[name]; !ok {
				findings = append(findings, "unexpected "+variant.name+"."+name)
			}
		}
	}
	sort.Strings(findings)
	return parityAudit{comparable: len(findings) == 0, findings: findings}
}

type scopeStatus string

const (
	scopeInScope      scopeStatus = "IN_SCOPE"
	scopeDrift        scopeStatus = "SCOPE_DRIFT"
	scopeInconclusive scopeStatus = "INCONCLUSIVE"
)

type scopeResult struct {
	status scopeStatus
	paths  []string
}

// inspectExactScope compares only paths attributed to the current turn. It is
// intentionally exact-path-only; it does not infer natural-language scope.
func inspectExactScope(allowedPaths, turnChangedPaths []string, changedPathsCapped bool) scopeResult {
	if changedPathsCapped || len(allowedPaths) == 0 {
		return scopeResult{status: scopeInconclusive}
	}
	allowed := make(map[string]struct{}, len(allowedPaths))
	for _, raw := range allowedPaths {
		clean, ok := normalizeRepoPath(raw)
		if !ok {
			return scopeResult{status: scopeInconclusive}
		}
		allowed[clean] = struct{}{}
	}
	var outside []string
	for _, raw := range turnChangedPaths {
		clean, ok := normalizeRepoPath(raw)
		if !ok {
			outside = append(outside, raw)
			continue
		}
		if _, ok := allowed[clean]; !ok {
			outside = append(outside, clean)
		}
	}
	sort.Strings(outside)
	if len(outside) > 0 {
		return scopeResult{status: scopeDrift, paths: outside}
	}
	return scopeResult{status: scopeInScope}
}

func normalizeRepoPath(raw string) (string, bool) {
	value := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if value == "" || strings.HasPrefix(value, "/") ||
		(len(value) >= 2 && value[1] == ':') {
		return "", false
	}
	for _, component := range strings.Split(value, "/") {
		if component == ".." {
			return "", false
		}
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

type effectState string

const (
	effectNotStarted effectState = "NOT_STARTED"
	effectConfirmed  effectState = "CONFIRMED"
	effectUnknown    effectState = "UNKNOWN_AFTER_CRASH"
)

// effectStateAfterCrash records only what the caller knows. A missing
// acknowledgement cannot prove that an external action did not happen.
func effectStateAfterCrash(attempted, acknowledged bool) effectState {
	if !attempted {
		return effectNotStarted
	}
	if acknowledged {
		return effectConfirmed
	}
	return effectUnknown
}

func effectMayBeRetried(state effectState) bool {
	return state == effectNotStarted
}

func effectUndoCanBeDeclaredComplete(state effectState) bool {
	return state == effectNotStarted
}
