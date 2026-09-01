//go:build rendered_fixture

package gui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"
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
	renderedRecoveryFixtureSession = "rendered-recovery-fixture"
	renderedRecoveryFixturePath    = "rendered-recovery-probe.txt"
	renderedRecoveryFixtureContent = "rendered recovery fixture\n"
)

// renderedRecoveryFixtureManifest is intentionally small and machine-readable
// so a browser run can be replayed without copying values from terminal logs.
// It is evidence plumbing only; the normal GUI binary never emits it.
type renderedRecoveryFixtureManifest struct {
	Issue        string `json:"issue"`
	ParentIssue  string `json:"parent_issue"`
	Phase        string `json:"phase"`
	URL          string `json:"url"`
	Home         string `json:"home"`
	Workspace    string `json:"workspace"`
	SessionID    string `json:"session_id"`
	ProbePath    string `json:"probe_path"`
	Before       string `json:"before"`
	AppliedSHA   string `json:"applied_sha256"`
	Source       string `json:"source_sha"`
	SourceOK     bool   `json:"source_sha_verified"`
	SourceDirty  bool   `json:"source_tree_modified"`
	Runtime      string `json:"runtime"`
	StartedAtUTC string `json:"started_at_utc"`
	PID          int    `json:"pid"`
}

// RunRenderedRecoveryFixture serves the real embedded GUI against a bounded,
// deterministic provider. Build and run it only with -tags rendered_fixture.
// The seed phase creates one contained file mutation after a rendered
// permission decision; the reload phase starts a fresh process against the
// same durable home/workspace/session so the browser can verify recovery.
func RunRenderedRecoveryFixture(ctx context.Context) error {
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

	home, workspace, err := renderedRecoveryFixturePaths(phase)
	if err != nil {
		return err
	}
	if err := os.Setenv("PICOGENT_HOME", home); err != nil {
		return fmt.Errorf("set fixture home: %w", err)
	}
	if os.Getenv("PICOGENT_NO_BROWSER") == "" {
		_ = os.Setenv("PICOGENT_NO_BROWSER", "1")
	}

	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Model = "rendered-recovery-fixture"
	cfg.Mode = config.ModeSafe
	cfg.SetupComplete = true
	cfg.Workspace = workspace

	store := taskstate.NewStore(filepath.Join(home, "tasks", projects.IDForPath(workspace)))
	client := renderedRecoveryFixtureClient(phase)
	ag := agent.New(cfg, client, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeSafe, workspace, nil))
	ag.SetTaskStore(store)
	if err := ag.SetTaskSession(renderedRecoveryFixtureSession); err != nil {
		return fmt.Errorf("load fixture task session: %w", err)
	}
	defer ag.Close()

	var hist []llm.Message
	if phase == "reload" {
		loaded, loadErr := session.Load(renderedRecoveryFixtureSession)
		if loadErr != nil {
			return fmt.Errorf("load fixture transcript: %w", loadErr)
		}
		hist = loaded.Messages
	}
	s := &server{
		cfg:       cfg,
		ag:        ag,
		hist:      hist,
		sessionID: renderedRecoveryFixtureSession,
		permCh:    make(chan perm.Decision, 1),
	}
	s.ensureProject()

	addr := strings.TrimSpace(os.Getenv("PICOGENT_RENDERED_FIXTURE_ADDR"))
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	if !loopbackListenAddress(addr) {
		return fmt.Errorf("PICOGENT_RENDERED_FIXTURE_ADDR must bind to loopback, got %q", addr)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen for rendered fixture: %w", err)
	}
	defer ln.Close()

	manifestPath := strings.TrimSpace(os.Getenv("PICOGENT_RENDERED_FIXTURE_MANIFEST"))
	if manifestPath == "" {
		manifestPath = filepath.Join(home, "rendered-recovery-fixture-"+phase+".json")
	}
	manifestPath, err = renderedRecoveryFixtureManifestPath(home, manifestPath)
	if err != nil {
		return err
	}
	sourceSHA, sourceOK, sourceDirty, err := renderedRecoveryFixtureSource()
	if err != nil {
		return err
	}
	manifest := renderedRecoveryFixtureManifest{
		Issue:        "291",
		ParentIssue:  "246",
		Phase:        phase,
		URL:          "http://" + ln.Addr().String() + "/",
		Home:         home,
		Workspace:    workspace,
		SessionID:    renderedRecoveryFixtureSession,
		ProbePath:    renderedRecoveryFixturePath,
		Before:       "absent",
		AppliedSHA:   renderedRecoveryFixtureContentSHA(),
		Source:       sourceSHA,
		SourceOK:     sourceOK,
		SourceDirty:  sourceDirty,
		Runtime:      "go-build-tags-rendered_fixture",
		StartedAtUTC: time.Now().UTC().Format(time.RFC3339Nano),
		PID:          os.Getpid(),
	}
	if err := writeRenderedRecoveryFixtureManifest(manifestPath, manifest); err != nil {
		return err
	}
	fmt.Printf("picogent rendered recovery fixture phase=%s url=%s\n", phase, manifest.URL)
	fmt.Printf("picogent rendered recovery fixture manifest=%s\n", manifestPath)
	fmt.Printf("picogent rendered recovery fixture workspace=%s session=%s\n", workspace, renderedRecoveryFixtureSession)

	return serveContext(ctx, ln, s.Handler(), s.stopForShutdown)
}

func renderedRecoveryFixturePaths(phase string) (string, string, error) {
	home := strings.TrimSpace(os.Getenv("PICOGENT_RENDERED_FIXTURE_HOME"))
	workspace := strings.TrimSpace(os.Getenv("PICOGENT_RENDERED_FIXTURE_WORKSPACE"))
	if phase == "seed" {
		if home == "" {
			var err error
			home, err = os.MkdirTemp("", "picogent-rendered-recovery-home-")
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
		if _, err := os.Lstat(filepath.Join(workspace, renderedRecoveryFixturePath)); err == nil {
			return "", "", errors.New("fixture seed workspace already contains the recovery probe; use a fresh workspace")
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
	if _, err := os.Lstat(filepath.Join(workspace, renderedRecoveryFixturePath)); err == nil {
		return "", "", errors.New("fixture reload requires the recovery probe to be absent after undo")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("inspect fixture probe before reload: %w", err)
	}
	taskPath := filepath.Join(home, "tasks", projects.IDForPath(workspace), renderedRecoveryFixtureSession+".json")
	if _, err := os.Stat(taskPath); err != nil {
		return "", "", fmt.Errorf("reload requires durable fixture task %q: %w", taskPath, err)
	}
	return filepath.Clean(home), filepath.Clean(workspace), nil
}

func renderedRecoveryFixtureTempPath(raw, label string) (string, error) {
	path, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	tempRoot, err := renderedRecoveryFixtureResolvedPath(os.TempDir())
	if err != nil {
		return "", fmt.Errorf("resolve fixture temp root: %w", err)
	}
	resolved, err := renderedRecoveryFixtureResolvedPath(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	if resolved == tempRoot || !renderedRecoveryFixtureIsWithin(tempRoot, resolved) {
		return "", fmt.Errorf("%s must be a disposable path below %q", label, tempRoot)
	}
	return path, nil
}

func renderedRecoveryFixtureContainedPath(root, raw, label string) (string, error) {
	path, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	if err := renderedRecoveryFixtureContainedPathCheck(root, path, label); err != nil {
		return "", err
	}
	return path, nil
}

func renderedRecoveryFixtureContainedPathCheck(root, candidate, label string) error {
	rootResolved, err := renderedRecoveryFixtureResolvedPath(root)
	if err != nil {
		return fmt.Errorf("resolve fixture home for %s: %w", label, err)
	}
	candidateResolved, err := renderedRecoveryFixtureResolvedPath(candidate)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", label, err)
	}
	if candidateResolved == rootResolved || !renderedRecoveryFixtureIsWithin(rootResolved, candidateResolved) {
		return fmt.Errorf("%s must be contained by fixture home %q", label, rootResolved)
	}
	return nil
}

func renderedRecoveryFixtureIsWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "."
}

func renderedRecoveryFixtureResolvedPath(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	current := abs
	var suffix []string
	for {
		_, statErr := os.Lstat(current)
		if statErr == nil {
			resolved, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil {
				return "", evalErr
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func renderedRecoveryFixtureEmptyDirectory(path, label string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", label, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s must be a real directory", label)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("%s must be empty; use a fresh disposable path", label)
	}
	return nil
}

func renderedRecoveryFixtureDirectoryExists(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s must be a real directory", label)
	}
	return nil
}

func renderedRecoveryFixtureManifestPath(home, raw string) (string, error) {
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(home, path)
	}
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve fixture manifest: %w", err)
	}
	if err := renderedRecoveryFixtureContainedPathCheck(home, path, "fixture manifest"); err != nil {
		return "", err
	}
	if _, err := os.Lstat(path); err == nil {
		return "", fmt.Errorf("fixture manifest %q already exists; use a fresh path", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect fixture manifest: %w", err)
	}
	return path, nil
}

type renderedRecoveryFixtureProvider struct {
	mu       sync.Mutex
	scripted *llm.Scripted
}

func (p *renderedRecoveryFixtureProvider) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	// The real GUI asks the provider for optional hero recommendations with a
	// read-only `ask` request. Keep that auxiliary request deterministic without
	// consuming the two responses reserved for the recovery turn.
	if req.ReadOnly && req.TaskMode == "ask" {
		return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "[]"}}, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.scripted.Chat(ctx, req)
}

func renderedRecoveryFixtureClient(phase string) llm.Client {
	if phase == "reload" {
		return &renderedRecoveryFixtureProvider{scripted: &llm.Scripted{}}
	}
	args, _ := json.Marshal(map[string]string{
		"path":    renderedRecoveryFixturePath,
		"content": renderedRecoveryFixtureContent,
	})
	return &renderedRecoveryFixtureProvider{scripted: &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:        "rendered-recovery-write",
				Name:      "write_file",
				Arguments: string(args),
			}},
		}},
		{Message: llm.Message{
			Role:    "assistant",
			Content: "The fixture mutation is complete. The contained recovery probe can now be undone.",
		}},
	}}}
}

func renderedRecoveryFixtureContentSHA() string {
	sum := sha256.Sum256([]byte(renderedRecoveryFixtureContent))
	return hex.EncodeToString(sum[:])
}

func renderedRecoveryFixtureSource() (string, bool, bool, error) {
	supplied := strings.TrimSpace(os.Getenv("PICOGENT_RENDERED_FIXTURE_SOURCE_SHA"))
	buildSHA, buildModified := renderedRecoveryFixtureBuildSource()
	if supplied != "" {
		normalized, ok := renderedRecoveryFixtureNormalizeSHA(supplied)
		if !ok {
			return "", false, buildModified, errors.New("PICOGENT_RENDERED_FIXTURE_SOURCE_SHA must be a full 40-character Git SHA")
		}
		supplied = normalized
		if buildSHA != "" && supplied != buildSHA {
			return "", false, buildModified, fmt.Errorf("fixture source SHA %q does not match compiled Git revision %q", supplied, buildSHA)
		}
	}
	if supplied == "" {
		supplied = buildSHA
	}
	if supplied == "" {
		return "UNRECORDED", false, buildModified, nil
	}
	return supplied, buildSHA != "" && supplied == buildSHA && !buildModified, buildModified, nil
}

func renderedRecoveryFixtureBuildSource() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision, _ = renderedRecoveryFixtureNormalizeSHA(setting.Value)
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return revision, modified
}

func renderedRecoveryFixtureNormalizeSHA(raw string) (string, bool) {
	if len(raw) != 40 {
		return "", false
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return "", false
	}
	return strings.ToLower(raw), true
}

func writeRenderedRecoveryFixtureManifest(path string, manifest renderedRecoveryFixtureManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode rendered fixture manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create rendered fixture manifest directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create rendered fixture manifest: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write rendered fixture manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close rendered fixture manifest: %w", err)
	}
	return nil
}
