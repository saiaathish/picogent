package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/gui"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/mcpbridge"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/setup"
	"github.com/saiaathish/picogent/internal/tui"
)

var version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		if config.NeedsSetup() {
			return gui.Run()
		}
		return tui.Run()
	}
	switch args[0] {
	case "gui":
		return gui.Run()
	case "setup":
		return gui.RunSetup()
	case "tui":
		return tui.Run()
	case "run":
		return runOnce(args[1:])
	case "init":
		return runInitArgs(args[1:])
	case "login":
		return runLogin(os.Args[2:])
	case "mcp":
		return runMCP(args[1:])
	case "version", "-v", "--version":
		fmt.Println("picogent", version)
		return nil
	case "help", "-h", "--help":
		printHelp()
		return nil
	default:
		if strings.HasPrefix(args[0], "-") {
			return runOnce(args)
		}
		printHelp()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printHelp() {
	fmt.Print(`picogent — tiny coding agent

Usage:
  picogent              first run: install cores + browser setup; later: TUI
  picogent gui          browser chat (setup first if needed)
  picogent setup        open the browser setup again
  picogent tui          terminal UI
  picogent run "..."    one-shot prompt (headless)
  picogent login        connect ChatGPT Codex (~/.codex/auth.json)
  picogent login claude connect Claude Code CLI (subscription, no API key)
  picogent mcp          list connected MCP tools
  picogent init         write ~/.picogent/config.yaml
  picogent version

No extra app. The GUI is a local page in your browser.

Default backend is your Codex subscription (same auth file as Codex CLI).
Claude Code provider uses the same login as the claude CLI — no Anthropic API key.

Init flags:
  --ollama              use local Ollama
  --model NAME          default model
  --base-url URL        OpenAI-compatible base URL

Run flags:
  --dir PATH            workspace (default: current directory)
  --yes                 Fast mode and auto-approve in-workspace writes/shell
  --model NAME          override model
`)
}

func runLogin(args []string) error {
	target := "codex"
	if len(args) > 0 {
		target = strings.ToLower(args[0])
	}
	switch target {
	case "claude", "quadcode", "anthropic":
		return runClaudeLogin()
	case "codex", "chatgpt", "":
		return runCodexLogin()
	default:
		return fmt.Errorf("unknown login target %q (try: picogent login  or  picogent login claude)", args[0])
	}
}

func runCodexLogin() error {
	codex, err := exec.LookPath("codex")
	if err != nil {
		return fmt.Errorf("Problem: Codex CLI is not installed.\nCause:   `codex` is not on PATH.\nFix:     install the Codex CLI, then run picogent login")
	}
	cmd := exec.Command(codex, "login")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if !codexauth.LoggedIn() {
		return fmt.Errorf("login finished, but ~/.codex/auth.json is still empty")
	}
	fmt.Println("Codex connected. Run picogent or picogent gui.")
	return nil
}

func runClaudeLogin() error {
	if err := setup.StartClaudeLogin(); err != nil {
		return err
	}
	if setup.ClaudeLoggedIn() {
		fmt.Println("Claude Code already connected. In Settings pick provider Claude Code.")
		return nil
	}
	fmt.Println("Finish Claude login in the window that opened, then pick Claude Code in Settings.")
	return nil
}

func runMCP(args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	if sub != "list" {
		return fmt.Errorf("unknown mcp subcommand %q (try: picogent mcp list)", sub)
	}
	wd, _ := os.Getwd()
	servers, err := mcpbridge.LoadServers(wd)
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		fmt.Println("No MCP servers configured.")
		fmt.Println("Add ~/.picogent/mcp.yaml or use ~/.cursor/mcp.json (same format as Cursor).")
		return nil
	}
	fmt.Printf("Configured servers (%d):\n", len(servers))
	for name := range servers {
		fmt.Println(" ", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	mgr, warns := mcpbridge.ConnectBestEffort(ctx, servers)
	defer mgr.Close()
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	lines := mgr.Report()
	if len(lines) == 0 {
		fmt.Println("No tools connected (servers may be offline).")
		return nil
	}
	fmt.Println("\nConnected tools:")
	for _, line := range lines {
		fmt.Println(line)
	}
	return nil
}

func runInitArgs(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	ollama := fs.Bool("ollama", false, "use local Ollama")
	model := fs.String("model", "", "model name")
	base := fs.String("base-url", "", "OpenAI-compatible base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := config.Default()
	if existing, err := config.Load(); err == nil {
		cfg = existing
	}
	if *ollama {
		cfg.Provider = config.ProviderOllama
		if *model == "" {
			*model = "qwen2.5-coder:7b"
		}
	}
	if *model != "" {
		cfg.Model = *model
	}
	if *base != "" {
		cfg.BaseURL = *base
		cfg.Provider = config.ProviderOpenAI
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	path, _ := config.Path()
	fmt.Println("wrote", path)
	switch cfg.Provider {
	case config.ProviderOllama:
		fmt.Println("next: ollama serve && ollama pull", cfg.Model)
	case config.ProviderCodex:
		if codexauth.LoggedIn() {
			fmt.Println("Codex already connected via ~/.codex/auth.json")
			fmt.Println("next: picogent   or   picogent gui")
		} else {
			fmt.Println("next: picogent login")
		}
	default:
		fmt.Println("next: export PICOGENT_API_KEY=sk-...   (or set api_key in the yaml)")
	}
	return nil
}

func runOnce(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	dir := fs.String("dir", ".", "workspace")
	yes := fs.Bool("yes", false, "auto-approve in-workspace writes and shell")
	model := fs.String("model", "", "model override")
	if err := fs.Parse(args); err != nil {
		return err
	}
	prompt := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("Problem: missing prompt.\nCause:   picogent run needs text.\nFix:     picogent run \"create hello.txt\"")
	}
	cfg, a, err := app.Load(*dir)
	if err != nil {
		return err
	}
	if *model != "" {
		cfg.Model = *model
		a.CFG.Model = *model
	}
	if *yes {
		cfg.Mode = config.ModeFast
		a.CFG.Mode = config.ModeFast
		a.Gate.Mode = config.ModeFast
	}
	h := &stdioHandler{yes: *yes, in: bufio.NewReader(os.Stdin)}
	_, _, err = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: prompt}, h)
	return err
}

type stdioHandler struct {
	yes bool
	in  *bufio.Reader
}

func (h *stdioHandler) OnText(text string) {
	if text == "" {
		fmt.Println()
		return
	}
	fmt.Println(text)
}
func (h *stdioHandler) OnTextDelta(delta string) {
	fmt.Print(delta)
}
func (h *stdioHandler) OnToolStart(call llm.ToolCall) {
	fmt.Fprintf(os.Stderr, "→ %s %s\n", call.Name, short(call.Arguments, 80))
}
func (h *stdioHandler) OnToolEnd(_ llm.ToolCall, result string, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "  error:", err)
		return
	}
	fmt.Fprintln(os.Stderr, " ", short(result, 120))
}
func (h *stdioHandler) OnNeedPermission(_ context.Context, req perm.Request) (perm.Decision, error) {
	if h.yes && !req.Destructive && !req.OutsideWorkspace {
		return perm.Allow, nil
	}
	if h.yes && (req.Destructive || req.OutsideWorkspace) {
		return perm.Deny, nil
	}
	fmt.Printf("Allow %s? [y/n] ", req.Summary)
	line, err := h.in.ReadString('\n')
	if err != nil {
		return perm.Deny, err
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
		return perm.Allow, nil
	}
	return perm.Deny, nil
}
func (h *stdioHandler) OnError(err error) { fmt.Fprintln(os.Stderr, err.Error()) }

func short(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
