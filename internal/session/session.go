package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
)

type Session struct {
	ID        string        `json:"id"`
	Workspace string        `json:"workspace"`
	Messages  []llm.Message `json:"messages"`
}

func New(workspace string) *Session {
	return &Session{
		ID:        time.Now().UTC().Format("20060102-150405"),
		Workspace: workspace,
	}
}

func (s *Session) Path() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions", s.ID+".json"), nil
}

func (s *Session) Save() error {
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
