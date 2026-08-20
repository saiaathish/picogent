package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/projects"
)

// Event is one durable, ordered log line. Seq is monotonic per file.
type Event struct {
	Seq    int    `json:"seq"`
	TS     string `json:"ts"`
	Kind   string `json:"kind"`
	Tool   string `json:"tool,omitempty"`
	OK     *bool  `json:"ok,omitempty"`
	Detail string `json:"detail,omitempty"`
	MS     int64  `json:"ms,omitempty"`
}

// Log appends JSONL events under ~/.picogent/logs/.
type Log struct {
	mu   sync.Mutex
	path string
	seq  int
	Now  func() time.Time
}

func dir() (string, error) {
	home, err := config.Dir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(home, "logs")
	return d, os.MkdirAll(d, 0o700)
}

// Open creates or resumes the log for a workspace.
func Open(workspace string) (*Log, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	id := projects.IDForPath(workspace)
	if id == "" {
		id = "default"
	}
	path := filepath.Join(d, id+".jsonl")
	seq := 0
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range splitLines(string(data)) {
			var ev Event
			if json.Unmarshal([]byte(line), &ev) == nil && ev.Seq > seq {
				seq = ev.Seq
			}
		}
	}
	return &Log{
		path: path,
		seq:  seq,
		Now:  func() time.Time { return time.Now().UTC() },
	}, nil
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Append writes one event. Nil log is a no-op.
func (l *Log) Append(kind, tool, detail string, ok *bool, ms int64) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	now := time.Now().UTC()
	if l.Now != nil {
		now = l.Now().UTC()
	}
	ev := Event{
		Seq:    l.seq,
		TS:     now.Format(time.RFC3339),
		Kind:   kind,
		Tool:   tool,
		OK:     ok,
		Detail: clip(detail, 240),
		MS:     ms,
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// Tail returns the last n events, oldest first.
func (l *Log) Tail(n int) []Event {
	if l == nil || n <= 0 {
		return nil
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil
	}
	lines := splitLines(string(data))
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := make([]Event, 0, len(lines))
	for _, line := range lines {
		var ev Event
		if json.Unmarshal([]byte(line), &ev) == nil {
			out = append(out, ev)
		}
	}
	return out
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func Bool(v bool) *bool { return &v }
