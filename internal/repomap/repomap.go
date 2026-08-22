// Package repomap provides an on-demand, deterministic summary of a repository.
// It deliberately keeps no index, cache, watcher, or background process.
package repomap

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// MaxOutputBytes is the hard limit for formatted repo-map output.
	MaxOutputBytes = 12 << 10
	maxFiles       = 20_000
	maxListItems   = 64
	maxValueBytes  = 512
)

// Commands lists deterministic commands discovered from project manifests.
type Commands struct {
	Build []string `json:"build,omitempty"`
	Test  []string `json:"test,omitempty"`
	Lint  []string `json:"lint,omitempty"`
}

// GitState is a cheap snapshot of the repository's current git state.
type GitState struct {
	Repository bool   `json:"repository"`
	Branch     string `json:"branch,omitempty"`
	Head       string `json:"head,omitempty"`
	Clean      bool   `json:"clean"`
	Staged     int    `json:"staged,omitempty"`
	Modified   int    `json:"modified,omitempty"`
	Untracked  int    `json:"untracked,omitempty"`
}

// Map is a bounded repository summary suitable for agent context.
type Map struct {
	Version         int      `json:"version"`
	Root            string   `json:"root"`
	Languages       []string `json:"languages,omitempty"`
	Frameworks      []string `json:"frameworks,omitempty"`
	PackageManagers []string `json:"package_managers,omitempty"`
	Commands        Commands `json:"commands"`
	Git             GitState `json:"git"`
	Manifests       []string `json:"manifests,omitempty"`
	SourceRoots     []string `json:"source_roots,omitempty"`
	TestRoots       []string `json:"test_roots,omitempty"`
	Rules           []string `json:"project_rules,omitempty"`
	InventoryFiles  int      `json:"inventory_files"`
	InventoryCutOff bool     `json:"inventory_cut_off,omitempty"`
	OutputTruncated bool     `json:"output_truncated,omitempty"`
}

// Inspect creates a fresh repo map. It never stores repository contents.
func Inspect(ctx context.Context, root string) (Map, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Map{}, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return Map{}, err
	}
	if !st.IsDir() {
		return Map{}, errors.New("repo map root is not a directory")
	}

	files, cutOff, err := inventory(ctx, abs)
	if err != nil {
		return Map{}, err
	}
	m := Map{
		Version:         1,
		Root:            cleanValue(abs),
		Git:             inspectGit(ctx, abs),
		InventoryFiles:  len(files),
		InventoryCutOff: cutOff,
	}
	detectFiles(abs, files, &m)
	return m, nil
}

// Generate is an alias for Inspect for callers that treat repo maps as output.
func Generate(ctx context.Context, root string) (Map, error) { return Inspect(ctx, root) }

// Build is an alias for Inspect.
func Build(ctx context.Context, root string) (Map, error) { return Inspect(ctx, root) }

// Format renders stable JSON and keeps it within MaxOutputBytes.
func Format(m Map) string {
	m = boundedMap(m)
	for {
		out, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return `{"version":1,"output_truncated":true}`
		}
		if len(out) <= MaxOutputBytes {
			return string(out)
		}
		m.OutputTruncated = true
		if !dropOne(&m) {
			if len(m.Root) > 128 {
				m.Root = cleanValue(m.Root[:128])
			}
			out, _ = json.Marshal(m)
			if len(out) > MaxOutputBytes {
				return `{"version":1,"output_truncated":true}`
			}
			return string(out)
		}
	}
}

func boundedMap(m Map) Map {
	m.Root = cleanValue(m.Root)
	m.Languages = boundList(m.Languages)
	m.Frameworks = boundList(m.Frameworks)
	m.PackageManagers = boundList(m.PackageManagers)
	m.Commands.Build = boundList(m.Commands.Build)
	m.Commands.Test = boundList(m.Commands.Test)
	m.Commands.Lint = boundList(m.Commands.Lint)
	m.Manifests = boundList(m.Manifests)
	m.SourceRoots = boundList(m.SourceRoots)
	m.TestRoots = boundList(m.TestRoots)
	m.Rules = boundList(m.Rules)
	m.Git.Branch = cleanValue(m.Git.Branch)
	m.Git.Head = cleanValue(m.Git.Head)
	return m
}

func dropOne(m *Map) bool {
	lists := []*[]string{
		&m.Manifests, &m.SourceRoots, &m.TestRoots, &m.Rules,
		&m.Commands.Build, &m.Commands.Test, &m.Commands.Lint,
		&m.Frameworks, &m.PackageManagers, &m.Languages,
	}
	best := -1
	for i := range lists {
		if best < 0 || len(*lists[i]) > len(*lists[best]) {
			best = i
		}
	}
	if best < 0 || len(*lists[best]) == 0 {
		return false
	}
	values := *lists[best]
	*lists[best] = values[:len(values)-1]
	return true
}

func inventory(ctx context.Context, root string) ([]string, bool, error) {
	if files, cutOff, ok := gitFiles(ctx, root); ok {
		return files, cutOff, nil
	}
	return walkFiles(ctx, root)
}

func gitFiles(ctx context.Context, root string) ([]string, bool, bool) {
	gitRoot, err := commandText(ctx, root, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, false, false
	}
	resolved, err := filepath.EvalSymlinks(strings.TrimSpace(gitRoot))
	if err != nil {
		resolved = strings.TrimSpace(gitRoot)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		want = root
	}
	if filepath.Clean(resolved) != filepath.Clean(want) {
		return nil, false, false
	}

	cmdCtx, cancel := shortContext(ctx)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "git", "ls-files", "-co", "--exclude-standard", "-z")
	cmd.Dir = root
	pipe, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return nil, false, false
	}
	reader := bufio.NewReader(pipe)
	files := make([]string, 0, 1024)
	cutOff := false
	for len(files) < maxFiles {
		name, readErr := reader.ReadString(0)
		name = strings.TrimSuffix(name, "\x00")
		if name != "" {
			files = append(files, filepath.ToSlash(filepath.Clean(name)))
		}
		if readErr != nil {
			if readErr != io.EOF {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return nil, false, false
			}
			break
		}
	}
	if len(files) == maxFiles {
		cutOff = true
		_ = cmd.Process.Kill()
	}
	err = cmd.Wait()
	if err != nil && !cutOff {
		return nil, false, false
	}
	sort.Strings(files)
	files = compact(files)
	return files, cutOff, true
}

func walkFiles(ctx context.Context, root string) ([]string, bool, error) {
	files := make([]string, 0, 512)
	cutOff := false
	errStop := errors.New("repo map inventory limit reached")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if entry.IsDir() && ignoredDir(entry.Name()) {
			return filepath.SkipDir
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		if len(files) >= maxFiles {
			cutOff = true
			return errStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		return nil, false, err
	}
	sort.Strings(files)
	return files, cutOff, nil
}

func ignoredDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", ".picogent", "node_modules", "vendor", "target", "dist", "build", ".next", ".venv", "venv", "__pycache__", "coverage", "graphify-out":
		return true
	default:
		return false
	}
}

func inspectGit(ctx context.Context, root string) GitState {
	inside, err := commandText(ctx, root, "git", "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		return GitState{}
	}
	head, _ := commandText(ctx, root, "git", "rev-parse", "--short=12", "HEAD")
	branch, _ := commandText(ctx, root, "git", "branch", "--show-current")
	status, err := commandText(ctx, root, "git", "status", "--porcelain=v1", "--untracked-files=normal")
	g := GitState{Repository: true, Branch: strings.TrimSpace(branch), Head: strings.TrimSpace(head)}
	if err != nil {
		return g
	}
	for _, line := range strings.Split(status, "\n") {
		if len(line) < 2 {
			continue
		}
		if strings.HasPrefix(line, "??") {
			g.Untracked++
			continue
		}
		if line[0] != ' ' {
			g.Staged++
		}
		if line[1] != ' ' {
			g.Modified++
		}
	}
	g.Clean = g.Staged == 0 && g.Modified == 0 && g.Untracked == 0
	return g
}

func commandText(ctx context.Context, root, name string, args ...string) (string, error) {
	cmdCtx, cancel := shortContext(ctx)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if out.Len() > 1<<20 {
		return out.String()[:1<<20], err
	}
	return out.String(), err
}

func shortContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, 3*time.Second)
}

func boundList(values []string) []string {
	values = append([]string(nil), values...)
	for i := range values {
		values[i] = cleanValue(values[i])
	}
	sort.Strings(values)
	values = compact(values)
	if len(values) > maxListItems {
		values = values[:maxListItems]
	}
	return values
}

func compact(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if value == "" || len(out) > 0 && out[len(out)-1] == value {
			continue
		}
		out = append(out, value)
	}
	return out
}

func cleanValue(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	if len(value) <= maxValueBytes {
		return value
	}
	return value[:maxValueBytes-3] + "..."
}
