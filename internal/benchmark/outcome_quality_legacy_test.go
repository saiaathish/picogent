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
	if execution.Metrics.ChangedLines != 1 || execution.Metrics.UnnecessaryChanges != 0 || execution.Metrics.ToolCalls != 5 {
		t.Fatalf("legacy filesystem metrics=%#v, want one changed line, no extras, five tools", execution.Metrics)
	}
	for _, want := range []string{
		"legacy v3 does not expose provider token usage or model-call counts",
		"legacy v3 does not expose structured repair counts",
		"legacy v3 does not expose context-growth measurement",
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
