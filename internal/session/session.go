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
	Workspace string        `json:"workspace"`
	Updated   time.Time     `json:"updated"`
	Messages  []llm.Message `json:"messages"`
}

func New(workspace string) *Session {
	now := time.Now().UTC()
	return &Session{
		ID:        now.Format("20060102-150405"),
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
	return s.Save()
}
