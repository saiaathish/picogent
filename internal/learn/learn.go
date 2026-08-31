package learn

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/redact"
	"github.com/saiaathish/picogent/internal/securefile"
	workspacefs "github.com/saiaathish/picogent/internal/workspace"
)

const (
	maxLearnBytes     = 256 << 10
	maxTrackedFiles   = 256
	maxTrackedTools   = 64
	maxTrackedPathLen = 512
	maxToolNameLen    = 96
	maxTestOutput     = 4000
)

// ErrRevisionConflict means a learning snapshot was saved after a newer
// snapshot had already been published. Learning is advisory, but stale
// callbacks must not overwrite another turn's counters or test evidence.
var ErrRevisionConflict = errors.New("learning state revision conflict")

type ChangeStat struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Count   int    `json:"count"`
}

type TestSnapshot struct {
	Passed  int       `json:"passed"`
	Failed  int       `json:"failed"`
	Skipped int       `json:"skipped"`
	Output  string    `json:"output,omitempty"`
	At      time.Time `json:"at"`
}

type Store struct {
	Workspace    string                `json:"workspace"`
	FilesRead    map[string]int        `json:"files_read"`
	FilesChanged map[string]ChangeStat `json:"files_changed"`
	ToolCounts   map[string]int        `json:"tool_counts"`
	Turns        int                   `json:"turns"`
	Searches     int                   `json:"searches"`
	LastTest     *TestSnapshot         `json:"last_test,omitempty"`
	Knowledge    int                   `json:"knowledge"`
	Overview     []string              `json:"overview"`
	UpdatedAt    time.Time             `json:"updated_at"`
	Revision     uint64                `json:"revision"`
}

func readPath(workspace string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "learn", projects.IDForPath(workspace)+".json"), nil
}

func storePath(workspace string) (string, error) {
	path, err := readPath(workspace)
	if err != nil {
		return "", err
	}
	if err := securefile.EnsureDir(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func Load(workspace string) (Store, error) {
	path, err := readPath(workspace)
	if err != nil {
		return Store{}, err
	}
	s := Store{
		Workspace:    workspace,
		FilesRead:    map[string]int{},
		FilesChanged: map[string]ChangeStat{},
		ToolCounts:   map[string]int{},
	}
	data, err := securefile.ReadFileLimited(path, maxLearnBytes)
	if err != nil {
		if os.IsNotExist(err) {
			s.Overview = defaultOverview(workspace)
			s.Knowledge = score(&s)
			return s, nil
		}
		return Store{}, err
	}
	// The probe preserves the no-creation behavior for a cold workspace. Once
	// a record exists, serialize the actual read with Save's lock and decode
	// only the locked snapshot.
	unlock, err := acquireLearnLock(path)
	if err != nil {
		return Store{}, err
	}
	defer unlock()
	data, err = securefile.ReadFileLimited(path, maxLearnBytes)
	if errors.Is(err, os.ErrNotExist) {
		s.Overview = defaultOverview(workspace)
		s.Knowledge = score(&s)
		return s, nil
	}
	if err != nil {
		return Store{}, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return Store{}, err
	}
	if s.FilesRead == nil {
		s.FilesRead = map[string]int{}
	}
	if s.FilesChanged == nil {
		s.FilesChanged = map[string]ChangeStat{}
	}
	if s.ToolCounts == nil {
		s.ToolCounts = map[string]int{}
	}
	s = boundStore(s)
	s.Overview = buildOverview(&s, workspace)
	s.Knowledge = score(&s)
	return s, nil
}

// Save atomically publishes s only when its revision still matches the
// current on-disk snapshot. A successful save advances s.Revision in place so
// a long-lived handler can safely persist later turns without reloading.
func Save(s *Store) error {
	if s == nil {
		return errors.New("learning store is nil")
	}
	candidate := boundStore(*s)
	path, err := storePath(candidate.Workspace)
	if err != nil {
		return err
	}
	unlock, err := acquireLearnLock(path)
	if err != nil {
		return err
	}
	defer unlock()

	data, readErr := securefile.ReadFileLimited(path, maxLearnBytes)
	exists := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	var current Store
	if exists {
		if err := json.Unmarshal(data, &current); err != nil {
			return err
		}
	}
	if !learnRevisionMatches(candidate, current, exists) {
		return fmt.Errorf("%w: expected %d, found %d", ErrRevisionConflict, candidate.Revision, current.Revision)
	}
	if candidate.Revision == ^uint64(0) {
		return errors.New("learning state revision exhausted")
	}
	candidate.Revision++
	candidate.UpdatedAt = time.Now().UTC()
	candidate.Overview = buildOverview(&candidate, candidate.Workspace)
	candidate.Knowledge = score(&candidate)
	data, err = json.MarshalIndent(candidate, "", "  ")
	if err != nil {
		return err
	}
	if err := securefile.WriteAtomic(path, data, 0o600); err != nil {
		return err
	}
	*s = candidate
	return nil
}

func learnRevisionMatches(candidate, current Store, exists bool) bool {
	if !exists {
		return candidate.Revision == 0
	}
	if candidate.Revision != current.Revision {
		return false
	}
	// Revision zero is also used by pre-revision files. Require their
	// timestamp as the compatibility token; a manually constructed empty
	// store must not overwrite an existing legacy snapshot.
	return candidate.Revision != 0 || (!candidate.UpdatedAt.IsZero() && candidate.UpdatedAt.Equal(current.UpdatedAt))
}

func (s *Store) RecordRead(path string) {
	if s == nil {
		return
	}
	path = boundedPath(path)
	if path == "" {
		return
	}
	if s.FilesRead == nil {
		s.FilesRead = map[string]int{}
	}
	if s.ToolCounts == nil {
		s.ToolCounts = map[string]int{}
	}
	s.FilesRead[path]++
	s.ToolCounts["read_file"]++
	s.FilesRead = topIntMap(s.FilesRead, maxTrackedFiles, maxTrackedPathLen)
	s.ToolCounts = topIntMap(s.ToolCounts, maxTrackedTools, maxToolNameLen)
}

func (s *Store) RecordSearch() {
	if s == nil {
		return
	}
	if s.ToolCounts == nil {
		s.ToolCounts = map[string]int{}
	}
	s.Searches++
	s.ToolCounts["search"]++
	s.ToolCounts = topIntMap(s.ToolCounts, maxTrackedTools, maxToolNameLen)
}

func (s *Store) RecordTool(name string) {
	if s == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if s.ToolCounts == nil {
		s.ToolCounts = map[string]int{}
	}
	name = clip(name, maxToolNameLen)
	s.ToolCounts[name]++
	s.ToolCounts = topIntMap(s.ToolCounts, maxTrackedTools, maxToolNameLen)
}

func (s *Store) RecordChange(path string, added, removed int) {
	if s == nil {
		return
	}
	path = boundedPath(path)
	if path == "" {
		return
	}
	if s.FilesChanged == nil {
		s.FilesChanged = map[string]ChangeStat{}
	}
	if s.ToolCounts == nil {
		s.ToolCounts = map[string]int{}
	}
	prev := s.FilesChanged[path]
	prev.Path = path
	prev.Added += added
	prev.Removed += removed
	prev.Count++
	s.FilesChanged[path] = prev
	s.ToolCounts["edit"]++
	s.FilesChanged = topChangeMap(s.FilesChanged, maxTrackedFiles, maxTrackedPathLen)
	s.ToolCounts = topIntMap(s.ToolCounts, maxTrackedTools, maxToolNameLen)
}

func (s *Store) RecordTurn() {
	s.Turns++
}

func (s *Store) RecordTest(passed, failed, skipped int, output string) {
	if s == nil {
		return
	}
	if s.ToolCounts == nil {
		s.ToolCounts = map[string]int{}
	}
	s.LastTest = &TestSnapshot{
		Passed:  passed,
		Failed:  failed,
		Skipped: skipped,
		Output:  boundedTestOutput(output),
		At:      time.Now().UTC(),
	}
	s.ToolCounts["test"]++
	s.ToolCounts = topIntMap(s.ToolCounts, maxTrackedTools, maxToolNameLen)
}

func boundStore(s Store) Store {
	s.FilesRead = topIntMap(s.FilesRead, maxTrackedFiles, maxTrackedPathLen)
	s.FilesChanged = topChangeMap(s.FilesChanged, maxTrackedFiles, maxTrackedPathLen)
	s.ToolCounts = topIntMap(s.ToolCounts, maxTrackedTools, maxToolNameLen)
	if s.LastTest != nil {
		s.LastTest.Output = boundedTestOutput(s.LastTest.Output)
	}
	return s
}

func boundedTestOutput(output string) string {
	return clip(redact.Text(output), maxTestOutput)
}

func boundedPath(path string) string {
	return clip(strings.TrimSpace(path), maxTrackedPathLen)
}

type intEntry struct {
	key string
	val int
}

func topIntMap(in map[string]int, limit, keyLimit int) map[string]int {
	if limit <= 0 {
		return map[string]int{}
	}
	entries := make([]intEntry, 0, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		key = clip(key, keyLimit)
		entries = append(entries, intEntry{key: key, val: value})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].val != entries[j].val {
			return entries[i].val > entries[j].val
		}
		return entries[i].key < entries[j].key
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	out := make(map[string]int, len(entries))
	for _, item := range entries {
		out[item.key] += item.val
	}
	return out
}

func topChangeMap(in map[string]ChangeStat, limit, keyLimit int) map[string]ChangeStat {
	if limit <= 0 {
		return map[string]ChangeStat{}
	}
	entries := make([]ChangeStat, 0, len(in))
	for key, stat := range in {
		key = clip(strings.TrimSpace(key), keyLimit)
		if key == "" {
			continue
		}
		stat.Path = key
		entries = append(entries, stat)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		left := entries[i].Added + entries[i].Removed
		right := entries[j].Added + entries[j].Removed
		if left != right {
			return left > right
		}
		return entries[i].Path < entries[j].Path
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	out := make(map[string]ChangeStat, len(entries))
	for _, stat := range entries {
		out[stat.Path] = stat
	}
	return out
}

func score(s *Store) int {
	n := len(s.FilesRead)*2 + len(s.FilesChanged)*5 + s.Turns*3 + s.Searches
	if s.LastTest != nil {
		n += 8
		if s.LastTest.Failed == 0 && s.LastTest.Passed > 0 {
			n += 12
		}
	}
	if n > 100 {
		return 100
	}
	return n
}

func defaultOverview(workspace string) []string {
	var out []string
	name := filepath.Base(workspace)
	out = append(out, name+" — not explored yet. Ask Picogent for an overview to start learning.")
	if fileReadable(workspace, "go.mod") {
		out = append(out, "Go module detected — run tests with go test ./...")
	}
	if fileReadable(workspace, "package.json") {
		out = append(out, "Node project detected — check package.json scripts for test commands")
	}
	if fileReadable(workspace, "README.md") {
		out = append(out, "README.md present — good starting point for context")
	}
	return out
}

func fileReadable(workspace, path string) bool {
	f, err := workspacefs.OpenRead(workspace, path)
	if err != nil {
		return false
	}
	return f.Close() == nil
}

func buildOverview(s *Store, workspace string) []string {
	if s.Turns == 0 && len(s.FilesRead) == 0 {
		return defaultOverview(workspace)
	}
	var out []string
	name := projects.NameFromPath(workspace)
	out = append(out, name+" — knowledge "+itoa(s.Knowledge)+"% from "+itoa(len(s.FilesRead))+" files read, "+itoa(len(s.FilesChanged))+" edited")

	type kv struct {
		k string
		v int
	}
	var reads []kv
	for p, c := range s.FilesRead {
		reads = append(reads, kv{p, c})
	}
	sort.Slice(reads, func(i, j int) bool { return reads[i].v > reads[j].v })
	if len(reads) > 0 {
		var parts []string
		for i := 0; i < len(reads) && i < 4; i++ {
			parts = append(parts, reads[i].k+" ("+itoa(reads[i].v)+"×)")
		}
		out = append(out, "Most explored: "+strings.Join(parts, ", "))
	}

	var changes []ChangeStat
	for _, c := range s.FilesChanged {
		changes = append(changes, c)
	}
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Added+changes[i].Removed > changes[j].Added+changes[j].Removed
	})
	if len(changes) > 0 {
		c := changes[0]
		out = append(out, "Largest edit: "+c.Path+" (+"+itoa(c.Added)+" −"+itoa(c.Removed)+")")
	}

	if s.LastTest != nil {
		t := s.LastTest
		status := "passed"
		if t.Failed > 0 {
			status = "has failures"
		}
		out = append(out, "Last test run: "+itoa(t.Passed)+" passed, "+itoa(t.Failed)+" failed — "+status)
	}

	for _, hint := range repoHints(workspace) {
		out = append(out, hint)
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func repoHints(workspace string) []string {
	var hints []string
	entries, err := workspacefs.ReadDir(workspace, ".")
	if err != nil {
		return hints
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name()+"/")
		}
	}
	sort.Strings(dirs)
	if len(dirs) > 0 {
		n := dirs
		if len(n) > 5 {
			n = n[:5]
		}
		hints = append(hints, "Top folders: "+strings.Join(n, ", "))
	}
	return hints
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
