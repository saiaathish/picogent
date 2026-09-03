//go:build rendered_fixture

package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

const (
	renderedLongHorizonFixtureIssue   = "368"
	renderedLongHorizonFixtureSession = "rendered-long-horizon-fixture"
	renderedLongHorizonFixturePath    = "rendered-long-horizon-probe.txt"
	renderedLongHorizonFixtureContent = "rendered long-horizon mutation\n"
)

// renderedLongHorizonFixtureManifest is startup metadata for a browser run.
// Per-turn observations are recorded by the task-owned integration harness or
// the later direct-browser evidence lane; this manifest never claims either.
type renderedLongHorizonFixtureManifest struct {
	Issue        string   `json:"issue"`
	ParentIssue  string   `json:"parent_issue"`
	BroaderIssue string   `json:"broader_issue"`
	Phase        string   `json:"phase"`
	Scenario     string   `json:"scenario"`
	URL          string   `json:"url"`
	Home         string   `json:"home"`
	Workspace    string   `json:"workspace"`
	SessionID    string   `json:"session_id"`
	ProbePath    string   `json:"probe_path"`
	Source       string   `json:"source_sha"`
	SourceOK     bool     `json:"source_sha_verified"`
	SourceDirty  bool     `json:"source_tree_modified"`
	Runtime      string   `json:"runtime"`
	ExpectedFlow []string `json:"expected_flow"`
	StartedAtUTC string   `json:"started_at_utc"`
	PID          int      `json:"pid"`
}

// RunRenderedLongHorizonFixture serves the real embedded GUI against a
// bounded, deterministic provider. Build and run it only with
// -tags rendered_fixture. The seed phase exposes the normal GUI boundaries
// for mutation, verification, and steering; reload starts a fresh agent and
// rebinds the same durable task and transcript.
func RunRenderedLongHorizonFixture(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	phase := strings.TrimSpace(os.Getenv("PICOGENT_RENDERED_FIXTURE_PHASE"))
	if phase == "" {
		phase = "seed"
	}
	if phase != "seed" && phase != "reload" {
		return fmt.Errorf("PICOGENT_RENDERED_FIXTURE_PHASE must be seed or reload, got %q", phase)
	}

	home, workspace, err := renderedLongHorizonFixturePaths(phase)
	if err != nil {
		return err
	}
	if err := os.Setenv("PICOGENT_HOME", home); err != nil {
		return fmt.Errorf("set fixture home: %w", err)
	}
	if os.Getenv("PICOGENT_NO_BROWSER") == "" {
		_ = os.Setenv("PICOGENT_NO_BROWSER", "1")
	}

	s, ag, _, err := newRenderedLongHorizonFixtureServer(phase, home, workspace)
	if err != nil {
		return err
	}
	defer ag.Close()

	addr := strings.TrimSpace(os.Getenv("PICOGENT_RENDERED_FIXTURE_ADDR"))
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	if !loopbackListenAddress(addr) {
		return fmt.Errorf("PICOGENT_RENDERED_FIXTURE_ADDR must bind to loopback, got %q", addr)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen for rendered long-horizon fixture: %w", err)
	}
	defer ln.Close()

	manifestPath := strings.TrimSpace(os.Getenv("PICOGENT_RENDERED_FIXTURE_MANIFEST"))
	if manifestPath == "" {
		manifestPath = filepath.Join(home, "rendered-long-horizon-fixture-"+phase+".json")
	}
	manifestPath, err = renderedRecoveryFixtureManifestPath(home, manifestPath)
	if err != nil {
		return err
	}
	sourceSHA, sourceOK, sourceDirty, err := renderedRecoveryFixtureSource()
	if err != nil {
		return err
	}
	manifest := renderedLongHorizonFixtureManifest{
		Issue:        renderedLongHorizonFixtureIssue,
		ParentIssue:  "366",
		BroaderIssue: "246",
		Phase:        phase,
		Scenario:     "rendered-multi-turn-outcome",
		URL:          "http://" + ln.Addr().String() + "/",
		Home:         home,
		Workspace:    workspace,
		SessionID:    renderedLongHorizonFixtureSession,
		ProbePath:    renderedLongHorizonFixturePath,
		Source:       sourceSHA,
		SourceOK:     sourceOK,
		SourceDirty:  sourceDirty,
		Runtime:      "go-build-tags-rendered_fixture",
		ExpectedFlow: []string{"mutation", "verification", "steering", "reload"},
		StartedAtUTC: time.Now().UTC().Format(time.RFC3339Nano),
		PID:          os.Getpid(),
	}
	if err := writeRenderedLongHorizonFixtureManifest(manifestPath, manifest); err != nil {
		return err
	}
	fmt.Printf("picogent rendered long-horizon fixture phase=%s url=%s\n", phase, manifest.URL)
	fmt.Printf("picogent rendered long-horizon fixture manifest=%s\n", manifestPath)
	fmt.Printf("picogent rendered long-horizon fixture workspace=%s session=%s\n", workspace, renderedLongHorizonFixtureSession)

	return serveContext(ctx, ln, s.Handler(), s.stopForShutdown)
}

func renderedLongHorizonFixturePaths(phase string) (string, string, error) {
	home := strings.TrimSpace(os.Getenv("PICOGENT_RENDERED_FIXTURE_HOME"))
	workspace := strings.TrimSpace(os.Getenv("PICOGENT_RENDERED_FIXTURE_WORKSPACE"))
	if phase == "seed" {
		if home == "" {
			var err error
			home, err = os.MkdirTemp("", "picogent-rendered-long-horizon-home-")
			if err != nil {
				return "", "", fmt.Errorf("create fixture home: %w", err)
			}
		} else {
			var err error
			home, err = renderedRecoveryFixtureTempPath(home, "fixture home")
			if err != nil {
				return "", "", err
			}
			if err := renderedRecoveryFixtureEmptyDirectory(home, "fixture home"); err != nil {
				return "", "", err
			}
		}
		if workspace == "" {
			workspace = filepath.Join(home, "workspace")
		} else {
			var err error
			workspace, err = renderedRecoveryFixtureContainedPath(home, workspace, "fixture workspace")
			if err != nil {
				return "", "", err
			}
		}
		if err := renderedRecoveryFixtureContainedPathCheck(home, workspace, "fixture workspace"); err != nil {
			return "", "", err
		}
		if err := renderedRecoveryFixtureEmptyDirectory(workspace, "fixture workspace"); err != nil {
			return "", "", err
		}
		if _, err := os.Lstat(filepath.Join(workspace, renderedLongHorizonFixturePath)); err == nil {
			return "", "", errors.New("fixture seed workspace already contains the long-horizon probe; use a fresh workspace")
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("inspect fixture probe: %w", err)
		}
		return filepath.Clean(home), filepath.Clean(workspace), nil
	}

	if home == "" || workspace == "" {
		return "", "", errors.New("reload requires both PICOGENT_RENDERED_FIXTURE_HOME and PICOGENT_RENDERED_FIXTURE_WORKSPACE")
	}
	var err error
	home, err = renderedRecoveryFixtureTempPath(home, "fixture home")
	if err != nil {
		return "", "", err
	}
	workspace, err = renderedRecoveryFixtureContainedPath(home, workspace, "fixture workspace")
	if err != nil {
		return "", "", err
	}
	if err := renderedRecoveryFixtureDirectoryExists(home, "fixture home"); err != nil {
		return "", "", err
	}
	if err := renderedRecoveryFixtureDirectoryExists(workspace, "fixture workspace"); err != nil {
		return "", "", err
	}
	taskPath := filepath.Join(home, "tasks", projects.IDForPath(workspace), renderedLongHorizonFixtureSession+".json")
	if _, err := os.Stat(taskPath); err != nil {
		return "", "", fmt.Errorf("reload requires durable long-horizon task %q: %w", taskPath, err)
	}
	return filepath.Clean(home), filepath.Clean(workspace), nil
}

type renderedLongHorizonFixtureRuntime struct {
	phase     string
	workspace string

	mu                sync.Mutex
	verificationCalls int
}

func newRenderedLongHorizonFixtureServer(phase, home, workspace string) (*server, *agent.Agent, *renderedLongHorizonFixtureRuntime, error) {
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Model = "rendered-long-horizon-fixture"
	cfg.Mode = config.ModeSafe
	cfg.SetupComplete = true
	cfg.Workspace = workspace

	runtime := &renderedLongHorizonFixtureRuntime{phase: phase, workspace: workspace}
	store := taskstate.NewStore(filepath.Join(home, "tasks", projects.IDForPath(workspace)))
	reg := tools.NewRegistry(tools.Context{
		Workspace:     workspace,
		VerifyTargets: runtime.verify,
	})
	ag := agent.New(cfg, renderedLongHorizonFixtureClient(phase), reg, perm.New(config.ModeSafe, workspace, nil))
	ag.SetTaskStore(store)
	if err := ag.SetTaskSession(renderedLongHorizonFixtureSession); err != nil {
		ag.Close()
		return nil, nil, nil, fmt.Errorf("load long-horizon fixture task session: %w", err)
	}

	var hist []llm.Message
	if phase == "reload" {
		loaded, err := session.Load(renderedLongHorizonFixtureSession)
		if err != nil {
			ag.Close()
			return nil, nil, nil, fmt.Errorf("load long-horizon fixture transcript: %w", err)
		}
		hist = loaded.Messages
	}
	s := &server{
		cfg:                      cfg,
		ag:                       ag,
		hist:                     hist,
		sessionID:                renderedLongHorizonFixtureSession,
		permCh:                   make(chan perm.Decision, 1),
		suppressExtensionRebuild: true,
	}
	s.ensureProject()
	return s, ag, runtime, nil
}

func renderedLongHorizonFixtureClient(phase string) llm.Client {
	if phase == "reload" {
		return &renderedLongHorizonFixtureProvider{scripted: &llm.Scripted{Responses: []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: "The reloaded task is still fail-closed; fresh rendered inspection is required."}},
		}}}
	}
	args, _ := json.Marshal(map[string]string{
		"path":    renderedLongHorizonFixturePath,
		"content": renderedLongHorizonFixtureContent,
	})
	return &renderedLongHorizonFixtureProvider{scripted: &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{
			ID:        "rendered-long-horizon-write",
			Name:      "write_file",
			Arguments: string(args),
		}}}},
		{Message: llm.Message{Role: "assistant", Content: "The contained mutation is staged; proof is pending."}},
		{Message: llm.Message{Role: "assistant", Content: "The deterministic check ran; rendered inspection is still pending."}},
		{Message: llm.Message{Role: "assistant", Content: "Steering changed the outcome contract; earlier proof is stale."}},
	}}}
}

type renderedLongHorizonFixtureProvider struct {
	mu       sync.Mutex
	scripted *llm.Scripted
}

func (p *renderedLongHorizonFixtureProvider) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	// Prompt recommendations are auxiliary read-only requests. They must not
	// consume one of the scripted responses reserved for the outcome flow.
	if req.ReadOnly && req.TaskMode == "ask" {
		return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "[]"}}, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.scripted.Chat(ctx, req)
}

func (r *renderedLongHorizonFixtureRuntime) verify(ctx context.Context, targets []string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	found := false
	for _, target := range targets {
		if strings.TrimPrefix(strings.TrimSpace(strings.ReplaceAll(target, "\\", "/")), "./") == renderedLongHorizonFixturePath {
			found = true
			break
		}
	}
	if !found {
		return "verify INCONCLUSIVE — the fixture probe was not a tracked verification target", nil
	}
	data, err := os.ReadFile(filepath.Join(r.workspace, renderedLongHorizonFixturePath))
	if err != nil {
		return "verify INCONCLUSIVE — the fixture probe is unavailable", nil
	}
	if string(data) != renderedLongHorizonFixtureContent {
		return "verify INCONCLUSIVE — the fixture probe content is not at the expected boundary", nil
	}

	r.mu.Lock()
	r.verificationCalls++
	call := r.verificationCalls
	phase := r.phase
	r.mu.Unlock()
	if phase == "seed" && call == 2 {
		return "verify PASS — deterministic workspace observation", nil
	}
	if phase == "seed" && call == 1 {
		return "verify INCONCLUSIVE — rendered inspection is pending", nil
	}
	return "verify INCONCLUSIVE — fresh rendered inspection is required", nil
}

func (r *renderedLongHorizonFixtureRuntime) verificationCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.verificationCalls
}

func writeRenderedLongHorizonFixtureManifest(path string, manifest renderedLongHorizonFixtureManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode rendered long-horizon fixture manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create rendered long-horizon fixture manifest directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create rendered long-horizon fixture manifest: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write rendered long-horizon fixture manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close rendered long-horizon fixture manifest: %w", err)
	}
	return nil
}
