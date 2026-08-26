package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/saiaathish/picogent/internal/gitobs"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/redact"
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
	cmd := shellCommand(ctx, in.Command)
	cmd.Dir = ws
	cmd.Env = sanitizedCommandEnv()
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

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		shell := os.Getenv("ComSpec")
		if shell == "" {
			shell = "cmd.exe"
		}
		return exec.CommandContext(ctx, shell, "/D", "/S", "/C", command)
	}
	// Do not source a user's shell profile: profiles are arbitrary code and
	// commonly export credentials or change the working directory.
	return exec.CommandContext(ctx, "bash", "--noprofile", "--norc", "-lc", command)
}

// sanitizedCommandEnv preserves normal build/runtime settings while keeping
// API keys, auth cookies, preload hooks, and shell startup hooks out of model
// visible command output.  An explicit user-approved command can still opt
// into its own environment, but Fast/--yes never leaks the parent process's
// credentials by default.
func sanitizedCommandEnv() []string {
	out := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || unsafeCommandEnvKey(key) {
			continue
		}
		out = append(out, entry)
	}
	if os.Getenv("PATH") != "" && !hasEnvKey(out, "PATH") {
		out = append(out, "PATH="+os.Getenv("PATH"))
	}
	return out
}

func hasEnvKey(env []string, want string) bool {
	for _, entry := range env {
		if key, _, ok := strings.Cut(entry, "="); ok && key == want {
			return true
		}
	}
	return false
}

func unsafeCommandEnvKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if upper == "" {
		return true
	}
	for _, marker := range []string{
		"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "AUTH", "COOKIE", "CREDENTIAL", "PRIVATE", "OAUTH",
	} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	for _, exact := range []string{
		"BASH_ENV", "ENV", "CDPATH", "PROMPT_COMMAND", "SHELLOPTS", "BASHOPTS", "PS4",
		"LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "DYLD_LIBRARY_PATH",
		"NODE_OPTIONS", "PYTHONPATH", "PYTHONSTARTUP", "RUBYOPT", "PERL5OPT", "GIT_EXEC_PATH",
	} {
		if upper == exact {
			return true
		}
	}
	return strings.HasPrefix(upper, "DYLD_") || strings.HasPrefix(upper, "LD_")
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
	result, err := gitobs.Combined(ctx, ws, args...)
	text := redact.Text(strings.TrimSpace(result.Output))
	if result.Truncated {
		text += "\n… git output truncated …"
	}
	text = clip(text)
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
