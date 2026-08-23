package gui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/perm"
)

const loopbackTestHost = "127.0.0.1:7420"

func newLoopbackAPITestServer(t *testing.T) *server {
	t.Helper()
	return newLoopbackAPITestServerAtHome(t, t.TempDir())
}

func newLoopbackAPITestServerAtHome(t *testing.T, home string) *server {
	t.Helper()
	t.Setenv("PICOGENT_HOME", home)
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	return &server{cfg: cfg, permCh: make(chan perm.Decision, 1)}
}

func loopbackAPIRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = loopbackTestHost
	return req
}

func TestGUIAPIRejectsUntrustedMutationOrigins(t *testing.T) {
	s := newLoopbackAPITestServer(t)
	h := s.Handler()
	for _, tc := range []struct {
		name   string
		host   string
		origin string
	}{
		{name: "cross origin", host: loopbackTestHost, origin: "https://evil.test"},
		{name: "origin absent", host: loopbackTestHost},
		{name: "opaque origin", host: loopbackTestHost, origin: "null"},
		{name: "wrong scheme", host: loopbackTestHost, origin: "https://127.0.0.1:7420"},
		{name: "loopback alias", host: loopbackTestHost, origin: "http://localhost:7420"},
		{name: "origin path", host: loopbackTestHost, origin: "http://127.0.0.1:7420/"},
		{name: "different loopback port", host: loopbackTestHost, origin: "http://127.0.0.1:7421"},
		{name: "rebound host", host: "evil.test:7420", origin: "http://evil.test:7420"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := loopbackAPIRequest(http.MethodPost, "/api/cancel", "")
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
			}
		})
	}
}

func TestGUIAPIAllowsSameLoopbackOriginForMutation(t *testing.T) {
	for _, tc := range []struct {
		host   string
		origin string
	}{
		{host: loopbackTestHost, origin: "http://" + loopbackTestHost},
		{host: "[::1]:7420", origin: "http://[::1]:7420"},
		{host: "127.0.0.1", origin: "http://127.0.0.1:80"},
		{host: "127.0.0.1:80", origin: "http://127.0.0.1"},
	} {
		t.Run(tc.host+" / "+tc.origin, func(t *testing.T) {
			s := newLoopbackAPITestServer(t)
			req := loopbackAPIRequest(http.MethodPost, "/api/cancel", "")
			req.Host = tc.host
			req.Header.Set("Origin", tc.origin)
			res := httptest.NewRecorder()
			s.Handler().ServeHTTP(res, req)
			if res.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusNoContent)
			}
		})
	}
}

func TestGUIAPIRejectsNonLiteralHostsForReads(t *testing.T) {
	s := newLoopbackAPITestServer(t)
	h := s.Handler()
	for _, host := range []string{
		"evil.test:7420", "127.0.0.2:7420", "127.0.0.1.evil.test:7420",
		"localhost.evil.test:7420", "127.0.0.1:not-a-port",
	} {
		for _, target := range []string{"/api/state", "/api/file?path=README.md"} {
			t.Run(host+" "+target, func(t *testing.T) {
				req := loopbackAPIRequest(http.MethodGet, target, "")
				req.Host = host
				res := httptest.NewRecorder()
				h.ServeHTTP(res, req)
				if res.Code != http.StatusForbidden {
					t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
				}
			})
		}
	}

	res := httptest.NewRecorder()
	h.ServeHTTP(res, loopbackAPIRequest(http.MethodGet, "/api/state", ""))
	if res.Code != http.StatusOK {
		t.Fatalf("same-loopback read status = %d, want %d", res.Code, http.StatusOK)
	}
}

func TestGUIAPIRejectsMultipleOriginHeaders(t *testing.T) {
	s := newLoopbackAPITestServer(t)
	req := loopbackAPIRequest(http.MethodPost, "/api/cancel", "")
	req.Header.Add("Origin", "http://"+loopbackTestHost)
	req.Header.Add("Origin", "https://evil.test")
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
}

func TestGUIAPIHTTPOriginBoundary(t *testing.T) {
	s := newLoopbackAPITestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	for _, tc := range []struct {
		name   string
		origin string
		want   int
	}{
		{name: "hostile", origin: "https://evil.test", want: http.StatusForbidden},
		{name: "same origin", origin: ts.URL, want: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/cancel", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Origin", tc.origin)
			res, err := ts.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}

func TestGUIAPIRejectsMethodMismatches(t *testing.T) {
	s := newLoopbackAPITestServer(t)
	h := s.Handler()
	paths := []string{
		"/api/state", "/api/setup", "/api/setup/install", "/api/setup/login", "/api/setup/finish",
		"/api/chat", "/api/scope", "/api/permission", "/api/mode", "/api/task-mode", "/api/cancel",
		"/api/reset", "/api/sessions", "/api/file", "/api/settings", "/api/router", "/api/projects",
		"/api/folder/pick", "/api/files/pick", "/api/files/read", "/api/overview", "/api/evolve",
		"/api/test", "/api/diff", "/api/extensions", "/api/trace", "/api/help", "/api/sidechat",
		"/api/prompts", "/api/events",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := loopbackAPIRequest(http.MethodPatch, path, "")
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusMethodNotAllowed)
			}
		})
	}
	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/reset"},
		{method: http.MethodGet, path: "/api/mode"},
		{method: http.MethodGet, path: "/api/chat"},
		{method: http.MethodPost, path: "/api/state"},
		{method: http.MethodPost, path: "/api/file"},
		{method: http.MethodPost, path: "/api/events"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			res := httptest.NewRecorder()
			h.ServeHTTP(res, loopbackAPIRequest(tc.method, tc.path, ""))
			if res.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestGUIAPIRejectsSimpleCrossSiteMutations(t *testing.T) {
	s := newLoopbackAPITestServer(t)
	h := s.Handler()
	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/api/mode", body: `{"mode":"fast"}`},
		{path: "/api/chat", body: `{"prompt":"change everything"}`},
		{path: "/api/permission", body: `{"allow":true}`},
		{path: "/api/setup/install", body: `{}`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			req := loopbackAPIRequest(http.MethodPost, tc.path, tc.body)
			req.Header.Set("Content-Type", "text/plain")
			req.Header.Set("Origin", "https://evil.test")
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
			}
		})
	}
}

func TestGUIAPIReadEndpointsDoNotPopulatePromptCaches(t *testing.T) {
	s := newLoopbackAPITestServer(t)
	h := s.Handler()
	for _, path := range []string{"/api/prompts?kind=main", "/api/prompts?kind=main&refresh=1", "/api/sidechat", "/api/projects"} {
		t.Run(path, func(t *testing.T) {
			res := httptest.NewRecorder()
			h.ServeHTTP(res, loopbackAPIRequest(http.MethodGet, path, ""))
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
			}
			if len(s.mainRecs) != 0 || len(s.sideRecs) != 0 {
				t.Fatalf("GET %s populated prompt caches: main=%d side=%d", path, len(s.mainRecs), len(s.sideRecs))
			}
		})
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("PICOGENT_HOME"), "projects.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("GET /api/projects created or changed project registry: %v", err)
	}
}

func TestGUIAPIPostReturnsFreshPromptCache(t *testing.T) {
	s := newLoopbackAPITestServer(t)
	s.mainRecs = []promptRec{{Title: "Already cached", Prompt: "Continue the task"}}
	s.mainRecsAt = time.Now()
	req := loopbackAPIRequest(http.MethodPost, "/api/prompts?kind=main", "")
	req.Header.Set("Origin", "http://"+loopbackTestHost)
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if !strings.Contains(res.Body.String(), "Already cached") {
		t.Fatalf("response did not return the fresh cache: %s", res.Body.String())
	}
}

func TestGUIAPIReadEndpointsDoNotCreateState(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "missing", "picogent")
	s := newLoopbackAPITestServerAtHome(t, home)
	h := s.Handler()
	for _, path := range []string{"/api/state", "/api/setup", "/api/overview", "/api/evolve"} {
		t.Run(path, func(t *testing.T) {
			res := httptest.NewRecorder()
			h.ServeHTTP(res, loopbackAPIRequest(http.MethodGet, path, ""))
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
			}
		})
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("GET API created state directory: %v", err)
	}
}

func TestGUIAPIAllowsOriginCheckedDelete(t *testing.T) {
	s := newLoopbackAPITestServer(t)
	req := loopbackAPIRequest(http.MethodDelete, "/api/evolve", `{"kind":"unknown"}`)
	req.Header.Set("Origin", "http://"+loopbackTestHost)
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
}

func TestGUIAPIDiffDisablesRepositoryConfiguredHelpers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}

	for _, tc := range []struct {
		name      string
		configure func(t *testing.T, repo, helper string)
	}{
		{
			name: "external diff",
			configure: func(t *testing.T, repo, helper string) {
				runGit(t, repo, "config", "diff.external", helper)
			},
		},
		{
			name: "text conversion",
			configure: func(t *testing.T, repo, helper string) {
				if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("content.txt diff=untrusted\\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				runGit(t, repo, "add", ".gitattributes")
				runGit(t, repo, "commit", "--quiet", "-m", "configure attributes")
				runGit(t, repo, "config", "diff.untrusted.textconv", helper)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			runGit(t, repo, "init", "--quiet")
			runGit(t, repo, "config", "user.email", "test@example.com")
			runGit(t, repo, "config", "user.name", "Picogent Test")
			content := filepath.Join(repo, "content.txt")
			if err := os.WriteFile(content, []byte("before\\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			runGit(t, repo, "add", "content.txt")
			runGit(t, repo, "commit", "--quiet", "-m", "initial")

			marker := filepath.Join(t.TempDir(), "helper-ran")
			t.Setenv("PICOGENT_TEST_DIFF_HELPER_MARKER", marker)
			helper := filepath.Join(t.TempDir(), "untrusted-diff-helper")
			if err := os.WriteFile(helper, []byte("#!/bin/sh\\n: > \\\"$PICOGENT_TEST_DIFF_HELPER_MARKER\\\"\\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			tc.configure(t, repo, helper)
			if err := os.WriteFile(content, []byte("after\\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			s := newLoopbackAPITestServer(t)
			s.cfg.Workspace = repo
			res := httptest.NewRecorder()
			s.Handler().ServeHTTP(res, loopbackAPIRequest(http.MethodGet, "/api/diff", ""))
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", res.Code, http.StatusOK, res.Body.String())
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("repository-configured helper ran: %v", err)
			}
			if !strings.Contains(res.Body.String(), "after") {
				t.Fatalf("ordinary diff output missing changed content: %s", res.Body.String())
			}
		})
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}
