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
	Ready          bool              `json:"ready"`
	Busy           bool              `json:"busy"`
	Log            string            `json:"log"`
	Components     []Component       `json:"components"`
	Logins         []LoginTarget     `json:"logins"`
	Workspace      string            `json:"workspace"`
	Mode           string            `json:"mode"`
	ActiveMode     string            `json:"active_mode"`
	ModeOverridden bool              `json:"mode_overridden"`
	Model          string            `json:"model"`
	Provider       string            `json:"provider"`
	ModelOptions   []llm.ModelChoice `json:"model_options"`
	LoggedIn       bool              `json:"logged_in"`
	SetupDone      bool              `json:"setup_done"`
}

var (
	mu           sync.Mutex
	busy         bool
	lastLog      string
	lastProvider config.Provider
)

func Busy() bool {
	mu.Lock()
	defer mu.Unlock()
	return busy
}

func LastLog(provider config.Provider) string {
	mu.Lock()
	defer mu.Unlock()
	if provider != lastProvider {
		return ""
	}
	return lastLog
}

func Provider(value string) (config.Provider, error) {
	switch config.Provider(strings.ToLower(strings.TrimSpace(value))) {
	case config.ProviderCodex, config.ProviderQuadCode, config.ProviderOpenCode, config.ProviderAntigravity, config.ProviderOpenAI:
		return config.Provider(strings.ToLower(strings.TrimSpace(value))), nil
	default:
		return "", fmt.Errorf("choose Codex, Claude Code, OpenCode, Antigravity, or OpenAI API")
	}
}

func Snapshot(cfg config.Config, provider config.Provider) Status {
	comps := []Component{homeComponent(), gitComponent()}
	var login LoginTarget
	switch provider {
	case config.ProviderQuadCode:
		comps = append(comps, claudeCLIComponent())
		login = claudeLogin()
	case config.ProviderOpenCode:
		comps = append(comps, openCodeCLIComponent())
		login = openCodeLogin()
	case config.ProviderAntigravity:
		comps = append(comps, agyCLIComponent())
		login = antigravityLogin()
	case config.ProviderOpenAI:
		login = LoginTarget{ID: "openai", Name: "OpenAI API key", OK: cfg.APIKeyResolved() != "",
			Detail: "Usage is billed by OpenAI when you send prompts."}
	default:
		provider = config.ProviderCodex
		comps = append(comps, codexCLIComponent())
		login = codexLogin()
	}
	ready := true
	for _, c := range comps {
		if !c.OK {
			ready = false
		}
	}
	previousProvider := cfg.Provider
	cfg.Provider = provider
	var choices []llm.ModelChoice
	if provider != config.ProviderOpenAI {
		choices = llm.ModelChoices(llm.EcosystemForProvider(string(provider)), cfg.FableAllowed())
	}
	model := cfg.DisplayModel()
	if provider == config.ProviderOpenAI && (previousProvider != provider || model == "" || model == config.ModelAuto) {
		model = "gpt-4.1-mini"
	}
	if len(choices) > 0 && previousProvider != provider {
		model = choices[0].Value
	}
	validModel := false
	for _, choice := range choices {
		if choice.Value == model {
			validModel = true
			break
		}
	}
	if !validModel && len(choices) > 0 {
		model = choices[0].Value
	}
	ws := cfg.Workspace
	if ws == "" || ws == "." {
		ws, _ = os.Getwd()
	}
	return Status{
		Ready:          ready,
		Busy:           Busy(),
		Log:            LastLog(provider),
		Components:     comps,
		Logins:         []LoginTarget{login},
		Workspace:      ws,
		Mode:           string(cfg.PersistentMode()),
		ActiveMode:     string(cfg.Mode),
		ModeOverridden: cfg.ModeOverridden(),
		Model:          model,
		Provider:       string(provider),
		ModelOptions:   choices,
		LoggedIn:       login.OK,
		SetupDone:      cfg.SetupComplete,
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
	if p := lookWithFallback("claude", ".local", "bin", "claude"); p != "" {
		return Component{ID: "claude-cli", Name: "Claude Code CLI", OK: true, Detail: p}
	}
	return Component{ID: "claude-cli", Name: "Claude Code CLI", CanFix: true, Detail: "not installed", FixHint: "Will run npm for you."}
}

func openCodeCLIComponent() Component {
	if p := lookWithFallback("opencode", ".opencode", "bin", "opencode"); p != "" {
		return Component{ID: "opencode-cli", Name: "OpenCode CLI", OK: true, Detail: p}
	}
	return Component{ID: "opencode-cli", Name: "OpenCode CLI", CanFix: true, Detail: "not installed", FixHint: "Will install OpenCode for you."}
}

func agyCLIComponent() Component {
	if p := lookWithFallback("agy", ".local", "bin", "agy"); p != "" {
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
	okCLI := lookWithFallback("claude", ".local", "bin", "claude") != ""
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
	okCLI := lookWithFallback("opencode", ".opencode", "bin", "opencode") != ""
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
	okCLI := lookWithFallback("agy", ".local", "bin", "agy") != ""
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

func lookWithFallback(name string, parts ...string) string {
	if p := look(name); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(append([]string{home}, parts...)...)
	if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
		return path
	}
	return ""
}

func InstallCores(provider config.Provider) (string, error) {
	if _, err := Provider(string(provider)); err != nil {
		return "", err
	}
	mu.Lock()
	if busy {
		mu.Unlock()
		return "", fmt.Errorf("setup is already running")
	}
	busy = true
	lastProvider = provider
	lastLog = ""
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

	switch provider {
	case config.ProviderCodex:
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

	case config.ProviderQuadCode:
		if lookWithFallback("claude", ".local", "bin", "claude") == "" {
			if os.Getenv("PICOGENT_SETUP_SKIP_CLIS") != "" {
				say("skip  claude cli (test)")
			} else if err := installNPM("Claude Code CLI", "@anthropic-ai/claude-code", "claude", say); err != nil {
				say("miss claude  " + err.Error())
			}
		} else {
			say("ok  claude cli")
		}

	case config.ProviderOpenCode:
		if lookWithFallback("opencode", ".opencode", "bin", "opencode") == "" {
			if os.Getenv("PICOGENT_SETUP_SKIP_CLIS") != "" {
				say("skip  opencode cli (test)")
			} else if err := installOpenCode(say); err != nil {
				say("miss opencode  " + err.Error())
			}
		} else {
			say("ok  opencode cli")
		}

	case config.ProviderAntigravity:
		if lookWithFallback("agy", ".local", "bin", "agy") == "" {
			if os.Getenv("PICOGENT_SETUP_SKIP_CLIS") != "" {
				say("skip  agy cli (test)")
			} else if err := installAgy(say); err != nil {
				say("miss agy  " + err.Error())
			}
		} else {
			say("ok  agy cli")
		}
	case config.ProviderOpenAI:
		say("ok  no provider CLI needed for OpenAI Chat Completions")

	}

	if ProviderConnected(provider) {
		say("ok  provider login")
	} else {
		say("next  log in to the selected provider")
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

func StartClaudeLogin() error { return StartClaudeLoginForce(false) }
func StartClaudeLoginForce(force bool) error {
	if !force && ClaudeLoggedIn() {
		return nil
	}
	bin := lookWithFallback("claude", ".local", "bin", "claude")
	if bin == "" {
		return fmt.Errorf("Claude Code is not installed; use Install missing pieces first")
	}
	return OpenInteractive(shellQuote(bin) + " auth login")
}

func StartOpenCodeLogin() error {
	if opencodeauth.LoggedIn() {
		return nil
	}
	bin := lookWithFallback("opencode", ".opencode", "bin", "opencode")
	if bin == "" {
		return fmt.Errorf("OpenCode is not installed; use Install missing pieces first")
	}
	return OpenInteractive(shellQuote(bin) + " auth login")
}

func StartAntigravityLogin() error { return StartAntigravityLoginForce(false) }
func StartAntigravityLoginForce(force bool) error {
	if !force && agyauth.LoggedIn() {
		return nil
	}
	bin := lookWithFallback("agy", ".local", "bin", "agy")
	if bin == "" {
		return fmt.Errorf("Antigravity CLI is not installed; use Install missing pieces first")
	}
	return OpenInteractive(shellQuote(bin))
}

// StartCodexCLILogin opens Codex CLI login in a terminal (fallback when browser OAuth isn't used).
func StartCodexCLILogin() error {
	if codexauth.LoggedIn() {
		return nil
	}
	if look("codex") == "" {
		return fmt.Errorf("Codex CLI is not installed; use Install missing pieces first")
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

func Prepare(cfg config.Config, provider config.Provider, workspace, mode, model string) (config.Config, error) {
	if _, err := Provider(string(provider)); err != nil {
		return cfg, err
	}
	status := Snapshot(cfg, provider)
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
	cfg.SetUserMode(m)
	cfg.Provider = provider
	model = strings.TrimSpace(model)
	if model == "" {
		model = status.Model
	}
	if provider == config.ProviderOpenAI {
		if cfg.APIKeyResolved() == "" {
			return cfg, fmt.Errorf("enter an OpenAI API key or set OPENAI_API_KEY")
		}
		if len(model) > 128 || strings.ContainsAny(model, "\r\n\t ") || model == config.ModelAuto {
			return cfg, fmt.Errorf("enter a Chat Completions model ID")
		}
		cfg.BaseURL = "https://api.openai.com/v1"
		cfg.Model = model
		cfg.Router.Enabled = false
		cfg.SetupComplete = true
		return cfg, nil
	}
	validModel := false
	for _, choice := range status.ModelOptions {
		if choice.Value == model {
			validModel = true
			break
		}
	}
	if !validModel {
		return cfg, fmt.Errorf("choose a model available for the selected provider")
	}
	supportsAuto := provider == config.ProviderCodex || provider == config.ProviderQuadCode
	if model != config.ModelAuto {
		cfg.Model = model
		cfg.Router.Enabled = false
	} else if supportsAuto {
		cfg.Model = config.ModelAuto
		cfg.Router.Enabled = true
	} else {
		return cfg, fmt.Errorf("auto model is unavailable for the selected provider")
	}
	cfg.SetupComplete = true
	return cfg, nil
}

func Apply(cfg config.Config, workspace, mode, model string) (config.Config, error) {
	provider := cfg.Provider
	if provider == "" {
		provider = config.ProviderCodex
	}
	if _, err := Provider(string(provider)); err != nil {
		workspace = strings.TrimSpace(workspace)
		abs, pathErr := filepath.Abs(workspace)
		if pathErr != nil {
			return cfg, pathErr
		}
		st, statErr := os.Stat(abs)
		if workspace == "" || statErr != nil || !st.IsDir() {
			return cfg, fmt.Errorf("folder does not exist: %s", abs)
		}
		cfg.Workspace = abs
		modeValue := config.Mode(mode)
		if !modeValue.Valid() {
			modeValue = config.ModeSafe
		}
		cfg.SetUserMode(modeValue)
		if strings.TrimSpace(model) != "" {
			cfg.Model = strings.TrimSpace(model)
		}
		cfg.SetupComplete = true
		if err := config.Save(cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}
	var err error
	cfg, err = Prepare(cfg, provider, workspace, mode, model)
	if err != nil {
		return cfg, err
	}
	if err := config.Save(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
