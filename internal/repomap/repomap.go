// Package repomap provides an on-demand, deterministic summary of a repository.
// It deliberately keeps no index, cache, watcher, or background process.
package repomap

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/gitobs"
	"github.com/saiaathish/picogent/internal/redact"
)

const (
	// MaxOutputBytes is the hard limit for formatted repo-map output.
	MaxOutputBytes   = 12 << 10
	maxFiles         = 20_000
	maxListItems     = 64
	maxValueBytes    = 512
	maxManifestDepth = 8
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

// Snapshot is an on-demand provenance capture paired with the bounded map.
// It deliberately has no persistence or invalidation machinery: callers take a
// fresh snapshot whenever they need to reason about the current workspace.
type Snapshot struct {
	Summary                Map
	Root                   string
	GitRoot                string
	Head                   string
	HeadKnown              bool
	DirtyKnown             bool
	DirtyPaths             []string
	DirtyPathsTruncated    bool
	ManifestPaths          []string
	ManifestPathsTruncated bool
}

// Delta describes observable provenance changes between two snapshots. It is
// intentionally not a content-equality claim; a stable delta only means that
// the captured metadata did not change.
type Delta struct {
	RootChanged          bool
	HeadChanged          bool
	DirtyPathsChanged    bool
	ManifestPathsChanged bool
	AddedDirtyPaths      []string
	RemovedDirtyPaths    []string
	AddedManifestPaths   []string
	RemovedManifestPaths []string
	RequiresRefresh      bool
}

type snapshotProvenance struct {
	Root                   string   `json:"root"`
	GitRoot                string   `json:"git_root,omitempty"`
	Head                   string   `json:"head,omitempty"`
	HeadKnown              bool     `json:"head_known"`
	Tree                   string   `json:"tree"`
	DirtyPathsKnown        bool     `json:"dirty_paths_known"`
	DirtyPaths             []string `json:"dirty_paths,omitempty"`
	DirtyPathsTruncated    bool     `json:"dirty_paths_truncated,omitempty"`
	ManifestPaths          []string `json:"manifest_paths,omitempty"`
	ManifestPathsTruncated bool     `json:"manifest_paths_truncated,omitempty"`
}

type snapshotOutput struct {
	Map
	Provenance snapshotProvenance `json:"provenance"`
}

type captureGitResult struct {
	root                string
	state               GitState
	head                string
	headKnown           bool
	dirtyKnown          bool
	dirtyPaths          []string
	dirtyPathsTruncated bool
}

type parsedGitStatus struct {
	state               GitState
	head                string
	headKnown           bool
	dirtyPaths          []string
	dirtyPathsTruncated bool
}

// Inspect creates a fresh repo map. It never stores repository contents.
func Inspect(ctx context.Context, root string) (Map, error) {
	m, _, err := inspect(ctx, root)
	return m, err
}

// Capture returns a bounded map plus exact, workspace-scoped provenance.
// Git failures are represented as unknown evidence rather than as clean state.
func Capture(ctx context.Context, root string) (Snapshot, error) {
	abs, err := absoluteDirectory(root)
	if err != nil {
		return Snapshot{}, err
	}
	files, cutOff, err := inventory(ctx, abs)
	if err != nil {
		return Snapshot{}, err
	}
	captured := captureGit(ctx, abs)
	m := mapFromFiles(abs, files, cutOff, captured.state)
	manifests, manifestsTruncated := manifestPaths(files)
	return Snapshot{
		Summary:                m,
		Root:                   abs,
		GitRoot:                captured.root,
		Head:                   captured.head,
		HeadKnown:              captured.headKnown,
		DirtyKnown:             captured.dirtyKnown,
		DirtyPaths:             captured.dirtyPaths,
		DirtyPathsTruncated:    captured.dirtyPathsTruncated,
		ManifestPaths:          manifests,
		ManifestPathsTruncated: manifestsTruncated,
	}, nil
}

func inspect(ctx context.Context, root string) (Map, []string, error) {
	abs, err := absoluteDirectory(root)
	if err != nil {
		return Map{}, nil, err
	}
	files, cutOff, err := inventory(ctx, abs)
	if err != nil {
		return Map{}, nil, err
	}
	return mapFromFiles(abs, files, cutOff, inspectGit(ctx, abs)), files, nil
}

func mapFromFiles(abs string, files []string, cutOff bool, git GitState) Map {
	m := Map{
		Version:         1,
		Root:            cleanValue(abs),
		Git:             git,
		InventoryFiles:  len(files),
		InventoryCutOff: cutOff,
	}
	detectFiles(abs, files, &m)
	return m
}

// Format renders stable JSON and keeps it within MaxOutputBytes.
func Format(m Map) string {
	m = redactedMap(boundedMap(m))
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

// FormatSnapshot renders stable JSON while preserving the legacy map fields at
// the top level and placing exact provenance under "provenance".
func FormatSnapshot(snapshot Snapshot) string {
	snapshot = redactedSnapshot(boundedSnapshot(snapshot))
	for {
		out, err := json.MarshalIndent(snapshotOutput{
			Map:        snapshot.Summary,
			Provenance: makeSnapshotProvenance(snapshot),
		}, "", "  ")
		if err != nil {
			return `{"version":1,"output_truncated":true}`
		}
		if len(out) <= MaxOutputBytes {
			return string(out)
		}
		snapshot.Summary.OutputTruncated = true
		if !dropOneSnapshot(&snapshot) {
			if len(snapshot.Summary.Root) > 128 {
				snapshot.Summary.Root = cleanValue(snapshot.Summary.Root[:128])
			}
			if len(snapshot.Root) > 128 {
				snapshot.Root = cleanValue(snapshot.Root[:128])
			}
			out, _ = json.Marshal(snapshotOutput{
				Map:        snapshot.Summary,
				Provenance: makeSnapshotProvenance(snapshot),
			})
			if len(out) > MaxOutputBytes {
				return `{"version":1,"output_truncated":true}`
			}
			return string(out)
		}
	}
}

func boundedSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Summary = boundedMap(snapshot.Summary)
	snapshot.Root = cleanValue(snapshot.Root)
	if snapshot.Root == "" {
		snapshot.Root = snapshot.Summary.Root
	}
	snapshot.GitRoot = cleanValue(snapshot.GitRoot)
	snapshot.Head = cleanValue(snapshot.Head)
	snapshot.DirtyPaths, snapshot.DirtyPathsTruncated = boundPaths(snapshot.DirtyPaths, snapshot.DirtyPathsTruncated)
	snapshot.ManifestPaths, snapshot.ManifestPathsTruncated = boundPaths(snapshot.ManifestPaths, snapshot.ManifestPathsTruncated)
	return snapshot
}

func boundPaths(values []string, truncated bool) ([]string, bool) {
	values = append([]string(nil), values...)
	for i := range values {
		values[i] = normalizeRelativePath(values[i])
	}
	values = sortedUnique(values)
	if len(values) > maxListItems {
		values = values[:maxListItems]
		truncated = true
	}
	return values, truncated
}

func dropOneSnapshot(snapshot *Snapshot) bool {
	if dropOne(&snapshot.Summary) {
		return true
	}
	if len(snapshot.DirtyPaths) > 0 {
		snapshot.DirtyPaths = snapshot.DirtyPaths[:len(snapshot.DirtyPaths)-1]
		snapshot.DirtyPathsTruncated = true
		return true
	}
	if len(snapshot.ManifestPaths) > 0 {
		snapshot.ManifestPaths = snapshot.ManifestPaths[:len(snapshot.ManifestPaths)-1]
		snapshot.ManifestPathsTruncated = true
		return true
	}
	return false
}

func makeSnapshotProvenance(snapshot Snapshot) snapshotProvenance {
	tree := "UNVERIFIED"
	if snapshot.HeadKnown && snapshot.DirtyKnown {
		if len(snapshot.DirtyPaths) == 0 && !snapshot.DirtyPathsTruncated {
			tree = "CLEAN"
		} else {
			tree = "DIRTY"
		}
	}
	return snapshotProvenance{
		Root:                   cleanValue(snapshot.Root),
		GitRoot:                cleanValue(snapshot.GitRoot),
		Head:                   cleanValue(snapshot.Head),
		HeadKnown:              snapshot.HeadKnown,
		Tree:                   tree,
		DirtyPathsKnown:        snapshot.DirtyKnown,
		DirtyPaths:             snapshot.DirtyPaths,
		DirtyPathsTruncated:    snapshot.DirtyPathsTruncated,
		ManifestPaths:          snapshot.ManifestPaths,
		ManifestPathsTruncated: snapshot.ManifestPathsTruncated,
	}
}

// Compare reports only captured metadata changes. It intentionally cannot
// establish that file contents are equal when the metadata is unchanged.
func Compare(before, after Snapshot) Delta {
	before = boundedSnapshot(before)
	after = boundedSnapshot(after)
	delta := Delta{
		RootChanged:          snapshotRoot(before) != snapshotRoot(after),
		HeadChanged:          before.HeadKnown != after.HeadKnown || before.Head != after.Head,
		DirtyPathsChanged:    before.DirtyKnown != after.DirtyKnown || before.DirtyPathsTruncated != after.DirtyPathsTruncated || !sameStrings(before.DirtyPaths, after.DirtyPaths),
		ManifestPathsChanged: before.ManifestPathsTruncated != after.ManifestPathsTruncated || !sameStrings(before.ManifestPaths, after.ManifestPaths),
	}
	delta.AddedDirtyPaths, delta.RemovedDirtyPaths = stringDifferences(before.DirtyPaths, after.DirtyPaths)
	delta.AddedManifestPaths, delta.RemovedManifestPaths = stringDifferences(before.ManifestPaths, after.ManifestPaths)
	delta.RequiresRefresh = delta.RootChanged || delta.HeadChanged || delta.DirtyPathsChanged || delta.ManifestPathsChanged
	return delta
}

func snapshotRoot(snapshot Snapshot) string {
	if snapshot.Root != "" {
		return filepath.Clean(snapshot.Root)
	}
	return filepath.Clean(snapshot.Summary.Root)
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func stringDifferences(before, after []string) (added, removed []string) {
	beforeSet := make(map[string]bool, len(before))
	afterSet := make(map[string]bool, len(after))
	for _, value := range before {
		beforeSet[value] = true
	}
	for _, value := range after {
		afterSet[value] = true
		if !beforeSet[value] {
			added = append(added, value)
		}
	}
	for _, value := range before {
		if !afterSet[value] {
			removed = append(removed, value)
		}
	}
	return sortedUnique(added), sortedUnique(removed)
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

func redactedMap(m Map) Map {
	m.Root = redact.Text(m.Root)
	m.Languages = redactList(m.Languages)
	m.Frameworks = redactList(m.Frameworks)
	m.PackageManagers = redactList(m.PackageManagers)
	m.Commands.Build = redactList(m.Commands.Build)
	m.Commands.Test = redactList(m.Commands.Test)
	m.Commands.Lint = redactList(m.Commands.Lint)
	m.Manifests = redactList(m.Manifests)
	m.SourceRoots = redactList(m.SourceRoots)
	m.TestRoots = redactList(m.TestRoots)
	m.Rules = redactList(m.Rules)
	m.Git.Branch = redact.Text(m.Git.Branch)
	m.Git.Head = redact.Text(m.Git.Head)
	return m
}

func redactedSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Summary = redactedMap(snapshot.Summary)
	snapshot.Root = redact.Text(snapshot.Root)
	snapshot.GitRoot = redact.Text(snapshot.GitRoot)
	snapshot.Head = redact.Text(snapshot.Head)
	snapshot.DirtyPaths = redactList(snapshot.DirtyPaths)
	snapshot.ManifestPaths = redactList(snapshot.ManifestPaths)
	return snapshot
}

func redactList(values []string) []string {
	values = append([]string(nil), values...)
	for i := range values {
		values[i] = redact.Text(values[i])
	}
	return values
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
	cmd := gitobs.Command(cmdCtx, root, "ls-files", "-co", "--exclude-standard", "-z")
	cmd.Stderr = io.Discard
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

func absoluteDirectory(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", errors.New("repo map root is not a directory")
	}
	return abs, nil
}

func gitRootFor(ctx context.Context, root string) (string, bool) {
	value, err := commandText(ctx, root, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(root, value)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return "", false
	}
	return filepath.Clean(abs), true
}

func validCommitID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func captureGit(ctx context.Context, workspace string) captureGitResult {
	gitRoot, ok := gitRootFor(ctx, workspace)
	if !ok {
		return captureGitResult{}
	}
	captured := captureGitResult{
		root:  gitRoot,
		state: GitState{Repository: true},
	}
	status, err := commandText(ctx, gitRoot, "git", "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all")
	if err != nil {
		return captured
	}
	parsed := parseGitStatusV2(status, workspace, gitRoot)
	// Keep the bounded summary compatible with Inspect: its untracked count
	// intentionally follows Git's normal directory-level presentation, while
	// Capture still uses the all-files status above for precise dirty paths.
	if summary, summaryErr := commandText(ctx, gitRoot, "git", "status", "--porcelain=v1", "--untracked-files=normal"); summaryErr == nil {
		legacy := parseGitStatusV1(summary)
		legacy.Branch = parsed.state.Branch
		legacy.Head = parsed.state.Head
		parsed.state = legacy
	}
	if shortHead, shortErr := commandText(ctx, gitRoot, "git", "rev-parse", "--short=12", "HEAD"); shortErr == nil {
		parsed.state.Head = strings.TrimSpace(shortHead)
	}
	captured.state = parsed.state
	captured.head = parsed.head
	captured.headKnown = parsed.headKnown
	captured.dirtyKnown = true
	captured.dirtyPaths = parsed.dirtyPaths
	captured.dirtyPathsTruncated = parsed.dirtyPathsTruncated
	return captured
}

func parseGitStatusV2(status, workspace, gitRoot string) parsedGitStatus {
	parsed := parsedGitStatus{state: GitState{Repository: true, Clean: true}}
	paths := make(map[string]bool)
	for i, records := 0, strings.Split(status, "\x00"); i < len(records); i++ {
		record := records[i]
		if record == "" {
			continue
		}
		switch {
		case strings.HasPrefix(record, "# branch.oid "):
			head := strings.TrimSpace(strings.TrimPrefix(record, "# branch.oid "))
			if validCommitID(head) {
				parsed.head = head
				parsed.headKnown = true
				parsed.state.Head = shortCommitID(head)
			}
		case strings.HasPrefix(record, "# branch.head "):
			branch := strings.TrimSpace(strings.TrimPrefix(record, "# branch.head "))
			if !strings.HasPrefix(branch, "(") {
				parsed.state.Branch = branch
			}
		case record[0] == '?':
			parsed.state.Clean = false
			parsed.state.Untracked++
			addWorkspacePath(paths, workspace, gitRoot, strings.TrimPrefix(record, "? "))
		case record[0] == '1' || record[0] == '2' || record[0] == 'u':
			parsed.state.Clean = false
			fieldCount := 9
			if record[0] == '2' || record[0] == 'u' {
				fieldCount = 10
			}
			fields := strings.SplitN(record, " ", fieldCount)
			if len(fields) != fieldCount || len(fields[1]) < 2 {
				continue
			}
			xy := fields[1]
			if xy[0] != '.' {
				parsed.state.Staged++
			}
			if xy[1] != '.' {
				parsed.state.Modified++
			}
			addWorkspacePath(paths, workspace, gitRoot, fields[fieldCount-1])
			if record[0] == '2' && i+1 < len(records) {
				i++
				addWorkspacePath(paths, workspace, gitRoot, records[i])
			}
		}
	}
	values := make([]string, 0, len(paths))
	for path := range paths {
		values = append(values, path)
	}
	parsed.dirtyPaths, parsed.dirtyPathsTruncated = boundPaths(values, len(values) > maxListItems)
	return parsed
}

func shortCommitID(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func parseGitStatusV1(status string) GitState {
	g := GitState{Repository: true}
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

func addWorkspacePath(paths map[string]bool, workspace, gitRoot, path string) {
	path = filepath.FromSlash(path)
	if filepath.IsAbs(path) {
		// Git's -z status output is normally relative, but do not allow an
		// unexpected absolute path to escape the workspace projection.
		return
	}
	full := filepath.Join(gitRoot, path)
	comparisonWorkspace := workspace
	if resolved, err := filepath.EvalSymlinks(workspace); err == nil {
		comparisonWorkspace = resolved
	}
	relative, err := filepath.Rel(comparisonWorkspace, full)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return
	}
	if normalized := normalizeRelativePath(relative); normalized != "" {
		paths[normalized] = true
	}
}

func manifestPaths(files []string) ([]string, bool) {
	paths := make([]string, 0)
	truncated := false
	for _, file := range files {
		relative := normalizeRelativePath(file)
		if relative == "" || excludedManifestPath(relative) || !manifestNames[filepath.Base(relative)] {
			continue
		}
		if strings.Count(relative, "/")+1 > maxManifestDepth {
			truncated = true
			continue
		}
		paths = append(paths, relative)
	}
	paths = sortedUnique(paths)
	if len(paths) > maxListItems {
		paths = paths[:maxListItems]
		truncated = true
	}
	return paths, truncated
}

func excludedManifestPath(path string) bool {
	parts := strings.Split(path, "/")
	for _, part := range parts[:len(parts)-1] {
		if ignoredDir(part) {
			return true
		}
	}
	return false
}

func normalizeRelativePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return ""
	}
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if path == "." || path == ".." || strings.HasPrefix(path, "../") {
		return ""
	}
	return path
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
	state := parseGitStatusV1(status)
	state.Branch = g.Branch
	state.Head = g.Head
	return state
}

func commandText(ctx context.Context, root, name string, args ...string) (string, error) {
	cmdCtx, cancel := shortContext(ctx)
	defer cancel()
	if name != "git" {
		return "", errors.New("unsupported repository command")
	}
	result, err := gitobs.Output(cmdCtx, root, args...)
	if result.Truncated {
		return result.Output, errors.New("git output truncated")
	}
	return result.Output, err
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
