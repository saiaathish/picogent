package gui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/perm"
)

func TestRenderedPermissionContractKeepsDecisionBeforeSideEffect(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	s := &server{
		cfg:       cfg,
		sessionID: "rendered-permission-contract",
		turnGen:   1,
		permCh:    make(chan perm.Decision, 1),
	}
	h := newGUIHandlerAtWithPerm(s, s.sessionID, s.turnGen, s.permCh)

	for _, tc := range []struct {
		name      string
		allow     bool
		want      perm.Decision
		wantBytes []byte
	}{
		{name: "deny leaves file absent", allow: false, want: perm.Deny},
		{name: "allow writes only after approval", allow: true, want: perm.Allow, wantBytes: []byte("rendered permission probe\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(workspace, "rendered-probe.txt")
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			resultCh := make(chan struct {
				decision perm.Decision
				err      error
			}, 1)
			go func() {
				decision, err := h.OnNeedPermission(ctx, perm.Request{
					Tool:    "write_file",
					Summary: "write rendered-probe.txt",
					Hint:    "Safe mode requires approval before a file change.",
				})
				if err == nil && decision == perm.Allow {
					err = os.WriteFile(path, tc.wantBytes, 0o600)
				}
				resultCh <- struct {
					decision perm.Decision
					err      error
				}{decision: decision, err: err}
			}()

			deadline := time.NewTimer(time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				s.mu.Lock()
				pending := s.pendingPerm.Tool
				s.mu.Unlock()
				if pending == "write_file" {
					break
				}
				select {
				case <-deadline.C:
					t.Fatal("permission request was not exposed to the GUI boundary")
				case <-ticker.C:
				}
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("side effect occurred before permission decision: err=%v", err)
			}

			body := `{"allow":false}`
			if tc.allow {
				body = `{"allow":true}`
			}
			req := loopbackAPIRequest(http.MethodPost, "/api/permission", body)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://"+loopbackTestHost)
			res := httptest.NewRecorder()
			s.Handler().ServeHTTP(res, req)
			if res.Code != http.StatusNoContent {
				t.Fatalf("permission response status = %d, want %d", res.Code, http.StatusNoContent)
			}

			var result struct {
				decision perm.Decision
				err      error
			}
			select {
			case result = <-resultCh:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.decision != tc.want {
				t.Fatalf("permission decision = %s, want %s", result.decision, tc.want)
			}

			got, err := os.ReadFile(path)
			switch {
			case tc.allow && err != nil:
				t.Fatalf("allowed mutation was not published: %v", err)
			case tc.allow && string(got) != string(tc.wantBytes):
				t.Fatalf("allowed mutation = %q, want %q", got, tc.wantBytes)
			case !tc.allow && !errors.Is(err, os.ErrNotExist):
				t.Fatalf("denied mutation changed the workspace: bytes=%q err=%v", got, err)
			}

			s.mu.Lock()
			remaining := s.pendingPerm.Tool
			s.mu.Unlock()
			if remaining != "" {
				t.Fatalf("permission request remained visible after decision: %q", remaining)
			}
		})
	}
}
