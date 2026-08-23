package learn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/projects"
)

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
	Workspace    string            `json:"workspace"`
	FilesRead    map[string]int    `json:"files_read"`
	FilesChanged map[string]ChangeStat `json:"files_changed"`
	ToolCounts   map[string]int    `json:"tool_counts"`
	Turns        int               `json:"turns"`
	Searches     int               `json:"searches"`
	LastTest     *TestSnapshot     `json:"last_test,omitempty"`
	Knowledge    int               `json:"knowledge"`
	Overview     []string          `json:"overview"`
	UpdatedAt    time.Time         `json:"updated_at"`
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.Overview = defaultOverview(workspace)
			s.Knowledge = score(&s)
			return s, nil
		}
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
	s.Overview = buildOverview(&s, workspace)
	s.Knowledge = score(&s)
	return s, nil
}

func Save(s Store) error {
	path, err := storePath(s.Workspace)
	if err != nil {
		return err
	}
	s.UpdatedAt = time.Now().UTC()
	s.Overview = buildOverview(&s, s.Workspace)
	s.Knowledge = score(&s)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) RecordRead(path string) {
	if path == "" {
		return
	}
	s.FilesRead[path]++
	s.ToolCounts["read_file"]++
}

func (s *Store) RecordSearch() {
	s.Searches++
	s.ToolCounts["search"]++
}

func (s *Store) RecordTool(name string) {
	s.ToolCounts[name]++
}

func (s *Store) RecordChange(path string, added, removed int) {
	if path == "" {
		return
	}
	prev := s.FilesChanged[path]
	prev.Path = path
	prev.Added += added
	prev.Removed += removed
	prev.Count++
	s.FilesChanged[path] = prev
	s.ToolCounts["edit"]++
}

func (s *Store) RecordTurn() {
	s.Turns++
}

func (s *Store) RecordTest(passed, failed, skipped int, output string) {
	s.LastTest = &TestSnapshot{
		Passed:  passed,
		Failed:  failed,
		Skipped: skipped,
		Output:  clip(output, 4000),
		At:      time.Now().UTC(),
	}
	s.ToolCounts["test"]++
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
	if _, err := os.Stat(filepath.Join(workspace, "go.mod")); err == nil {
		out = append(out, "Go module detected — run tests with go test ./...")
	}
	if _, err := os.Stat(filepath.Join(workspace, "package.json")); err == nil {
		out = append(out, "Node project detected — check package.json scripts for test commands")
	}
	if _, err := os.Stat(filepath.Join(workspace, "README.md")); err == nil {
		out = append(out, "README.md present — good starting point for context")
	}
	return out
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
	entries, err := os.ReadDir(workspace)
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
