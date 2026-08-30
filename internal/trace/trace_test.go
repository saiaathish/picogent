package trace

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/projects"
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

func TestAppendRedactsEveryTextualEventField(t *testing.T) {
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
	kindSecret := "kind-secret"
	toolSecret := "tool-secret"
	detailSecret := "detail-secret"
	if err := log.Append(
		"prompt token="+kindSecret,
		"authorization="+toolSecret,
		`{"arguments":{"access_token":"`+detailSecret+`"},"output":"mcp result"}`,
		nil,
		0,
	); err != nil {
		t.Fatal(err)
	}
	got := log.Tail(1)
	if len(got) != 1 {
		t.Fatalf("events=%d", len(got))
	}
	payload := strings.Join([]string{got[0].Kind, got[0].Tool, got[0].Detail}, "\n")
	for _, secret := range []string{kindSecret, toolSecret, detailSecret} {
		if strings.Contains(payload, secret) {
			t.Fatalf("trace retained secret %q: %q", secret, payload)
		}
	}
	if strings.Count(payload, "[REDACTED]") < 3 {
		t.Fatalf("trace did not redact every textual field: %q", payload)
	}
}

func TestTailRedactsLegacyCredentialShapedDetails(t *testing.T) {
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
	legacy := `{"seq":1,"ts":"2026-08-20T04:00:00Z","kind":"tool_end","detail":"api_key=sk-legacy-secret"}` + "\n"
	if err := os.WriteFile(log.Path(), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	got := log.Tail(1)
	if len(got) != 1 {
		t.Fatalf("events=%d", len(got))
	}
	if strings.Contains(got[0].Detail, "sk-legacy-secret") || !strings.Contains(got[0].Detail, "[REDACTED]") {
		t.Fatalf("legacy trace detail was not redacted: %q", got[0].Detail)
	}
}

func TestTailResanitizesLegacyTextualEventFields(t *testing.T) {
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
	legacy := `{"seq":1,"ts":"2026-08-20T04:00:00Z","kind":"token=legacy-kind-secret","tool":"authorization=legacy-tool-secret","detail":"password=legacy-detail-secret"}` + "\n"
	if err := os.WriteFile(log.Path(), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	got := log.Tail(1)
	if len(got) != 1 {
		t.Fatalf("events=%d", len(got))
	}
	payload := strings.Join([]string{got[0].Kind, got[0].Tool, got[0].Detail}, "\n")
	for _, secret := range []string{"legacy-kind-secret", "legacy-tool-secret", "legacy-detail-secret"} {
		if strings.Contains(payload, secret) {
			t.Fatalf("legacy trace retained secret %q: %q", secret, payload)
		}
	}
	if strings.Count(payload, "[REDACTED]") < 3 {
		t.Fatalf("legacy trace did not redact every textual field: %q", payload)
	}
}

func TestAppendBoundsTraceLog(t *testing.T) {
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
	for i := 0; i < 1600; i++ {
		if err := log.Append("tool_end", "test", strings.Repeat("x", 220), nil, int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(log.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > maxTraceBytes {
		t.Fatalf("trace log grew beyond bound: %d", info.Size())
	}
	got := log.Tail(1)
	if len(got) != 1 || got[0].Seq != 1600 {
		t.Fatalf("latest trace event=%+v", got)
	}
}

func TestAppendCoordinatesMultipleLogHandles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	ws := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := Open(ws)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(ws)
	if err != nil {
		t.Fatal(err)
	}
	const perHandle = 200
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, log := range []*Log{a, b} {
		wg.Add(1)
		go func(log *Log) {
			defer wg.Done()
			for i := 0; i < perHandle; i++ {
				if err := log.Append("tool_end", "parallel", "ok", nil, int64(i)); err != nil {
					errs <- err
					return
				}
			}
		}(log)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	events := a.Tail(perHandle * 2)
	if len(events) != perHandle*2 {
		t.Fatalf("events=%d, want %d", len(events), perHandle*2)
	}
	for i := 1; i < len(events); i++ {
		if events[i].Seq <= events[i-1].Seq {
			t.Fatalf("sequence not increasing at %d: %d then %d", i, events[i-1].Seq, events[i].Seq)
		}
	}
}

func TestOpenRejectsSymlinkedTracePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	workspace := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(home, "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logs, projects.IDForPath(workspace)+".jsonl")
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := Open(workspace); err == nil {
		t.Fatal("trace Open accepted a symlink target")
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "outside\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestAppendRejectsSymlinkedTracePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	workspace := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(home, "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logs, projects.IDForPath(workspace)+".jsonl")
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	log := &Log{path: path, Now: func() time.Time { return time.Now().UTC() }}
	if err := log.Append("test", "trace", "must not escape", nil, 0); err == nil {
		t.Fatal("trace Append accepted a symlink target")
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "outside\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}
