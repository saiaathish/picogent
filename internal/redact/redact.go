// Package redact removes common credential-shaped values before text is
// persisted or shown to a model.
package redact

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	privateKey      = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	terminalEscape  = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	sensitiveURL    = regexp.MustCompile(`(?i)(https?://[^/\s:@]+:)[^@\s/]+@`)
	sensitiveAssign = regexp.MustCompile(`(?i)(\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|token|secret|password|passwd|authorization|cookie|private[_-]?key|client[_-]?secret)\b[\s"]*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;}&]+)`)
	sensitiveQuery  = regexp.MustCompile(`(?i)([?&](?:api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|token|secret|password|key)=)[^&#\s]+`)
	bearerSecret    = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	basicSecret     = regexp.MustCompile(`(?i)\bBasic\s+[A-Za-z0-9+/=]+`)
	knownToken      = regexp.MustCompile(`(?i)\b(?:sk-[A-Za-z0-9_-]{8,}|gh[pousr]_[A-Za-z0-9_]{8,}|github_pat_[A-Za-z0-9_]{16,}|xox[baprs]-[A-Za-z0-9-]{8,}|AIza[A-Za-z0-9_-]{16,}|npm_[A-Za-z0-9]{16,})\b`)
)

// Text redacts common credential-shaped values. It is intentionally
// conservative: arbitrary repository text is not treated as a secret unless
// it has a recognizable credential label, transport form, or token prefix.
func Text(value string) string {
	value = privateKey.ReplaceAllString(value, "[REDACTED PRIVATE KEY]")
	value = sensitiveURL.ReplaceAllString(value, "$1[REDACTED]@")
	value = bearerSecret.ReplaceAllString(value, "Bearer [REDACTED]")
	value = basicSecret.ReplaceAllString(value, "Basic [REDACTED]")
	value = sensitiveAssign.ReplaceAllString(value, "$1[REDACTED]")
	value = sensitiveQuery.ReplaceAllString(value, "$1[REDACTED]")
	return knownToken.ReplaceAllString(value, "[REDACTED]")
}

// Diagnostic redacts credential-shaped values, removes terminal control
// characters, and bounds text intended for a compact user-facing diagnostic.
// It is deliberately separate from Text so ordinary model answers and
// persisted structured values keep their original formatting.
func Diagnostic(value string, limit int) string {
	value = terminalEscape.ReplaceAllString(value, " ")
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = Text(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
