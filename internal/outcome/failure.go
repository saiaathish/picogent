package outcome

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/saiaathish/picogent/internal/redact"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/verify"
)

const (
	maxFailureFingerprintInput = 4096
	maxFailureRepeatCount      = 128
	failureFingerprintPrefix   = "failure-"
)

// FailureClass is a fixed, advisory category for a failed verification. It is
// derived from evidence but never copies evidence into a routing contract.
type FailureClass string

const (
	FailureClassCompiler        FailureClass = "compiler"
	FailureClassTests           FailureClass = "tests"
	FailureClassAuth            FailureClass = "auth"
	FailureClassDependency      FailureClass = "dependency"
	FailureClassConcurrency     FailureClass = "concurrency"
	FailureClassFrontendRuntime FailureClass = "frontend_runtime"
	FailureClassGeneratedDrift  FailureClass = "generated_drift"
	FailureClassWindowsPath     FailureClass = "windows_path"
	FailureClassUnknown         FailureClass = "unknown"
)

// FailureIntelligence is a bounded, derived failure signal. It intentionally
// contains no summary, command, path, model text, or suggested file. The
// fingerprint is a short digest, and Route is selected from a fixed
// vocabulary, so untrusted verification output cannot become instructions.
type FailureIntelligence struct {
	Class               FailureClass `json:"class,omitempty"`
	Fingerprint         string       `json:"fingerprint,omitempty"`
	RepeatCount         int          `json:"repeat_count,omitempty"`
	NeedsNewHypothesis  bool         `json:"needs_new_hypothesis,omitempty"`
	NeedsDifferentRoute bool         `json:"needs_different_route,omitempty"`
	Route               string       `json:"route,omitempty"`
}

// FailureIntelligenceForTask derives the latest contiguous failure signal
// without mutating the task. A passing verification clears the active signal;
// a different failure starts a new repeat count.
func FailureIntelligenceForTask(task *taskstate.Task) FailureIntelligence {
	if task == nil || len(task.Verification) == 0 {
		return FailureIntelligence{}
	}
	latest := task.Verification[len(task.Verification)-1]
	if latest.Passed || unavailableVerification(latest) {
		return FailureIntelligence{}
	}
	fingerprint := verificationFingerprint(latest)
	if fingerprint == "" {
		return FailureIntelligence{}
	}
	class := ClassifyFailure(latest.Summary, latest.Command)
	repeatCount := 0
	for i := len(task.Verification) - 1; i >= 0; i-- {
		verification := task.Verification[i]
		if verification.Passed {
			break
		}
		// The latest record already established that these exact fields describe
		// an actionable failure, so identical history needs no second parse.
		if verification.Summary == latest.Summary && verification.Command == latest.Command {
			repeatCount++
			continue
		}
		if unavailableVerification(verification) || verificationFingerprint(verification) != fingerprint {
			break
		}
		repeatCount++
	}
	return boundFailureIntelligence(FailureIntelligence{
		Class:         class,
		Fingerprint:   fingerprint,
		RepeatCount:   repeatCount,
		Route:         routeForFailureClass(class),
	})
}

// ClassifyFailure returns one fixed category from a bounded summary and
// command. It is a hint for choosing a discriminating inspection route, not a
// claim about root cause.
func ClassifyFailure(summary, command string) FailureClass {
	value := normalizeFailureText(summary + "\n" + command)
	if value == "" {
		return FailureClassUnknown
	}
	// More specific runtime and platform signatures win over generic test or
	// compiler words. The order is deterministic and deliberately conservative.
	switch {
	case hasFailureMarker(value, "windows", "win32", "invalid win32", "filename too long", "path too long", "cannot find the path", "the system cannot find"):
		return FailureClassWindowsPath
	case hasFailureMarker(value, "generated file", "generated files", "code generated", "generated output", "generated drift"):
		return FailureClassGeneratedDrift
	case hasFailureMarker(value, "uncaught typeerror", "uncaught referenceerror", "typeerror", "referenceerror", "hydration failed", "browser console", "frontend", "react", "vite", "webpack", "render error"):
		return FailureClassFrontendRuntime
	case hasFailureMarker(value, "data race", "race detector", "concurrent map", "deadlock", "fatal error: concurrent", "mutex", "goroutine leak"):
		return FailureClassConcurrency
	case hasFailureMarker(value, "authentication", "authorization", "unauthorized", "forbidden", "oauth", "credential", "access token", "api key", "permission denied", "401", "403"):
		return FailureClassAuth
	case hasFailureMarker(value, "module not found", "no required module provides", "cannot find package", "dependency", "go.sum", "package-lock", "pnpm-lock", "yarn.lock", "cargo", "version conflict", "peer dep", "no matching distribution"):
		return FailureClassDependency
	case hasFailureMarker(value, "undefined:", "syntax error", "cannot use", "does not compile", "compile error", "build failed", "type error", "no new variables", "too few values", "not enough arguments", "cannot find symbol"):
		return FailureClassCompiler
	case hasFailureMarker(value, "test failed", "tests failed", "--- fail:", "assertion", "verification reported failed tests", "go test", "npm test", "pnpm test", "cargo test"):
		return FailureClassTests
	default:
		return FailureClassUnknown
	}
}

// FailureFingerprint returns a compact deterministic digest of normalized
// evidence. Credential-shaped values are redacted before hashing so the
// digest never depends on a raw secret representation exposed to the model.
func FailureFingerprint(evidence string) string {
	normalized := normalizeFailureText(evidence)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return failureFingerprintPrefix + hex.EncodeToString(sum[:6])
}

// RequiresNewHypothesis reports the explicit route-diversity boundary.
func (f FailureIntelligence) RequiresNewHypothesis() bool {
	return f.NeedsNewHypothesis
}

// RequiresDifferentRoute reports whether the next repair must choose a
// materially different safe route.
func (f FailureIntelligence) RequiresDifferentRoute() bool {
	return f.NeedsDifferentRoute
}

func verificationFingerprint(verification taskstate.Verification) string {
	evidence := verification.Summary
	if strings.TrimSpace(evidence) == "" {
		evidence = verification.Command
	}
	return FailureFingerprint(evidence)
}

func unavailableVerification(verification taskstate.Verification) bool {
	if verification.Passed {
		return false
	}
	// Older callers may store a typed false with a short human summary. Only
	// suppress records that carry the verifier's explicit unavailable status.
	summary := strings.TrimSpace(strings.ToUpper(verification.Summary))
	if !strings.HasPrefix(summary, "VERIFY ") {
		return false
	}
	status := verify.StatusFromEvidence(verification.Summary)
	return status == verify.StatusInconclusive || status == verify.StatusSkipped
}

func normalizeFailureText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > maxFailureFingerprintInput {
		value = value[:maxFailureFingerprintInput]
	}
	value = redact.Text(value)
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	if len(value) > maxFailureFingerprintInput {
		value = value[:maxFailureFingerprintInput]
	}
	return value
}

func hasFailureMarker(value string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func routeForFailureClass(class FailureClass) string {
	switch class {
	case FailureClassCompiler:
		return "inspect compiler output and affected source"
	case FailureClassTests:
		return "inspect the failing test and its implementation"
	case FailureClassAuth:
		return "inspect the authentication boundary and safe configuration"
	case FailureClassDependency:
		return "inspect the dependency manifest and lockfile"
	case FailureClassConcurrency:
		return "inspect shared state and synchronization"
	case FailureClassFrontendRuntime:
		return "inspect the rendered surface and browser runtime evidence"
	case FailureClassGeneratedDrift:
		return "inspect generator inputs and generated-file ownership"
	case FailureClassWindowsPath:
		return "inspect platform-specific path and filesystem handling"
	case FailureClassUnknown:
		return "inspect the latest failure and choose a safe discriminating check"
	default:
		return ""
	}
}

func boundFailureIntelligence(failure FailureIntelligence) FailureIntelligence {
	failure.Class = normalizeFailureClass(failure.Class)
	failure.Fingerprint = normalizeFailureFingerprint(failure.Fingerprint)
	if failure.Class == "" || failure.Fingerprint == "" {
		return FailureIntelligence{}
	}
	if failure.RepeatCount < 1 {
		failure.RepeatCount = 1
	}
	if failure.RepeatCount > maxFailureRepeatCount {
		failure.RepeatCount = maxFailureRepeatCount
	}
	failure.NeedsNewHypothesis = failure.RepeatCount >= 2
	failure.NeedsDifferentRoute = failure.RepeatCount >= 2
	failure.Route = routeForFailureClass(failure.Class)
	return failure
}

func normalizeFailureClass(class FailureClass) FailureClass {
	switch strings.ToLower(strings.TrimSpace(string(class))) {
	case string(FailureClassCompiler):
		return FailureClassCompiler
	case string(FailureClassTests), "test":
		return FailureClassTests
	case string(FailureClassAuth), "authentication", "authorization":
		return FailureClassAuth
	case string(FailureClassDependency), "dependencies":
		return FailureClassDependency
	case string(FailureClassConcurrency), "race":
		return FailureClassConcurrency
	case string(FailureClassFrontendRuntime), "frontend-runtime", "frontend runtime":
		return FailureClassFrontendRuntime
	case string(FailureClassGeneratedDrift), "generated", "generated-file-drift":
		return FailureClassGeneratedDrift
	case string(FailureClassWindowsPath), "windows", "windows-path":
		return FailureClassWindowsPath
	case string(FailureClassUnknown):
		return FailureClassUnknown
	default:
		return ""
	}
}

func normalizeFailureFingerprint(fingerprint string) string {
	fingerprint = strings.TrimSpace(strings.ToLower(fingerprint))
	if len(fingerprint) != len(failureFingerprintPrefix)+12 || !strings.HasPrefix(fingerprint, failureFingerprintPrefix) {
		return ""
	}
	for _, char := range fingerprint[len(failureFingerprintPrefix):] {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return ""
		}
	}
	return fingerprint
}
