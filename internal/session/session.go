package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
)

type Session struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Workspace string        `json:"workspace"`
	Updated   time.Time     `json:"updated"`
	Messages  []llm.Message `json:"messages"`
}

const MaxSessions = 60

func deriveTitle(msgs []llm.Message) string {
	for _, m := range msgs {
		if m.Role == "user" {
			t := strings.TrimSpace(m.Content)
			if t != "" {
				if len(t) > 56 {
					return t[:56] + "…"
				}
				return t
			}
		}
	}
	return "New chat"
}

// Meta is a lightweight session summary for list views (no message bodies).
type Meta struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Updated time.Time `json:"updated"`
}

func New(workspace string) *Session {
	now := time.Now().UTC()
	return &Session{
		ID:        now.Format("20060102-150405"),
		Title:     "New chat",
		Workspace: workspace,
		Updated:   now,
	}
}

func Dir() (string, error) {
	root, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "sessions"), nil
}

func (s *Session) Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, s.ID+".json"), nil
}

func (s *Session) Save() error {
	s.Updated = time.Now().UTC()
	path, err := s.Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func Load(id string) (*Session, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, id+".json")
	if !strings.HasSuffix(id, ".json") {
		path = filepath.Join(dir, id+".json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func List() ([]Session, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Session
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Updated.After(out[j].Updated)
	})
	return out, nil
}

func ListMeta(workspace string) ([]Meta, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	ws, _ := filepath.Abs(workspace)
	var out []Meta
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		sw, _ := filepath.Abs(s.Workspace)
		if ws != "" && sw != ws {
			continue
		}
		title := s.Title
		if title == "" {
			title = deriveTitle(s.Messages)
		}
		out = append(out, Meta{ID: s.ID, Title: title, Updated: s.Updated})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Updated.After(out[j].Updated)
	})
	if len(out) > MaxSessions {
		out = out[:MaxSessions]
	}
	return out, nil
}

func Prune(workspace string) error {
	all, err := ListMeta(workspace)
	if err != nil {
		return err
	}
	if len(all) <= MaxSessions {
		return nil
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	for _, m := range all[MaxSessions:] {
		_ = os.Remove(filepath.Join(dir, m.ID+".json"))
	}
	return nil
}

func Delete(id string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	id = strings.TrimSuffix(id, ".json")
	return os.Remove(filepath.Join(dir, id+".json"))
}

func Latest(workspace string) (*Session, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	ws, _ := filepath.Abs(workspace)
	for _, s := range all {
		sw, _ := filepath.Abs(s.Workspace)
		if sw == ws && len(s.Messages) > 0 {
			cp := s
			return &cp, nil
		}
	}
	return nil, os.ErrNotExist
}

func SaveMessages(workspace string, id string, msgs []llm.Message) error {
	s := &Session{ID: id, Workspace: workspace, Messages: msgs}
	if s.ID == "" {
		s = New(workspace)
	}
	s.Messages = msgs
	s.Title = deriveTitle(msgs)
	if err := s.Save(); err != nil {
		return err
	}
	return Prune(workspace)
}
