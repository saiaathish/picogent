package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/perm"
)

func permissionAckFixture(t *testing.T, buffered bool) (*server, chan perm.Decision) {
	t.Helper()
	t.Setenv("PICOGENT_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	capacity := 0
	if buffered {
		capacity = 1
	}
	ch := make(chan perm.Decision, capacity)
	return &server{
		cfg:            cfg,
		ag:             &agent.Agent{Gate: perm.New(config.ModeSafe, cfg.Workspace, nil)},
		turnGen:        3,
		pendingPerm:    perm.Request{Tool: "write_file", Summary: "write a file"},
		pendingPermGen: 3,
		pendingPermID:  7,
		pendingPermCh:  ch,
	}, ch
}

func postPermissionAck(s *server, ctx context.Context, body string) *httptest.ResponseRecorder {
	res := httptest.NewRecorder()
	req := loopbackAPIRequest(http.MethodPost, "/api/permission", body).WithContext(ctx)
	s.permission(res, req)
	return res
}

func assertPermissionAckRetryable(t *testing.T, s *server, ch chan perm.Decision) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingPerm.Tool != "write_file" || s.pendingPermID != 7 || s.pendingPermGen != 3 || s.pendingPermCh != ch {
		t.Fatalf("pending prompt changed: %#v, id=%d gen=%d", s.pendingPerm, s.pendingPermID, s.pendingPermGen)
	}
	if s.pendingPermResponseID != 0 {
		t.Fatalf("response claim not released: %d", s.pendingPermResponseID)
	}
	if len(s.cfg.Extensions.AlwaysAllowTools) != 0 || len(s.ag.Gate.AlwaysAllowedTools()) != 0 {
		t.Fatal("undelivered choice changed future approval")
	}
}

func TestGUIPermissionResponseTimeoutPreservesRetry(t *testing.T) {
	s, ch := permissionAckFixture(t, false)
	res := postPermissionAck(s, context.Background(), `{"always":true,"permission_id":"7"}`)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("timeout status=%d, want 503", res.Code)
	}
	assertPermissionAckRetryable(t, s, ch)
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("undelivered Always choice saved config: %v", err)
	}
	go func() { <-ch }()
	res = postPermissionAck(s, context.Background(), `{"allow":true,"permission_id":"7"}`)
	if res.Code != http.StatusNoContent {
		t.Fatalf("retry status=%d, want 204", res.Code)
	}
}

func TestGUIPermissionResponseCanceledPreservesRetry(t *testing.T) {
	for _, buffered := range []bool{false, true} {
		t.Run(map[bool]string{false: "waiting", true: "send-ready"}[buffered], func(t *testing.T) {
			s, ch := permissionAckFixture(t, buffered)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			res := postPermissionAck(s, ctx, `{"always":true,"permission_id":"7"}`)
			if res.Code != http.StatusRequestTimeout {
				t.Fatalf("canceled status=%d, want 408", res.Code)
			}
			assertPermissionAckRetryable(t, s, ch)
			select {
			case d := <-ch:
				t.Fatalf("canceled choice was sent: %v", d)
			default:
			}
		})
	}
}

func TestGUIPermissionResponseOldGenerationRejectedBeforeSend(t *testing.T) {
	s, ch := permissionAckFixture(t, true)
	s.turnGen++
	res := postPermissionAck(s, context.Background(), `{"always":true,"permission_id":"7"}`)
	if res.Code != http.StatusConflict {
		t.Fatalf("old generation status=%d, want 409", res.Code)
	}
	assertPermissionAckRetryable(t, s, ch)
	if len(ch) != 0 {
		t.Fatal("stale generation sent a decision")
	}
}

func TestGUIPermissionResponseCompetingChoicesDeliverOnce(t *testing.T) {
	s, ch := permissionAckFixture(t, true)
	delivered := make(chan struct{})
	release := make(chan struct{})
	s.beforePermissionResponseCleanup = func() {
		close(delivered)
		<-release
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- postPermissionAck(s, context.Background(), `{"allow":true,"permission_id":"7"}`)
	}()
	select {
	case <-delivered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("first choice did not reach cleanup")
	}
	second := postPermissionAck(s, context.Background(), `{"always":true,"permission_id":"7"}`)
	close(release)
	if second.Code != http.StatusConflict {
		t.Fatalf("competing choice status=%d, want 409", second.Code)
	}
	select {
	case first := <-done:
		if first.Code != http.StatusNoContent {
			t.Fatalf("first choice status=%d, want 204", first.Code)
		}
	case <-time.After(time.Second):
		t.Fatal("first choice did not finish")
	}
	if len(ch) != 1 || <-ch != perm.Allow {
		t.Fatal("choices were duplicated or the first decision was replaced")
	}
	if len(s.cfg.Extensions.AlwaysAllowTools) != 0 {
		t.Fatal("rejected competing Always choice was persisted")
	}
}

func TestGUIPermissionResponseFailedPreferenceSaveStillAcknowledgesDelivery(t *testing.T) {
	s, ch := permissionAckFixture(t, true)
	blockedHome := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedHome, []byte("user data"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", blockedHome)
	events := make(chan event, 1)
	s.subs = []chan event{events}
	res := postPermissionAck(s, context.Background(), `{"always":true,"permission_id":"7"}`)
	if res.Code != http.StatusNoContent || len(ch) != 1 || <-ch != perm.AllowAlways {
		t.Fatalf("delivered choice not acknowledged: status=%d", res.Code)
	}
	s.mu.Lock()
	pending := s.pendingPerm
	s.mu.Unlock()
	if pending.Tool != "" {
		t.Fatal("delivered choice remained retryable")
	}
	if len(s.cfg.Extensions.AlwaysAllowTools) != 0 || len(s.ag.Gate.AlwaysAllowedTools()) != 0 {
		t.Fatal("failed save published a future-session preference")
	}
	select {
	case e := <-events:
		if e.Type != "error" || e.Text != "Your choice applied, but saving it for future sessions could not be confirmed." {
			t.Fatalf("save failure warning=%#v", e)
		}
	default:
		t.Fatal("preference save failure was hidden")
	}
}
