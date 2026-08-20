package slash

import (
	"os/exec"
	"strings"

	"github.com/saiaathish/picogent/internal/commands"
	"github.com/saiaathish/picogent/internal/evolve"
	"github.com/saiaathish/picogent/internal/projectctx"
)

type Kind int

const (
	Unknown Kind = iota
	Local
	Prompt
)

// Resolve maps input to a local TUI action or an agent prompt (Claude Code-style).
func Resolve(workspace, line string) (Kind, string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "/") {
		return Unknown, line
	}
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return Unknown, line
	}
	name := strings.ToLower(strings.TrimPrefix(parts[0], "/"))
	_ = strings.TrimSpace(strings.TrimPrefix(line, parts[0]))

	if prompt, ok := commands.Resolve(workspace, line); ok {
		return Prompt, prompt
	}

	switch name {
	case "commit":
		return Prompt, commitPrompt()
	case "review":
		return Prompt, reviewPrompt()
	case "clear", "reset":
		return Local, "clear"
	case "compact":
		return Local, "compact"
	case "status":
		return Local, "status"
	case "diff":
		return Local, "diff"
	case "memory":
		mem := projectctx.Load(workspace)
		if store, err := evolve.Load(workspace); err == nil {
			if summary := evolve.Summary(store); summary != "" {
				if mem != "" {
					mem += "\n\n"
				}
				mem += summary
			}
		}
		return Local, "memory:" + mem
	case "resume":
		return Local, "resume"
	case "commands":
		return Local, "commands"
	case "agent":
		return Local, "task:agent"
	case "ask":
		return Local, "task:ask"
	case "plan":
		return Local, "task:plan"
	case "debug":
		return Local, "task:debug"
	case "goal":
		rest := strings.TrimSpace(strings.TrimPrefix(line, parts[0]))
		if rest == "" {
			return Local, "goal:show"
		}
		if strings.EqualFold(rest, "clear") {
			return Local, "goal:clear"
		}
		return Local, "goal:set:" + rest
	default:
		return Unknown, line
	}
}

type Item struct {
	Name   string `json:"name"`
	Hint   string `json:"hint"`
	Insert string `json:"insert,omitempty"`
}

// Catalog is the / menu in the GUI.
func Catalog(workspace string) []Item {
	out := []Item{
		{Name: "commit", Hint: "Commit current changes"},
		{Name: "review", Hint: "Review uncommitted diffs"},
		{Name: "status", Hint: "Mode, model, workspace"},
		{Name: "memory", Hint: "Project rules + what Picogent learned"},
		{Name: "clear", Hint: "New chat"},
	}
	for _, name := range commands.List(workspace) {
		out = append(out, Item{Name: name, Hint: "Custom command"})
	}
	return out
}

func commitPrompt() string {
	return `Create a git commit for the current changes.

Git safety:
- NEVER update git config
- NEVER skip hooks unless the user explicitly asks
- NEVER amend unless the user explicitly asks and HEAD was not pushed
- Do not commit secrets (.env, credentials.json)
- No empty commits if nothing changed
- No interactive git (-i)

Steps:
1. Run git status and git diff HEAD
2. Stage relevant files
3. Commit with a concise 1-2 sentence message matching this repo's style (focus on why)

Use bash/git tools only. Do not explain — just do it.`
}

func reviewPrompt() string {
	return `Review the current uncommitted changes in this repository.

1. Run git diff and read changed files as needed
2. List findings: bugs, edge cases, style issues (if any)
3. End with: Verdict: OK | needs fixes

Be concise. Use tools to inspect — do not guess.`
}

func GitDiff() string {
	out, err := exec.Command("git", "diff", "HEAD").CombinedOutput()
	if err != nil {
		return string(out)
	}
	if len(out) == 0 {
		return "(no diff)"
	}
	if len(out) > 8000 {
		return string(out[:8000]) + "\n… truncated …"
	}
	return string(out)
}
