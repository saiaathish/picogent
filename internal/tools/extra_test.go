package tools

import (
	"context"
	"strings"
	"testing"
)

func TestWebFetchRejectsPrivateAndLocalTargetsAfterApproval(t *testing.T) {
	for _, raw := range []string{
		"http://localhost/metadata",
		"http://127.0.0.1:8080/",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]/",
		"http://100.100.100.100/",
	} {
		_, err := (webFetch{}).Run(context.Background(), `{"url":"`+raw+`"}`, Context{})
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "private or local") {
			t.Fatalf("web fetch %q error = %v, want private/local rejection", raw, err)
		}
	}
}

func TestWebFetchURLParserRejectsCredentials(t *testing.T) {
	if _, err := parseWebFetchURL("https://user:pass@example.com/"); err == nil {
		t.Fatal("URL credentials were accepted")
	}
}
