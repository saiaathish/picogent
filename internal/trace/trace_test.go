package trace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendAndTailOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	ws := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	log, err := Open(ws)
	if err != nil {
		t.Fatal(err)
	}
	log.Now = func() time.Time { return time.Date(2026, 8, 20, 4, 0, 0, 0, time.UTC) }
	if err := log.Append("turn_start", "", "hi", nil, 0); err != nil {
		t.Fatal(err)
	}
	ok := true
	if err := log.Append("tool_end", "verify", "ok", &ok, 12); err != nil {
		t.Fatal(err)
	}
	got := log.Tail(10)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Seq != 1 || got[1].Seq != 2 {
		t.Fatalf("seq %+v", got)
	}
	if got[0].Kind != "turn_start" || got[1].Tool != "verify" {
		t.Fatalf("%+v", got)
	}
	if got[0].TS != "2026-08-20T04:00:00Z" {
		t.Fatalf("ts %s", got[0].TS)
	}
}

func TestAppendRedactsCredentialShapedDetails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	ws := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	log, err := Open(ws)
	if err != nil {
		t.Fatal(err)
	}
	detail := `{"api_key":"sk-live-secret-value","access_token":"access-secret-value","password":"pw-value","authorization":"Bearer should-hide","url":"https://example.test/?token=query-secret"}`
	if err := log.Append("tool_start", "web_fetch", detail, nil, 0); err != nil {
		t.Fatal(err)
	}
	got := log.Tail(1)
	if len(got) != 1 {
		t.Fatalf("events=%d", len(got))
	}
	for _, secret := range []string{"sk-live-secret-value", "access-secret-value", "pw-value", "should-hide", "query-secret"} {
		if strings.Contains(got[0].Detail, secret) {
			t.Fatalf("trace retained secret %q: %q", secret, got[0].Detail)
		}
	}
	if !strings.Contains(got[0].Detail, "[REDACTED]") {
		t.Fatalf("trace did not record a redaction marker: %q", got[0].Detail)
	}
}
