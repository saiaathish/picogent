package redact

import (
	"strings"
	"testing"
)

func TestTextRedactsCredentialShapes(t *testing.T) {
	secretText := strings.Join([]string{
		`api_key="api-secret"`,
		`access_token=access-secret`,
		`Authorization: Bearer bearer-secret`,
		`Authorization: Basic YWJjOnNlY3JldA==`,
		`https://user:url-secret@example.test/path`,
		`https://example.test/?token=query-secret`,
		`sk-live-secret-value`,
		`-----BEGIN OPENSSH PRIVATE KEY-----secret-----END OPENSSH PRIVATE KEY-----`,
	}, " ")
	got := Text(secretText)
	for _, secret := range []string{
		"api-secret", "access-secret", "bearer-secret", "YWJjOnNlY3JldA==",
		"url-secret", "query-secret", "sk-live-secret-value", "OPENSSH PRIVATE KEY-----secret",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("redaction retained %q: %q", secret, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") || !strings.Contains(got, "[REDACTED PRIVATE KEY]") {
		t.Fatalf("redaction markers missing: %q", got)
	}
}

func TestTextPreservesOrdinaryText(t *testing.T) {
	const want = "changed password documentation and tokenization notes"
	if got := Text(want); got != want {
		t.Fatalf("ordinary text changed: %q", got)
	}
}
