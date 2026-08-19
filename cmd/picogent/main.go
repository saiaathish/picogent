package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/gui"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tui")

var version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return tui.Run()
	}
	switch args[0] {
	case "gui":
		return gui.Run()
	case "run":
		return runOnce(args[1:])
	case "init":
		return runInitArgs(args[1:])
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
  picogent              start the TUI
  picogent gui          start the local web GUI
  picogent run "..."    one-shot prompt (headless)
  picogent init         write ~/.picogent/config.yaml
  picogent version

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
	if cfg.Provider == config.ProviderOllama {
		fmt.Println("next: ollama serve && ollama pull", cfg.Model)
		return nil
	}
	fmt.Println("next: export PICOGENT_API_KEY=sk-...   (or set api_key in the yaml)")
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
	_, _, err = a.Run(context.Background(), nil, prompt, h)
	return err
}

type stdioHandler struct {
	yes bool
	in  *bufio.Reader
}

func (h *stdioHandler) OnText(text string) { fmt.Println(text) }
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
