package trace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/redact"
	"github.com/saiaathish/picogent/internal/securefile"
)

const (
	maxTraceBytes       = 256 << 10
	retainedTraceBytes  = maxTraceBytes * 3 / 4
	maxTraceKindBytes   = 80
	maxTraceToolBytes   = 128
	maxTraceDetailBytes = 240
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
	return d, securefile.EnsureDir(d, 0o700)
}

// Open creates or resumes the log for a workspace.
func Open(workspace string) (*Log, error) {
	d, err := dir()
	if err != nil {
		return nil, fmt.Errorf("prepare trace directory: %w", err)
	}
	id := projects.IDForPath(workspace)
	if id == "" {
		id = "default"
	}
	path := filepath.Join(d, id+".jsonl")
	// Bootstrap and serialize the lock before reading the log. Without this
	// first-use transaction, several fresh processes could race while creating
	// the lock entry itself and one could fail before Append had a chance to
	// coordinate the writers.
	unlock, err := acquireTraceLock(path)
	if err != nil {
		return nil, fmt.Errorf("lock trace log: %w", err)
	}
	defer func() { _ = unlock() }()
	seq := 0
	if data, err := readTail(path, maxTraceBytes); err == nil {
		for _, line := range splitLines(string(data)) {
			var ev Event
			if json.Unmarshal([]byte(line), &ev) == nil && ev.Seq > seq {
				seq = ev.Seq
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read trace log: %w", err)
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
	unlock, err := acquireTraceLock(l.path)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	data, readErr := readTail(l.path, maxTraceBytes)
	if readErr == nil {
		for _, line := range splitLines(string(data)) {
			var existing Event
			if json.Unmarshal([]byte(line), &existing) == nil && existing.Seq > l.seq {
				l.seq = existing.Seq
			}
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
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
		Detail: detail,
		MS:     ms,
	}
	ev = sanitizeEvent(ev)
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	data = append(data, b...)
	data = append(data, '\n')
	if len(data) > maxTraceBytes {
		data = retainTail(data, retainedTraceBytes)
	}
	// Append used to reopen the path with os.OpenFile after an Lstat/read
	// check. A same-UID symlink swap could redirect that write. Publish the
	// complete bounded log through securefile's descriptor-anchored atomic
	// writer instead.
	return securefile.WriteAtomic(l.path, data, 0o600)
}

// Tail returns the last n events, oldest first.
func (l *Log) Tail(n int) []Event {
	if l == nil || n <= 0 {
		return nil
	}
	data, err := readTail(l.path, maxTraceBytes)
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
			// Re-sanitize on read as well as append: older logs and externally
			// supplied log files must not bypass the current boundary.
			ev = sanitizeEvent(ev)
			out = append(out, ev)
		}
	}
	return out
}

func readTail(path string, limit int) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("trace read limit must be positive")
	}
	// Trace events are individually bounded, so a small overshoot lets us
	// recover the tail while still keeping untrusted log growth bounded. The
	// secure reader anchors the file descriptor and rejects symlink targets.
	data, err := securefile.ReadFileLimited(path, limit+(4<<10))
	if err != nil {
		return nil, err
	}
	return retainTail(data, limit), nil
}

func retainTail(data []byte, limit int) []byte {
	if limit <= 0 || len(data) <= limit {
		return data
	}
	data = data[len(data)-limit:]
	if index := bytes.IndexByte(data, '\n'); index >= 0 {
		return data[index+1:]
	}
	return nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func sanitizeEvent(ev Event) Event {
	ev.Kind = clip(redact.Text(ev.Kind), maxTraceKindBytes)
	ev.Tool = clip(redact.Text(ev.Tool), maxTraceToolBytes)
	ev.Detail = clip(redact.Text(ev.Detail), maxTraceDetailBytes)
	return ev
}

func Bool(v bool) *bool { return &v }
