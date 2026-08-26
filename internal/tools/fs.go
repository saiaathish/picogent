package tools

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/workspace"
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

func (readFile) Run(ctx context.Context, args string, c Context) (string, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
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
	f, err := workspace.OpenRead(ws, abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, truncated, err := readBoundedReader(f, maxReadBytes)
	if err != nil {
		return "", err
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	if truncated {
		data, err = trimIncompleteUTF8(data)
		if err != nil {
			return "", fmt.Errorf("not a utf-8 text file: %s", relDisplay(ws, abs))
		}
	} else if !utf8.Valid(data) {
		return "", fmt.Errorf("not a utf-8 text file: %s", relDisplay(ws, abs))
	}
	text := string(data)
	lines := strings.Split(text, "\n")
	if len(lines) > maxReadLines {
		text = strings.Join(lines[:maxReadLines], "\n") + "\n… truncated (more than 2000 lines) …"
	}
	if truncated {
		return clipWithSuffix(text, "\n… truncated (file larger than 256KiB) …"), nil
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

func (writeFile) Run(ctx context.Context, args string, c Context) (string, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
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
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	f, err := workspace.OpenWrite(ws, abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	if err := f.Truncate(0); err != nil {
		return "", err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	if err := writeAll(f, []byte(in.Content)); err != nil {
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

func (editFile) Run(ctx context.Context, args string, c Context) (string, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
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
	f, err := workspace.OpenEdit(ws, abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, truncated, err := readBoundedReader(f, maxReadBytes)
	if err != nil {
		return "", err
	}
	if truncated {
		return "", fmt.Errorf("file is larger than 256KiB: %s", relDisplay(ws, abs))
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("not a utf-8 text file: %s", relDisplay(ws, abs))
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
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	if err := f.Truncate(0); err != nil {
		return "", err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	if err := writeAll(f, []byte(updated)); err != nil {
		return "", err
	}
	return "edited " + relDisplay(ws, abs), nil
}

func readBoundedReader(r io.Reader, limit int) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) <= limit {
		return data, false, nil
	}
	return data[:limit], true, nil
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

// trimIncompleteUTF8 preserves a valid prefix when the byte cap lands in the
// middle of a final UTF-8 rune. Invalid data earlier in the file is rejected.
func trimIncompleteUTF8(data []byte) ([]byte, error) {
	if utf8.Valid(data) {
		return data, nil
	}
	start := len(data) - utf8.UTFMax + 1
	if start < 0 {
		start = 0
	}
	for n := len(data) - 1; n >= start; n-- {
		if utf8.Valid(data[:n]) {
			return data[:n], nil
		}
	}
	return nil, fmt.Errorf("invalid utf-8")
}

func clipWithSuffix(text, suffix string) string {
	if len(text)+len(suffix) <= maxToolOut {
		return text + suffix
	}
	return text[:maxToolOut-len(suffix)] + suffix
}
