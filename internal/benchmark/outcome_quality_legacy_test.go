package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// The legacy executor deliberately launches an opaque v3 command. These
// subprocess modes let Execute failure and cancellation paths be tested
// without compiling another fixture binary or depending on a live provider.
// The marker is injected only by outcomeQualityLegacyEnvironment; normal test
// invocations never enter this helper.
func init() {
	if os.Getenv("PICOGENT_OUTCOME_QUALITY_LEGACY_CHILD") != "1" || len(os.Args) < 3 || os.Args[1] != "run" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "legacy-test-command-failure":
		fmt.Fprintln(os.Stderr, "legacy test command failure")
		os.Exit(7)
	case "legacy-test-provider-failure":
		baseURL := strings.TrimRight(os.Getenv("PICOGENT_BASE_URL"), "/")
		response, err := http.Post(baseURL+"/chat/completions", "application/json", strings.NewReader(`{"model":"test","messages":[]}`))
		if err != nil {
			fmt.Fprintln(os.Stderr, "legacy test provider request:", err)
			os.Exit(8)
		}
		_ = response.Body.Close()
		if response.StatusCode < http.StatusBadRequest {
			fmt.Fprintln(os.Stderr, "legacy test provider unexpectedly succeeded")
			os.Exit(9)
		}
		os.Exit(7)
	case "legacy-test-timeout":
		select {}
	}
}

func TestBuildOutcomeQualityLegacyLaunchesExactV3Binary(t *testing.T) {
	legacySource := cleanOutcomeQualitySourceAtHead(t, "a07943b31044049afb0142f39198244cd3c75218")
	head := outcomeQualityGitHead(t, legacySource)
	server := newOutcomeQualityLegacyProvider(t)
	build, err := BuildOutcomeQualityLegacy(context.Background(), OutcomeQualitySourceBinding{
		Target:    outcomeQualityLegacySourceTarget(head),
		Workspace: legacySource,
	}, OutcomeQualityLegacyBuildConfig{
		TempParent:  t.TempDir(),
		ProviderURL: server.server.URL,
		Model:       "legacy-fixture-model",
	})
	if err != nil {
		t.Fatalf("build legacy v3: %v", err)
	}
	defer build.Close()
	executor := build.ProcessExecutor()
	if executor == nil || executor.Command == "" {
		t.Fatal("legacy build returned no process executor")
	}
	if filepath.Base(executor.Command) == "outcome-quality-worker" || filepath.Base(executor.Command) == "outcome-quality-worker.exe" {
		t.Fatalf("legacy executor points at v4 worker: %q", executor.Command)
	}
	if outcomeQualityPathWithin(executor.Command, legacySource) {
		t.Fatalf("legacy binary %q is inside source workspace %q", executor.Command, legacySource)
	}
	if _, err := os.Stat(executor.Command); err != nil {
		t.Fatalf("built legacy binary: %v", err)
	}

	request := outcomeQualityLegacyTestRequest(t, outcomeQualityLegacySourceTarget(head))
	execution, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute exact v3 binary: %v (provider requests=%d)", err, server.requestCount())
	}
	if execution.Metrics.OutcomeSuccess != OutcomeAssessmentPass || execution.Metrics.Correctness != OutcomeAssessmentPass {
		t.Fatalf("legacy metrics=%#v, want filesystem pass", execution.Metrics)
	}
	if execution.Metrics.VerificationQuality != OutcomeVerificationPass || execution.Metrics.Evidence != EvidenceCurrent {
		t.Fatalf("legacy verification metrics=%#v, want current pass", execution.Metrics)
	}
	if execution.Metrics.Tokens != 80 || execution.Metrics.ModelCalls != 4 || execution.Metrics.ChangedLines != 1 || execution.Metrics.UnnecessaryChanges != 0 || execution.Metrics.ToolCalls != 5 {
		t.Fatalf("legacy filesystem metrics=%#v, want one changed line, no extras, five tools", execution.Metrics)
	}
	for _, want := range []string{
		"legacy v3 does not expose structured repair counts",
		"legacy v3 does not expose context-growth measurement",
		"legacy v3 token and model-call counts are observed at the local provider boundary",
	} {
		if !containsOutcomeQualityReason(execution.Unverified, want) {
			t.Fatalf("legacy unverified=%v, missing %q", execution.Unverified, want)
		}
	}
	if got := server.requestCount(); got != 4 {
		t.Fatalf("provider requests=%d, want four typed v3 model turns", got)
	}
	if err := validateOutcomeQualitySourceBinding(context.Background(), "legacy post-test", outcomeQualityLegacySourceTarget(head), legacySource); err != nil {
		t.Fatalf("legacy source changed during build/run: %v", err)
	}
}

func TestOutcomeQualityLegacyToolchainMatchesDeclaredVersion(t *testing.T) {
	command, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOutcomeQualityLegacyToolchain(context.Background(), command, runtime.Version()); err != nil {
		t.Fatalf("current Go toolchain rejected: %v", err)
	}
	if err := validateOutcomeQualityLegacyToolchain(context.Background(), command, "go0.0.0"); err == nil || !strings.Contains(err.Error(), "does not match declared") {
		t.Fatalf("mismatched Go toolchain error=%v, want declared-version rejection", err)
	}
}

func TestOutcomeQualityLegacyProviderURLRequiresLoopbackHTTP(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{name: "empty", url: ""},
		{name: "https", url: "https://127.0.0.1:8080"},
		{name: "public host", url: "http://example.com:8080"},
		{name: "credentials", url: "http://user:pass@127.0.0.1:8080"},
		{name: "query", url: "http://127.0.0.1:8080/v1?token=secret"},
		{name: "bad port", url: "http://127.0.0.1:0"},
		{name: "empty port", url: "http://127.0.0.1:"},
		{name: "empty ipv6 port", url: "http://[::1]:"},
		{name: "out of range port", url: "http://127.0.0.1:65536"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := normalizeOutcomeQualityLegacyProviderURL(tc.url); err == nil {
				t.Fatalf("provider URL %q unexpectedly accepted", tc.url)
			}
		})
	}
	for _, raw := range []string{"http://127.0.0.1:8080/v1/", "http://localhost:8080"} {
		if got, err := normalizeOutcomeQualityLegacyProviderURL(raw); err != nil || strings.HasSuffix(got, "/") {
			t.Fatalf("valid provider URL %q normalized to %q, err=%v", raw, got, err)
		}
	}
}

func TestOutcomeQualityLegacyEnvironmentDoesNotInheritCredentialsOrWorkerSettings(t *testing.T) {
	t.Setenv("PICOGENT_API_KEY", "real-secret")
	t.Setenv("PICOGENT_CODEX_HOME", "/untrusted/codex")
	t.Setenv("OLLAMA_HOST", "http://untrusted")
	env := outcomeQualityLegacyEnvironment("/tmp/legacy-home", "/tmp/legacy-tmp", "/tmp/legacy-cache", "http://127.0.0.1:8080", "fixture-model")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "real-secret") || strings.Contains(joined, "PICOGENT_CODEX_HOME=") || strings.Contains(joined, "OLLAMA_HOST=") {
		t.Fatalf("legacy environment inherited untrusted settings: %q", joined)
	}
	if !strings.Contains(joined, "PICOGENT_API_KEY="+outcomeQualityLegacyLocalAPIKey) {
		t.Fatalf("legacy environment lacks fixed local provider key: %q", joined)
	}
	if strings.Contains(joined, "PICOGENT_OUTCOME_QUALITY_WORKER_CHILD=") {
		t.Fatalf("legacy environment inherited v4 worker marker: %q", joined)
	}
}

func TestOutcomeQualityLegacyBuildEnvironmentUsesExternalGoCaches(t *testing.T) {
	t.Setenv("GOCACHE", "/untrusted/source/go-cache")
	t.Setenv("GOMODCACHE", "/untrusted/source/go-mod-cache")
	t.Setenv("GOPATH", "/untrusted/source/go-path")
	env := outcomeQualityLegacyBuildEnvironment("/tmp/legacy-cache", "/tmp/legacy-mod-cache")
	joined := strings.Join(env, "\n")
	for _, want := range []string{
		"GOCACHE=/tmp/legacy-cache",
		"GOMODCACHE=/tmp/legacy-mod-cache",
		"GOPATH=/tmp/go-path",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("legacy build environment lacks %q: %q", want, joined)
		}
	}
}

func TestRemoveOutcomeQualityLegacyDirHandlesReadOnlyModuleCache(t *testing.T) {
	root := filepath.Join(t.TempDir(), "build")
	cache := filepath.Join(root, "go-mod-cache", "module@v1.0.0")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "go.mod"), []byte("module fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cache, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := removeOutcomeQualityLegacyDir(root); err != nil {
		t.Fatalf("remove read-only legacy build tree: %v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy build tree still exists: err=%v", err)
	}
}

func TestOutcomeQualityLegacyRejectsV4WorkerCommand(t *testing.T) {
	source := t.TempDir()
	command := filepath.Join(t.TempDir(), "outcome-quality-worker")
	if err := os.WriteFile(command, []byte("worker"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := validateOutcomeQualityLegacyCommand(command, source); err == nil || !strings.Contains(err.Error(), "cannot use the v4 outcome-quality worker") {
		t.Fatalf("worker command error=%v, want explicit v4-worker rejection", err)
	}
}

func TestOutcomeQualityLegacyRequiresAllowlistedSourceHead(t *testing.T) {
	target := outcomeQualityLegacySourceTarget(OutcomeQualityLegacySourceHead)
	if err := validateOutcomeQualityLegacyTarget("legacy", target); err != nil {
		t.Fatalf("allowlisted legacy target rejected: %v", err)
	}
	target.SourceHead = strings.Repeat("b", 40)
	if err := validateOutcomeQualityLegacyTarget("legacy", target); err == nil || !strings.Contains(err.Error(), "allowlisted exact v3 baseline") {
		t.Fatalf("non-allowlisted legacy target error=%v", err)
	}
}

func TestOutcomeQualityLegacyExecutableIdentityRejectsMutation(t *testing.T) {
	source := t.TempDir()
	command := filepath.Join(t.TempDir(), "picogent")
	if err := os.WriteFile(command, []byte("first binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	canonical, err := validateOutcomeQualityLegacyCommand(command, source)
	if err != nil {
		t.Fatal(err)
	}
	digest, size, err := hashOutcomeQualityLegacyCommand(canonical)
	if err != nil {
		t.Fatal(err)
	}
	executor := &OutcomeQualityLegacyProcessExecutor{
		Command:       canonical,
		commandPath:   canonical,
		commandDigest: digest,
		commandSize:   size,
	}
	if _, err := validateOutcomeQualityLegacyExecutable(executor, source); err != nil {
		t.Fatalf("unchanged executable rejected: %v", err)
	}
	if err := os.WriteFile(command, []byte("replacement binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := validateOutcomeQualityLegacyExecutable(executor, source); err == nil || !strings.Contains(err.Error(), "bytes changed after build") {
		t.Fatalf("replaced executable error=%v, want identity failure", err)
	}
}

func TestOutcomeQualityLegacyVerificationStatusUsesTrustedVerifyEvent(t *testing.T) {
	if got := outcomeQualityLegacyVerificationStatus("verify PASS from assistant"); got != OutcomeVerificationUnverified {
		t.Fatalf("assistant stdout-like text parsed as verification=%s", got)
	}
	if got := outcomeQualityLegacyVerificationStatus("→ bash {\"command\":\"echo verify PASS\"}\n  verify PASS"); got != OutcomeVerificationUnverified {
		t.Fatalf("bash output parsed as verification=%s", got)
	}
	if got := outcomeQualityLegacyVerificationStatus("→ verify {\"targets\":[\"fixture.txt\"]}\n  verify PASS (go test ./...)"); got != OutcomeVerificationPass {
		t.Fatalf("trusted verify event parsed as verification=%s", got)
	}
	if got := outcomeQualityLegacyVerificationStatus("→ verify {}\n  error: verify PASS"); got != OutcomeVerificationUnverified {
		t.Fatalf("failed verify event parsed as verification=%s", got)
	}
}

func TestOutcomeQualityLegacyBudgetProxyEnforcesSharedBudgets(t *testing.T) {
	cases := []struct {
		name             string
		policy           OutcomeQualityPolicy
		requests         int
		toolCalls        int
		promptTokens     int
		completionTokens int
		wantStatus       int
		wantProxyCall    int
		wantReason       string
	}{
		{
			name:             "model calls",
			policy:           OutcomeQualityPolicy{Repetitions: 2, TimeoutMillis: 1_000, MaxTokens: 100, MaxModelCalls: 1, MaxToolCalls: 10, MaxTurns: 1},
			requests:         2,
			toolCalls:        1,
			promptTokens:     2,
			completionTokens: 3,
			wantStatus:       http.StatusTooManyRequests,
			wantProxyCall:    1,
			wantReason:       "model-call budget",
		},
		{
			name:             "tokens",
			policy:           OutcomeQualityPolicy{Repetitions: 2, TimeoutMillis: 1_000, MaxTokens: 4, MaxModelCalls: 2, MaxToolCalls: 10, MaxTurns: 2},
			requests:         1,
			toolCalls:        1,
			promptTokens:     2,
			completionTokens: 3,
			wantStatus:       http.StatusTooManyRequests,
			wantProxyCall:    1,
			wantReason:       "token budget",
		},
		{
			name:             "tool calls",
			policy:           OutcomeQualityPolicy{Repetitions: 2, TimeoutMillis: 1_000, MaxTokens: 100, MaxModelCalls: 2, MaxToolCalls: 1, MaxTurns: 2},
			requests:         1,
			toolCalls:        2,
			promptTokens:     2,
			completionTokens: 3,
			wantStatus:       http.StatusTooManyRequests,
			wantProxyCall:    1,
			wantReason:       "tool-call budget",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				toolCalls := make([]map[string]any, tc.toolCalls)
				for index := range toolCalls {
					toolCalls[index] = map[string]any{"id": fmt.Sprintf("tool-%d", index)}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []any{map[string]any{"message": map[string]any{"tool_calls": toolCalls}}},
					"usage":   map[string]any{"prompt_tokens": tc.promptTokens, "completion_tokens": tc.completionTokens},
				})
			}))
			defer upstream.Close()

			proxy, err := newOutcomeQualityLegacyBudgetProxy(upstream.URL, tc.policy)
			if err != nil {
				t.Fatal(err)
			}
			defer proxy.Close()
			for index := 0; index < tc.requests; index++ {
				response, err := http.Post(proxy.URL()+"/chat/completions", "application/json", strings.NewReader(`{"model":"fixture"}`))
				if err != nil {
					t.Fatal(err)
				}
				body, readErr := io.ReadAll(response.Body)
				closeErr := response.Body.Close()
				if readErr != nil || closeErr != nil {
					t.Fatalf("read proxy response: read=%v close=%v", readErr, closeErr)
				}
				if index == tc.requests-1 && (response.StatusCode != tc.wantStatus || !strings.Contains(string(body), tc.wantReason)) {
					t.Fatalf("proxy response status=%d body=%q, want status=%d containing %q", response.StatusCode, body, tc.wantStatus, tc.wantReason)
				}
			}
			if calls != tc.wantProxyCall {
				t.Fatalf("upstream calls=%d, want %d", calls, tc.wantProxyCall)
			}
		})
	}
}

func TestOutcomeQualityLegacyBudgetProxyValidatesAndNormalizesInputs(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer upstream.Close()

	validPolicy := OutcomeQualityPolicy{
		Repetitions:   2,
		TimeoutMillis: 1_000,
		MaxTokens:     10,
		MaxModelCalls: 2,
		MaxToolCalls:  2,
		MaxTurns:      2,
	}
	proxy, err := newOutcomeQualityLegacyBudgetProxy(upstream.URL+"/v1/", validPolicy)
	if err != nil {
		t.Fatalf("valid proxy inputs rejected: %v", err)
	}
	proxy.Close()

	if _, err := newOutcomeQualityLegacyBudgetProxy("http://example.com", validPolicy); err == nil || !strings.Contains(err.Error(), "not loopback") {
		t.Fatalf("non-loopback proxy URL error=%v", err)
	}
	if _, err := newOutcomeQualityLegacyBudgetProxy(upstream.URL, OutcomeQualityPolicy{}); err == nil || !strings.Contains(err.Error(), "repetitions") {
		t.Fatalf("invalid proxy policy error=%v", err)
	}
}

func TestOutcomeQualityLegacyBudgetProxyRejectsMissingUsage(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer upstream.Close()
	proxy, err := newOutcomeQualityLegacyBudgetProxy(upstream.URL, validOutcomeQualityLegacyProxyPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	response, err := http.Post(proxy.URL()+"/chat/completions", "application/json", strings.NewReader(`{"model":"fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read proxy response: read=%v close=%v", readErr, closeErr)
	}
	if response.StatusCode != http.StatusBadGateway || !strings.Contains(string(body), "usage is missing") {
		t.Fatalf("proxy response status=%d body=%q, want missing-usage 502", response.StatusCode, body)
	}
	if calls != 1 {
		t.Fatalf("upstream calls=%d, want one", calls)
	}
}

func TestOutcomeQualityLegacyBudgetProxyRejectsRedirects(t *testing.T) {
	redirectTargetCalls := 0
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectTargetCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer redirectTarget.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()
	proxy, err := newOutcomeQualityLegacyBudgetProxy(upstream.URL, validOutcomeQualityLegacyProxyPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	response, err := http.Post(proxy.URL()+"/chat/completions", "application/json", strings.NewReader(`{"model":"fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read proxy response: read=%v close=%v", readErr, closeErr)
	}
	if response.StatusCode != http.StatusBadGateway || !strings.Contains(string(body), "redirects are not allowed") {
		t.Fatalf("proxy response status=%d body=%q, want redirect rejection", response.StatusCode, body)
	}
	if redirectTargetCalls != 0 {
		t.Fatalf("redirect target calls=%d, want zero", redirectTargetCalls)
	}
}

func TestOutcomeQualityLegacyProviderUsageRejectsTokenOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	payload, err := json.Marshal(map[string]any{
		"usage": map[string]int{
			"prompt_tokens":     maxInt,
			"completion_tokens": 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := outcomeQualityLegacyProviderUsage(payload); err == nil || !strings.Contains(err.Error(), "overflows integer range") {
		t.Fatalf("overflow usage error=%v, want explicit overflow rejection", err)
	}
}

func TestOutcomeQualityLegacyBudgetProxyBoundsPayloads(t *testing.T) {
	t.Run("request", func(t *testing.T) {
		calls := 0
		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
		defer upstream.Close()
		proxy, err := newOutcomeQualityLegacyBudgetProxy(upstream.URL, validOutcomeQualityLegacyProxyPolicy())
		if err != nil {
			t.Fatal(err)
		}
		defer proxy.Close()

		response, err := http.Post(proxy.URL()+"/chat/completions", "application/json", strings.NewReader(strings.Repeat("x", maxOutcomeQualityLegacyProviderPayloadBytes+1)))
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read proxy response: read=%v close=%v", readErr, closeErr)
		}
		if response.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(string(body), "request is too large") {
			t.Fatalf("proxy response status=%d body=%q, want oversized-request 413", response.StatusCode, body)
		}
		if calls != 0 {
			t.Fatalf("upstream calls=%d, want zero", calls)
		}
	})

	t.Run("response", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, strings.Repeat("x", maxOutcomeQualityLegacyProviderPayloadBytes+1))
		}))
		defer upstream.Close()
		proxy, err := newOutcomeQualityLegacyBudgetProxy(upstream.URL, validOutcomeQualityLegacyProxyPolicy())
		if err != nil {
			t.Fatal(err)
		}
		defer proxy.Close()

		response, err := http.Post(proxy.URL()+"/chat/completions", "application/json", strings.NewReader(`{"model":"fixture"}`))
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read proxy response: read=%v close=%v", readErr, closeErr)
		}
		if response.StatusCode != http.StatusBadGateway || !strings.Contains(string(body), "response is too large") {
			t.Fatalf("proxy response status=%d body=%q, want oversized-response 502", response.StatusCode, body)
		}
	})
}

func TestOutcomeQualityLegacyBuildCloseIsIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "build")
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	build := &OutcomeQualityLegacyBuild{dir: dir}
	if err := build.Close(); err != nil {
		t.Fatalf("close legacy build: %v", err)
	}
	if err := build.Close(); err != nil {
		t.Fatalf("second close legacy build: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy build directory still exists after close: %v", err)
	}
}

func TestOutcomeQualityLegacyExecuteSurfacesCommandFailureAndCleansRunDir(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeLegacyProviderResponse(w, nil, "unused")
	}))
	t.Cleanup(provider.Close)
	executor, request, tempParent := newOutcomeQualityLegacyTestExecutor(t, provider.URL, "legacy-test-command-failure")

	if _, err := executor.Execute(context.Background(), request); err == nil || !strings.Contains(err.Error(), "command failed") {
		t.Fatalf("legacy command failure=%v, want command failure", err)
	}
	assertOutcomeQualityLegacyRunDirRemoved(t, tempParent)
}

func TestOutcomeQualityLegacyExecuteSurfacesProviderFailureAndCleansRunDir(t *testing.T) {
	var requests int
	var requestsMu sync.Mutex
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestsMu.Lock()
		requests++
		requestsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "not-json")
	}))
	t.Cleanup(provider.Close)
	executor, request, tempParent := newOutcomeQualityLegacyTestExecutor(t, provider.URL, "legacy-test-provider-failure")

	if _, err := executor.Execute(context.Background(), request); err == nil || !strings.Contains(err.Error(), "command failed") {
		t.Fatalf("legacy provider failure=%v, want command failure", err)
	}
	requestsMu.Lock()
	gotRequests := requests
	requestsMu.Unlock()
	if gotRequests != 1 {
		t.Fatalf("provider requests=%d, want one rejected request", gotRequests)
	}
	assertOutcomeQualityLegacyRunDirRemoved(t, tempParent)
}

func TestOutcomeQualityLegacyExecuteTimeoutCleansRunDir(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeLegacyProviderResponse(w, nil, "unused")
	}))
	t.Cleanup(provider.Close)
	executor, request, tempParent := newOutcomeQualityLegacyTestExecutor(t, provider.URL, "legacy-test-timeout")
	request.Policy.TimeoutMillis = 100

	if _, err := executor.Execute(context.Background(), request); err == nil {
		t.Fatal("legacy timeout unexpectedly succeeded")
	}
	assertOutcomeQualityLegacyRunDirRemoved(t, tempParent)
}

func newOutcomeQualityLegacyTestExecutor(t *testing.T, providerURL, prompt string) (*OutcomeQualityLegacyProcessExecutor, OutcomeQualityExecutionRequest, string) {
	t.Helper()
	legacySource := cleanOutcomeQualitySourceAtHead(t, OutcomeQualityLegacySourceHead)
	target := outcomeQualityLegacySourceTarget(OutcomeQualityLegacySourceHead)
	command, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	command, err = validateOutcomeQualityLegacyCommand(command, legacySource)
	if err != nil {
		t.Fatalf("validate test helper command: %v", err)
	}
	digest, size, err := hashOutcomeQualityLegacyCommand(command)
	if err != nil {
		t.Fatalf("hash test helper command: %v", err)
	}
	input := outcomeQualityLegacyInput(DefaultOutcomeQualityScenarios()[0])
	input.Prompt = prompt
	digestInput := outcomeQualityInputDigest(input)
	scenario := DefaultOutcomeQualityScenarios()[0]
	scenario.InputSHA256 = digestInput
	request := OutcomeQualityExecutionRequest{
		Scenario:    scenario,
		Variant:     OutcomeVariantBaseline,
		Repetition:  1,
		InputSHA256: digestInput,
		Input:       input,
		Target:      target,
		Policy:      validOutcomeQualityLegacyProxyPolicy(),
	}
	tempParent := t.TempDir()
	return &OutcomeQualityLegacyProcessExecutor{
		Command:       command,
		Binding:       OutcomeQualitySourceBinding{Target: target, Workspace: legacySource},
		ProviderURL:   providerURL,
		Model:         "legacy-test-model",
		TempParent:    tempParent,
		commandPath:   command,
		commandDigest: digest,
		commandSize:   size,
	}, request, tempParent
}

func assertOutcomeQualityLegacyRunDirRemoved(t *testing.T, tempParent string) {
	t.Helper()
	entries, err := os.ReadDir(tempParent)
	if err != nil {
		t.Fatalf("read legacy temp parent: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "picogent-outcome-quality-legacy-run-") {
			t.Fatalf("legacy run directory leaked: %q", entry.Name())
		}
	}
}

func TestOutcomeQualityLegacyCommandRejectsInvalidIdentity(t *testing.T) {
	source, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(t.TempDir(), "picogent")
	if err := os.WriteFile(valid, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	nonExecutable := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(nonExecutable, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(source, "picogent")
	if err := os.WriteFile(inside, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "empty", path: "", want: "is required"},
		{name: "relative", path: "picogent", want: "absolute path"},
		{name: "directory", path: directory, want: "not a regular file"},
		{name: "inside source", path: inside, want: "outside source workspace"},
	}
	if runtime.GOOS != "windows" {
		cases = append(cases, struct {
			name string
			path string
			want string
		}{name: "not executable", path: nonExecutable, want: "not executable"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateOutcomeQualityLegacyCommand(tc.path, source); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("command %q error=%v, want %q", tc.path, err, tc.want)
			}
		})
	}
}

func TestOutcomeQualityLegacyRequestRejectsRepetitionOutsidePolicy(t *testing.T) {
	target := outcomeQualityLegacySourceTarget(OutcomeQualityLegacySourceHead)
	scenario := DefaultOutcomeQualityScenarios()[0]
	input, err := normalizeOutcomeQualityInput(outcomeQualityLegacyInput(scenario))
	if err != nil {
		t.Fatal(err)
	}
	digest := outcomeQualityInputDigest(input)
	scenario.InputSHA256 = digest
	request := OutcomeQualityExecutionRequest{
		Scenario:    scenario,
		Variant:     OutcomeVariantBaseline,
		Repetition:  3,
		InputSHA256: digest,
		Input:       input,
		Target:      target,
		Policy:      validOutcomeQualityLegacyProxyPolicy(),
	}
	if _, err := validateOutcomeQualityLegacyRequest(request); err == nil || !strings.Contains(err.Error(), "exceeds policy repetitions") {
		t.Fatalf("repetition validation error=%v, want policy bound rejection", err)
	}
}

func TestInspectOutcomeQualityLegacyWorkspaceBoundsEntries(t *testing.T) {
	root := t.TempDir()
	input := OutcomeQualityInput{Files: []OutcomeQualityInputFile{{Path: "fixture.txt", Content: "before"}}}
	if err := writeOutcomeQualityFixture(context.Background(), root, input); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maxOutcomeQualityLegacyWorkspaceEntries; index++ {
		path := filepath.Join(root, fmt.Sprintf("extra-%03d.txt", index))
		if err := os.WriteFile(path, []byte("extra"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, _, _, err := inspectOutcomeQualityLegacyWorkspace(context.Background(), root, input, map[string]string{"fixture.txt": "before"})
	if err == nil || !strings.Contains(err.Error(), "more than 512 filesystem entries") {
		t.Fatalf("unbounded legacy workspace inspection error=%v", err)
	}
}

func validOutcomeQualityLegacyProxyPolicy() OutcomeQualityPolicy {
	return OutcomeQualityPolicy{
		Repetitions:   2,
		TimeoutMillis: 1_000,
		MaxTokens:     100,
		MaxModelCalls: 2,
		MaxToolCalls:  10,
		MaxTurns:      2,
	}
}

func outcomeQualityLegacySourceTarget(head string) OutcomeQualityTarget {
	return OutcomeQualityTarget{
		SourceHead:  head,
		Host:        runtime.GOOS + "/" + runtime.GOARCH,
		GoVersion:   runtime.Version(),
		ToolVersion: OutcomeQualityRunnerToolVersion,
	}
}

func outcomeQualityLegacyTestRequest(t *testing.T, target OutcomeQualityTarget) OutcomeQualityExecutionRequest {
	t.Helper()
	scenario := DefaultOutcomeQualityScenarios()[0]
	input, err := normalizeOutcomeQualityInput(outcomeQualityLegacyInput(scenario))
	if err != nil {
		t.Fatal(err)
	}
	digest := outcomeQualityInputDigest(input)
	scenario.InputSHA256 = digest
	return OutcomeQualityExecutionRequest{
		Scenario:    scenario,
		Variant:     OutcomeVariantBaseline,
		Repetition:  1,
		InputSHA256: digest,
		Input:       input,
		Target:      target,
		Policy:      testOutcomeQualityRunnerConfig(2).Policy,
	}
}

func cleanOutcomeQualitySourceAtHead(t *testing.T, head string) string {
	t.Helper()
	root := outcomeQualityModuleRoot(t)
	clone := filepath.Join(t.TempDir(), "source")
	command := exec.Command("git", "-c", "core.autocrlf=false", "-c", "core.filemode=false", "clone", "--quiet", "--no-hardlinks", root, clone)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("clone source: %v\n%s", err, output)
	}
	command = exec.Command("git", "-C", clone, "checkout", "--quiet", "--detach", head)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("checkout source %s: %v\n%s", head, err, output)
	}
	t.Cleanup(func() { _ = os.RemoveAll(clone) })
	return clone
}

type outcomeQualityLegacyProvider struct {
	server    *httptest.Server
	mu        sync.Mutex
	requests  []legacyProviderRequest
	serverErr string
}

type legacyProviderRequest struct {
	Model    string                  `json:"model"`
	Messages []legacyProviderMessage `json:"messages"`
}

type legacyProviderMessage struct {
	Role      string `json:"role"`
	Content   any    `json:"content"`
	ToolCalls []struct {
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

func newOutcomeQualityLegacyProvider(t *testing.T) *outcomeQualityLegacyProvider {
	t.Helper()
	provider := &outcomeQualityLegacyProvider{}
	provider.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		var request legacyProviderRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		provider.mu.Lock()
		provider.requests = append(provider.requests, request)
		callNumber := len(provider.requests)
		provider.mu.Unlock()

		scenario := DefaultOutcomeQualityScenarios()[0]
		for _, message := range request.Messages {
			content, _ := message.Content.(string)
			for _, candidate := range DefaultOutcomeQualityScenarios() {
				if strings.Contains(content, candidate.ID) {
					scenario = candidate
					break
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch callNumber {
		case 1:
			calls := make([]legacyProviderToolCall, 0, 3)
			for index, path := range []string{"fixture.txt", "fixture_test.go", "go.mod"} {
				calls = append(calls, legacyProviderToolCall{ID: fmt.Sprintf("read-%d", index+1), Name: "read_file", Arguments: fmt.Sprintf(`{"path":%q}`, path)})
			}
			writeLegacyProviderResponse(w, calls, "")
		case 2:
			writeLegacyProviderResponse(w, []legacyProviderToolCall{{ID: "write-1", Name: "write_file", Arguments: fmt.Sprintf(`{"path":"fixture.txt","content":%q}`, fmt.Sprintf("after %s seed=%d\n", scenario.ID, scenario.Seed))}}, "")
		case 3:
			writeLegacyProviderResponse(w, []legacyProviderToolCall{{ID: "verify-1", Name: "verify", Arguments: `{"targets":["fixture.txt"]}`}}, "")
		case 4:
			writeLegacyProviderResponse(w, nil, "Goal complete: the deterministic fixture is complete and verified.")
		default:
			http.Error(w, "too many model calls", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(provider.server.Close)
	return provider
}

type legacyProviderToolCall struct {
	ID        string
	Name      string
	Arguments string
}

func writeLegacyProviderResponse(w http.ResponseWriter, calls []legacyProviderToolCall, content string) {
	toolCalls := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		toolCalls = append(toolCalls, map[string]any{
			"id":   call.ID,
			"type": "function",
			"function": map[string]any{
				"name":      call.Name,
				"arguments": call.Arguments,
			},
		})
	}
	message := map[string]any{"role": "assistant", "content": content}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": message}},
		"usage":   map[string]any{"prompt_tokens": 12, "completion_tokens": 8},
	})
}

func (p *outcomeQualityLegacyProvider) requestCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

func containsOutcomeQualityReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, want) {
			return true
		}
	}
	return false
}

var _ OutcomeQualitySourceExecutor = (*OutcomeQualityLegacyProcessExecutor)(nil)
