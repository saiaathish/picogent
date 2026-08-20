package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
)

type globTool struct{}

func (globTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "glob",
		Description: "List files in the workspace matching a glob pattern such as **/*.go or src/*.ts.",
		Parameters: schema(map[string]any{
			"pattern": map[string]any{"type": "string"},
		}, []string{"pattern"}),
	}
}

func (globTool) Permission(args string, c Context) perm.Request {
	var in struct {
		Pattern string `json:"pattern"`
	}
	_ = parseJSON(args, &in)
	return perm.Request{Tool: "glob", Summary: "glob " + in.Pattern}
}

func (globTool) Run(_ context.Context, args string, c Context) (string, error) {
	var in struct {
		Pattern string `json:"pattern"`
	}
	if err := parseJSON(args, &in); err != nil {
		return "", err
	}
	ws, err := mustWorkspace(c)
	if err != nil {
		return "", err
	}
	matches, err := globWalk(ws, in.Pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "no matches", nil
	}
	if len(matches) > 200 {
		matches = matches[:200]
		return clip(strings.Join(matches, "\n") + "\n… truncated …"), nil
	}
	return strings.Join(matches, "\n"), nil
}

func globWalk(root, pattern string) ([]string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		pattern = "**/*"
	}
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if globMatch(pattern, rel) {
			out = append(out, rel)
		}
		return nil
	})
	return out, err
}

func globMatch(pattern, rel string) bool {
	rel = filepath.ToSlash(rel)
	pattern = filepath.ToSlash(pattern)
	// path.Match always treats '/' as separator; filepath.Match follows the OS
	// and on Windows would let '*' cross '/' after ToSlash.
	if ok, _ := path.Match(pattern, rel); ok {
		return true
	}
	if strings.HasPrefix(pattern, "**/") {
		suf := strings.TrimPrefix(pattern, "**/")
		if ok, _ := path.Match(suf, rel); ok {
			return true
		}
		if ok, _ := path.Match(suf, path.Base(rel)); ok {
			return true
		}
		parts := strings.Split(rel, "/")
		for i := range parts {
			if ok, _ := path.Match(suf, strings.Join(parts[i:], "/")); ok {
				return true
			}
		}
	}
	if strings.Contains(pattern, "**/") {
		pre, suf, _ := strings.Cut(pattern, "**/")
		if pre != "" {
			if !strings.HasPrefix(rel, strings.TrimSuffix(pre, "/")) && !strings.HasPrefix(rel, pre) {
				return false
			}
			return globMatch("**/"+suf, rel)
		}
		// Leading **/ already handled above; continue with the suffix only.
		return globMatch(suf, rel)
	}
	return false
}

type grepTool struct{}

func (grepTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "grep",
		Description: "Search file contents for a regex or literal string. Prefer this over reading the whole repo.",
		Parameters: schema(map[string]any{
			"pattern": map[string]any{"type": "string"},
			"glob":    map[string]any{"type": "string", "description": "Optional file glob, e.g. *.go"},
		}, []string{"pattern"}),
	}
}

func (grepTool) Permission(args string, c Context) perm.Request {
	var in struct {
		Pattern string `json:"pattern"`
	}
	_ = parseJSON(args, &in)
	return perm.Request{Tool: "grep", Summary: "grep " + in.Pattern}
}

func (grepTool) Run(ctx context.Context, args string, c Context) (string, error) {
	var in struct {
		Pattern string `json:"pattern"`
		Glob    string `json:"glob"`
	}
	if err := parseJSON(args, &in); err != nil {
		return "", err
	}
	ws, err := mustWorkspace(c)
	if err != nil {
		return "", err
	}
	if _, err := exec.LookPath("rg"); err == nil {
		return runRipgrep(ctx, ws, in.Pattern, in.Glob)
	}
	return walkGrep(ws, in.Pattern, in.Glob)
}

func runRipgrep(ctx context.Context, ws, pattern, glob string) (string, error) {
	args := []string{"--line-number", "--no-heading", "--color", "never", "-m", "50", "--max-filesize", "1M"}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	args = append(args, pattern, ".")
	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = ws
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil && stdout.Len() == 0 {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return "no matches", nil
		}
		return "", fmt.Errorf("%s", strings.TrimSpace(stderr.String()+" "+err.Error()))
	}
	out := stdout.String()
	if strings.TrimSpace(out) == "" {
		return "no matches", nil
	}
	return clip(out), nil
}

func walkGrep(ws, pattern, glob string) (string, error) {
	var b strings.Builder
	hits := 0
	_ = filepath.WalkDir(ws, func(path string, d os.DirEntry, err error) error {
		if err != nil || hits >= 50 {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) && path != ws {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(ws, path)
		rel = filepath.ToSlash(rel)
		if glob != "" && !globMatch(glob, rel) && !globMatch(glob, filepath.Base(rel)) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || !utf8ish(data) {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, pattern) {
				fmt.Fprintf(&b, "%s:%d:%s\n", rel, i+1, line)
				hits++
				if hits >= 50 {
					return nil
				}
			}
		}
		return nil
	})
	if hits == 0 {
		return "no matches", nil
	}
	return clip(b.String()), nil
}

func utf8ish(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return len(data) < 1<<20
}

type bashTool struct{}

func (bashTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "bash",
		Description: "Run a shell command in the workspace. No interactive programs. Never push git remotes.",
		Parameters: schema(map[string]any{
			"command": map[string]any{"type": "string"},
		}, []string{"command"}),
	}
}

func (bashTool) Permission(args string, c Context) perm.Request {
	var in struct {
		Command string `json:"command"`
	}
	_ = parseJSON(args, &in)
	return c.ClassifyBash(in.Command, c.Workspace)
}

func (bashTool) Run(ctx context.Context, args string, c Context) (string, error) {
	var in struct {
		Command string `json:"command"`
	}
	if err := parseJSON(args, &in); err != nil {
		return "", err
	}
	ws, err := mustWorkspace(c)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, c.BashTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", in.Command)
	cmd.Dir = ws
	cmd.Env = os.Environ()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	text := clip(out.String())
	if err != nil {
		if text == "" {
			return "", err
		}
		return fmt.Sprintf("%s\n(exit: %v)", text, err), nil
	}
	if strings.TrimSpace(text) == "" {
		return "(no output)", nil
	}
	return text, nil
}

type gitTool struct{}

func (gitTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "git",
		Description: "Git helper. action is status, diff, or commit. commit requires message. Never pushes.",
		Parameters: schema(map[string]any{
			"action":  map[string]any{"type": "string", "enum": []any{"status", "diff", "commit"}},
			"message": map[string]any{"type": "string"},
		}, []string{"action"}),
	}
}

func (gitTool) Permission(args string, c Context) perm.Request {
	var in struct {
		Action string `json:"action"`
	}
	_ = parseJSON(args, &in)
	req := perm.Request{Tool: "git", Summary: "git " + in.Action, Command: in.Action}
	if in.Action == "commit" {
		req.Destructive = true
	}
	return req
}

func (gitTool) Run(ctx context.Context, args string, c Context) (string, error) {
	var in struct {
		Action  string `json:"action"`
		Message string `json:"message"`
	}
	if err := parseJSON(args, &in); err != nil {
		return "", err
	}
	ws, err := mustWorkspace(c)
	if err != nil {
		return "", err
	}
	switch in.Action {
	case "status":
		return gitOut(ctx, ws, "status", "--short")
	case "diff":
		return gitOut(ctx, ws, "diff")
	case "commit":
		if strings.TrimSpace(in.Message) == "" {
			return "", fmt.Errorf("commit requires message")
		}
		return gitOut(ctx, ws, "commit", "-m", in.Message)
	default:
		return "", fmt.Errorf("unknown git action %q", in.Action)
	}
}

func gitOut(ctx context.Context, ws string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = ws
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	text := clip(strings.TrimSpace(out.String()))
	if err != nil {
		if text == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", text)
	}
	if text == "" {
		return "(clean)", nil
	}
	return text, nil
}
