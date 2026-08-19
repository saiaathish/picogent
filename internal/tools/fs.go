package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
)

type readFile struct{}

func (readFile) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "read_file",
		Description: "Read a UTF-8 text file in the workspace. Use this before editing.",
		Parameters: schema(map[string]any{
			"path": map[string]any{"type": "string", "description": "Path relative to the workspace"},
		}, []string{"path"}),
	}
}

func (readFile) Permission(args string, c Context) perm.Request {
	var in struct {
		Path string `json:"path"`
	}
	_ = parseJSON(args, &in)
	return c.ClassifyPath("read_file", in.Path, c.Workspace, "read "+in.Path)
}

func (readFile) Run(_ context.Context, args string, c Context) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := parseJSON(args, &in); err != nil {
		return "", err
	}
	ws, err := mustWorkspace(c)
	if err != nil {
		return "", err
	}
	abs, err := resolvePath(ws, in.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("not a utf-8 text file: %s", relDisplay(ws, abs))
	}
	text := string(data)
	if len(data) > maxReadBytes {
		text = text[:maxReadBytes] + "\n… truncated (file larger than 256KiB) …"
	}
	lines := strings.Split(text, "\n")
	if len(lines) > maxReadLines {
		text = strings.Join(lines[:maxReadLines], "\n") + "\n… truncated (more than 2000 lines) …"
	}
	return clip(text), nil
}

type writeFile struct{}

func (writeFile) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "write_file",
		Description: "Create or overwrite a text file. Prefer edit_file for small changes to existing files.",
		Parameters: schema(map[string]any{
			"path":    map[string]any{"type": "string"},
			"content": map[string]any{"type": "string"},
		}, []string{"path", "content"}),
	}
}

func (writeFile) Permission(args string, c Context) perm.Request {
	var in struct {
		Path string `json:"path"`
	}
	_ = parseJSON(args, &in)
	return c.ClassifyPath("write_file", in.Path, c.Workspace, "write "+in.Path)
}

func (writeFile) Run(_ context.Context, args string, c Context) (string, error) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := parseJSON(args, &in); err != nil {
		return "", err
	}
	ws, err := mustWorkspace(c)
	if err != nil {
		return "", err
	}
	abs, err := resolvePath(ws, in.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(in.Content), 0o644); err != nil {
		return "", err
	}
	return "wrote " + relDisplay(ws, abs), nil
}

type editFile struct{}

func (editFile) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "edit_file",
		Description: "Replace exactly one occurrence of old_string with new_string in a file. old_string must be unique.",
		Parameters: schema(map[string]any{
			"path":       map[string]any{"type": "string"},
			"old_string": map[string]any{"type": "string"},
			"new_string": map[string]any{"type": "string"},
		}, []string{"path", "old_string", "new_string"}),
	}
}

func (editFile) Permission(args string, c Context) perm.Request {
	var in struct {
		Path string `json:"path"`
	}
	_ = parseJSON(args, &in)
	return c.ClassifyPath("edit_file", in.Path, c.Workspace, "edit "+in.Path)
}

func (editFile) Run(_ context.Context, args string, c Context) (string, error) {
	var in struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := parseJSON(args, &in); err != nil {
		return "", err
	}
	if in.OldString == "" {
		return "", fmt.Errorf("old_string is required")
	}
	ws, err := mustWorkspace(c)
	if err != nil {
		return "", err
	}
	abs, err := resolvePath(ws, in.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	text := string(data)
	n := strings.Count(text, in.OldString)
	if n == 0 {
		return "", fmt.Errorf("old_string not found in %s", relDisplay(ws, abs))
	}
	if n > 1 {
		return "", fmt.Errorf("old_string found %d times in %s; make it unique", n, relDisplay(ws, abs))
	}
	updated := strings.Replace(text, in.OldString, in.NewString, 1)
	if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
		return "", err
	}
	return "edited " + relDisplay(ws, abs), nil
}
