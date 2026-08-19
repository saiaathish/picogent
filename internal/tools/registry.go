package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
)

const (
	maxReadBytes = 256 << 10
	maxReadLines = 2000
	maxToolOut   = 32 << 10
)

type Context struct {
	Workspace      string
	BashTimeout    time.Duration
	ClassifyBash   func(command, workspace string) perm.Request
	ClassifyPath   func(tool, path, workspace, summary string) perm.Request
}

type Tool interface {
	Spec() llm.ToolSpec
	Permission(args string, c Context) perm.Request
	Run(ctx context.Context, args string, c Context) (string, error)
}

type Registry struct {
	byName map[string]Tool
	Ctx    Context
}

func NewRegistry(c Context) *Registry {
	if c.ClassifyBash == nil {
		c.ClassifyBash = perm.ClassifyBash
	}
	if c.ClassifyPath == nil {
		c.ClassifyPath = perm.ClassifyPath
	}
	if c.BashTimeout <= 0 {
		c.BashTimeout = 60 * time.Second
	}
	r := &Registry{byName: map[string]Tool{}, Ctx: c}
	for _, t := range []Tool{
		readFile{},
		writeFile{},
		editFile{},
		globTool{},
		grepTool{},
		bashTool{},
		gitTool{},
	} {
		r.byName[t.Spec().Name] = t
	}
	return r
}

func (r *Registry) Specs() []llm.ToolSpec {
	order := []string{"read_file", "write_file", "edit_file", "glob", "grep", "bash", "git"}
	out := make([]llm.ToolSpec, 0, len(order))
	for _, name := range order {
		out = append(out, r.byName[name].Spec())
	}
	return out
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

func parseJSON(args string, dest any) error {
	args = strings.TrimSpace(args)
	if args == "" {
		args = "{}"
	}
	if err := json.Unmarshal([]byte(args), dest); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func resolvePath(workspace, p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	return filepath.Abs(filepath.Join(workspace, p))
}

func relDisplay(workspace, abs string) string {
	rel, err := filepath.Rel(workspace, abs)
	if err != nil {
		return abs
	}
	return rel
}

func clip(s string) string {
	if len(s) <= maxToolOut {
		return s
	}
	return s[:maxToolOut] + "\n… truncated …"
}

func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "build", ".venv", "vendor", "target", ".picogent", ".idea", ".next":
		return true
	}
	return false
}

func mustWorkspace(c Context) (string, error) {
	ws, err := filepath.Abs(c.Workspace)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(ws)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", ws)
	}
	return ws, nil
}

func schema(props map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}
