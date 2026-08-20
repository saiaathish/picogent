package setup

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/saiaathish/picogent/internal/agyauth"
	"github.com/saiaathish/picogent/internal/claudeauth"
	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/opencodeauth"
)

type Component struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail"`
	CanFix  bool   `json:"can_fix"`
	FixHint string `json:"fix_hint,omitempty"`
}

type LoginTarget struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	Detail    string `json:"detail"`
	Button    string `json:"button"`
	NeedOAuth bool   `json:"need_oauth"`
}

type Status struct {
	Ready      bool          `json:"ready"`
	Busy       bool          `json:"busy"`
	Log        string        `json:"log"`
	Components []Component   `json:"components"`
	Logins     []LoginTarget `json:"logins"`
	Workspace  string        `json:"workspace"`
	Mode       string        `json:"mode"`
	Model        string            `json:"model"`
	Provider     string            `json:"provider"`
	ModelOptions []llm.ModelChoice `json:"model_options"`
	LoggedIn     bool              `json:"logged_in"`
	SetupDone  bool          `json:"setup_done"`
}

var (
	mu      sync.Mutex
	busy    bool
	lastLog string
)

func Busy() bool {
	mu.Lock()
	defer mu.Unlock()
	return busy
}

func LastLog() string {
	mu.Lock()
	defer mu.Unlock()
	return lastLog
}

func Snapshot(cfg config.Config) Status {
	llm.RefreshCLIModels(false)
	comps := []Component{homeComponent(), gitComponent(), codexCLIComponent(), claudeCLIComponent(), openCodeCLIComponent(), agyCLIComponent()}
	logins := []LoginTarget{codexLogin(), claudeLogin(), openCodeLogin(), antigravityLogin()}
	ready := true
	for _, c := range comps {
		if !c.OK && (c.ID == "home" || c.ID == "git") {
			ready = false
		}
	}
	if !codexauth.LoggedIn() {
		ready = false
	}
	ws := cfg.Workspace
	if ws == "" || ws == "." {
		ws, _ = os.Getwd()
	}
	return Status{
		Ready:        ready,
		Busy:         Busy(),
		Log:          LastLog(),
		Components:   comps,
		Logins:       logins,
		Workspace:    ws,
		Mode:         string(cfg.Mode),
		Model:        cfg.DisplayModel(),
		Provider:     string(cfg.Provider),
		ModelOptions: llm.ModelChoices(llm.EcoCodex, false),
		LoggedIn:     codexauth.LoggedIn(),
		SetupDone:    cfg.SetupComplete,
	}
}

func homeComponent() Component {
	dir, err := config.Dir()
	if err != nil {
		return Component{ID: "home", Name: "Picogent folder", Detail: err.Error(), CanFix: true}
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return Component{ID: "home", Name: "Picogent folder", OK: true, Detail: dir}
	}
	return Component{ID: "home", Name: "Picogent folder", Detail: "missing " + dir, CanFix: true, FixHint: "Create ~/.picogent"}
}

func gitComponent() Component {
	p, err := exec.LookPath("git")
	if err != nil {
		return Component{ID: "git", Name: "Git", Detail: "not on PATH", CanFix: false, FixHint: "Install Git, then click Install again."}
	}
	return Component{ID: "git", Name: "Git", OK: true, Detail: p}
}

func codexCLIComponent() Component {
	if p := look("codex"); p != "" {
		return Component{ID: "codex-cli", Name: "Codex CLI", OK: true, Detail: p}
	}
	return Component{ID: "codex-cli", Name: "Codex CLI", CanFix: true, Detail: "not installed", FixHint: "Will run npm/brew for you."}
}

func claudeCLIComponent() Component {
	if p := look("claude"); p != "" {
		return Component{ID: "claude-cli", Name: "Claude Code CLI", OK: true, Detail: p}
	}
	return Component{ID: "claude-cli", Name: "Claude Code CLI", CanFix: true, Detail: "not installed", FixHint: "Will run npm for you."}
}

func openCodeCLIComponent() Component {
	if p := look("opencode"); p != "" {
		return Component{ID: "opencode-cli", Name: "OpenCode CLI", OK: true, Detail: p}
	}
	return Component{ID: "opencode-cli", Name: "OpenCode CLI", CanFix: true, Detail: "not installed", FixHint: "Will install OpenCode for you."}
}

func agyCLIComponent() Component {
	if p := look("agy"); p != "" {
		return Component{ID: "agy-cli", Name: "Antigravity CLI", OK: true, Detail: p}
	}
	return Component{ID: "agy-cli", Name: "Antigravity CLI", CanFix: true, Detail: "not installed", FixHint: "Will run the official install script."}
}

func codexLogin() LoginTarget {
	if codexauth.LoggedIn() {
		return LoginTarget{ID: "codex", Name: "ChatGPT Codex", OK: true, Detail: "connected via ~/.codex/auth.json", Button: "Connected"}
	}
	return LoginTarget{ID: "codex", Name: "ChatGPT Codex", Detail: "needed to run Picogent", Button: "Log in to ChatGPT Codex", NeedOAuth: true}
}

func claudeLogin() LoginTarget {
	if ClaudeLoggedIn() {
		return LoginTarget{ID: "claude", Name: "Claude Code", OK: true, Detail: "connected via Claude Code CLI", Button: "Connected"}
	}
	okCLI := look("claude") != ""
	return LoginTarget{ID: "claude", Name: "Claude Code", Detail: "uses your Claude subscription (same as `claude` CLI)", Button: "Log in to Claude", NeedOAuth: okCLI}
}

func openCodeLogin() LoginTarget {
	if opencodeauth.LoggedIn() {
		detail := "connected"
		if opencodeauth.ZenLoggedIn() && opencodeauth.GoLoggedIn() {
			detail = "Zen + Go connected"
		} else if opencodeauth.GoLoggedIn() {
			detail = "Go connected via OpenCode CLI"
		} else {
			detail = "Zen connected via OpenCode CLI"
		}
		return LoginTarget{ID: "opencode", Name: "OpenCode (Zen/Go)", OK: true, Detail: detail, Button: "Connected"}
	}
	okCLI := look("opencode") != ""
	return LoginTarget{ID: "opencode", Name: "OpenCode (Zen/Go)", Detail: "uses OpenCode Zen or Go API key (`opencode auth login`)", Button: "Log in to OpenCode", NeedOAuth: okCLI}
}

func antigravityLogin() LoginTarget {
	if agyauth.LoggedIn() {
		detail := "connected via Antigravity CLI"
		if agyauth.GeminiAPIKey() != "" {
			detail = "connected via GEMINI_API_KEY"
		}
		return LoginTarget{ID: "antigravity", Name: "Antigravity", OK: true, Detail: detail, Button: "Connected"}
	}
	okCLI := look("agy") != ""
	return LoginTarget{ID: "antigravity", Name: "Antigravity", Detail: "uses Google Antigravity CLI (`agy`) or GEMINI_API_KEY", Button: "Log in to Antigravity", NeedOAuth: okCLI}
}

func ClaudeLoggedIn() bool {
	return claudeauth.LoggedIn()
}

func look(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

func InstallCores() (string, error) {
	mu.Lock()
	if busy {
		mu.Unlock()
		return "", fmt.Errorf("setup is already running")
	}
	busy = true
	mu.Unlock()
	defer func() {
		mu.Lock()
		busy = false
		mu.Unlock()
	}()

	var log bytes.Buffer
	say := func(s string) {
		log.WriteString(s)
		if !strings.HasSuffix(s, "\n") {
			log.WriteByte('\n')
		}
		mu.Lock()
		lastLog = log.String()
		mu.Unlock()
	}

	dir, err := config.Dir()
	if err != nil {
		return log.String(), err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return log.String(), err
	}
	say("ok  picogent home  " + dir)

	path, err := config.Path()
	if err != nil {
		return log.String(), err
	}
	if _, err := os.Stat(path); err != nil {
		cfg := config.Default()
		if wd, e := os.Getwd(); e == nil {
			cfg.Workspace = wd
		}
		if err := config.Save(cfg); err != nil {
			return log.String(), err
		}
		say("ok  wrote " + path)
	} else {
		say("ok  config already exists")
	}

	if look("git") == "" {
		say("miss git  — install Git first (on a Mac: Xcode Command Line Tools)")
	} else {
		say("ok  git")
	}

	if look("codex") == "" {
		if os.Getenv("PICOGENT_SETUP_SKIP_CLIS") != "" {
			say("skip  codex cli (test)")
		} else if err := installNPM("Codex CLI", "@openai/codex", "codex", say); err != nil {
			if brew := look("brew"); brew != "" {
				say("trying Homebrew for Codex…")
				out, e := runTimed(4*time.Minute, brew, "install", "codex")
				if e != nil {
					say("miss codex  " + e.Error() + "\n" + strings.TrimSpace(out))
				} else if look("codex") != "" {
					say("ok  codex cli")
				} else {
					say("miss codex  still not on PATH")
				}
			} else {
				say("miss codex  " + err.Error())
			}
		}
	} else {
		say("ok  codex cli")
	}

	if look("claude") == "" {
		if os.Getenv("PICOGENT_SETUP_SKIP_CLIS") != "" {
			say("skip  claude cli (test)")
		} else if err := installNPM("Claude Code CLI", "@anthropic-ai/claude-code", "claude", say); err != nil {
			say("miss claude  " + err.Error())
		}
	} else {
		say("ok  claude cli")
	}

	if look("opencode") == "" {
		if os.Getenv("PICOGENT_SETUP_SKIP_CLIS") != "" {
			say("skip  opencode cli (test)")
		} else if err := installOpenCode(say); err != nil {
			say("miss opencode  " + err.Error())
		}
	} else {
		say("ok  opencode cli")
	}

	if look("agy") == "" {
		if os.Getenv("PICOGENT_SETUP_SKIP_CLIS") != "" {
			say("skip  agy cli (test)")
		} else if err := installAgy(say); err != nil {
			say("miss agy  " + err.Error())
		}
	} else {
		say("ok  agy cli")
	}

	if codexauth.LoggedIn() {
		say("ok  chatgpt codex login")
	} else {
		say("next  tap Log in to ChatGPT Codex")
	}

	mu.Lock()
	lastLog = log.String()
	mu.Unlock()
	return log.String(), nil
}

func installNPM(label, pkg, bin string, say func(string)) error {
	npm := look("npm")
	if npm == "" {
		return fmt.Errorf("npm is not installed, cannot install %s", label)
	}
	say("installing " + label + "  (npm i -g " + pkg + ")")
	out, err := runTimed(4*time.Minute, npm, "install", "-g", pkg)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(out))
	}
	if look(bin) == "" {
		return fmt.Errorf("npm finished but `%s` is still not on PATH", bin)
	}
	say("ok  " + bin)
	return nil
}

func runTimed(d time.Duration, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return buf.String(), err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return buf.String(), err
	case <-time.After(d):
		_ = cmd.Process.Kill()
		return buf.String(), fmt.Errorf("timed out")
	}
}

func StartClaudeLogin() error {
	if ClaudeLoggedIn() {
		return nil
	}
	if look("claude") == "" {
		say := func(string) {}
		if err := installNPM("Claude Code CLI", "@anthropic-ai/claude-code", "claude", say); err != nil {
			return fmt.Errorf("Claude Code is not installed and auto-install failed: %w", err)
		}
	}
	bin := look("claude")
	if bin == "" {
		return fmt.Errorf("Claude Code is not installed yet")
	}
	return OpenInteractive(bin + " auth login")
}

func StartOpenCodeLogin() error {
	if opencodeauth.LoggedIn() {
		return nil
	}
	if look("opencode") == "" {
		say := func(string) {}
		if err := installOpenCode(say); err != nil {
			return fmt.Errorf("OpenCode is not installed and auto-install failed: %w", err)
		}
	}
	bin := look("opencode")
	if bin == "" {
		home, _ := os.UserHomeDir()
		cand := filepath.Join(home, ".opencode", "bin", "opencode")
		if st, e := os.Stat(cand); e == nil && !st.IsDir() {
			bin = cand
		}
	}
	if bin == "" {
		return fmt.Errorf("OpenCode is not installed yet")
	}
	return OpenInteractive(shellQuote(bin) + " auth login")
}

func StartAntigravityLogin() error {
	if agyauth.LoggedIn() {
		return nil
	}
	if look("agy") == "" {
		say := func(string) {}
		if err := installAgy(say); err != nil {
			return fmt.Errorf("Antigravity CLI is not installed and auto-install failed: %w", err)
		}
	}
	bin := look("agy")
	if bin == "" {
		home, _ := os.UserHomeDir()
		cand := filepath.Join(home, ".local", "bin", "agy")
		if st, e := os.Stat(cand); e == nil && !st.IsDir() {
			bin = cand
		}
	}
	if bin == "" {
		return fmt.Errorf("Antigravity CLI (`agy`) is not installed yet")
	}
	return OpenInteractive(shellQuote(bin))
}

// StartCodexCLILogin opens Codex CLI login in a terminal (fallback when browser OAuth isn't used).
func StartCodexCLILogin() error {
	if codexauth.LoggedIn() {
		return nil
	}
	if look("codex") == "" {
		say := func(string) {}
		if err := installNPM("Codex CLI", "@openai/codex", "codex", say); err != nil {
			return fmt.Errorf("Codex CLI is not installed and auto-install failed: %w", err)
		}
	}
	bin := look("codex")
	if bin == "" {
		return fmt.Errorf("Codex CLI is not installed yet")
	}
	return OpenInteractive(shellQuote(bin) + " login")
}

func installOpenCode(say func(string)) error {
	say("installing OpenCode CLI…")
	// Official installer: https://opencode.ai/docs
	script := `curl -fsSL https://opencode.ai/install | bash`
	out, err := runTimed(4*time.Minute, "bash", "-lc", script)
	if err != nil {
		if e2 := installNPM("OpenCode CLI", "opencode-ai", "opencode", say); e2 == nil {
			return nil
		}
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(out))
	}
	if look("opencode") == "" {
		// Common install location
		home, _ := os.UserHomeDir()
		cand := filepath.Join(home, ".opencode", "bin", "opencode")
		if st, e := os.Stat(cand); e == nil && !st.IsDir() {
			say("ok  opencode at " + cand + " (add ~/.opencode/bin to PATH)")
			return nil
		}
		return fmt.Errorf("install finished but `opencode` is still not on PATH")
	}
	say("ok  opencode")
	return nil
}

func installAgy(say func(string)) error {
	say("installing Antigravity CLI (`agy`)…")
	out, err := runTimed(4*time.Minute, "bash", "-lc", `curl -fsSL https://antigravity.google/cli/install.sh | bash`)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(out))
	}
	if look("agy") == "" {
		home, _ := os.UserHomeDir()
		cand := filepath.Join(home, ".local", "bin", "agy")
		if st, e := os.Stat(cand); e == nil && !st.IsDir() {
			say("ok  agy at " + cand)
			return nil
		}
		return fmt.Errorf("install finished but `agy` is still not on PATH")
	}
	say("ok  agy")
	return nil
}

func Apply(cfg config.Config, workspace, mode, model string) (config.Config, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return cfg, fmt.Errorf("pick a folder")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return cfg, err
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return cfg, fmt.Errorf("folder does not exist: %s", abs)
	}
	m := config.Mode(mode)
	if !m.Valid() {
		m = config.ModeSafe
	}
	cfg.Workspace = abs
	cfg.Mode = m
	prov := cfg.Provider
	if prov == "" {
		prov = config.ProviderCodex
	}
	supportsAuto := prov == config.ProviderCodex || prov == config.ProviderQuadCode
	if strings.TrimSpace(model) != "" && model != config.ModelAuto {
		cfg.Model = strings.TrimSpace(model)
		cfg.Router.Enabled = false
	} else if supportsAuto {
		cfg.Model = config.ModelAuto
		cfg.Router.Enabled = true
	} else {
		cfg.Model = ""
		cfg.Model = cfg.BackendModel()
		cfg.Router.Enabled = false
	}
	if cfg.Provider == "" {
		cfg.Provider = config.ProviderCodex
	}
	cfg.SetupComplete = true
	if err := config.Save(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
