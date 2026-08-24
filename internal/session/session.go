package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	id := now.Format("20060102-150405")
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err == nil {
		id += "-" + hex.EncodeToString(suffix[:])
	} else {
		id += "-" + fmt.Sprintf("%x", now.UnixNano())
	}
	return &Session{
		ID:        id,
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
	if s == nil || !validID(s.ID) {
		return "", errors.New("invalid session id")
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, s.ID+".json"), nil
}

func (s *Session) Save() error {
	if s == nil || !validID(s.ID) {
		return errors.New("invalid session id")
	}
	path, err := s.Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	unlock, err := acquireSessionsLock(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer unlock()
	s.Updated = time.Now().UTC()
	return saveLocked(path, s)
}

func Load(id string) (*Session, error) {
	id = strings.TrimSuffix(strings.TrimSpace(id), ".json")
	if !validID(id) {
		return nil, errors.New("invalid session id")
	}
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, id+".json")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return loadLocked(path, id)
}

func List() ([]Session, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	_, err = os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return listLocked(dir)
}

func listLocked(dir string) ([]Session, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		s, err := loadLocked(filepath.Join(dir, e.Name()), id)
		if err != nil {
			continue
		}
		out = append(out, *s)
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
	_, err = os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return listMetaLocked(dir, workspace, MaxSessions)
}

func listMetaLocked(dir, workspace string, limit int) ([]Meta, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	ws, _ := filepath.Abs(workspace)
	var out []Meta
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		s, err := loadLocked(filepath.Join(dir, e.Name()), id)
		if err != nil {
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
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func Prune(workspace string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	all, err := listMetaLocked(dir, workspace, 0)
	if err != nil {
		return err
	}
	if len(all) <= MaxSessions {
		return nil
	}
	for _, m := range all[MaxSessions:] {
		if validID(m.ID) {
			if err := os.Remove(filepath.Join(dir, m.ID+".json")); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func Delete(id string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	id = strings.TrimSuffix(id, ".json")
	if !validID(id) {
		return errors.New("invalid session id")
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return os.ErrNotExist
		}
		return err
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	return os.Remove(filepath.Join(dir, id+".json"))
}

func Latest(workspace string) (*Session, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	ws, _ := filepath.Abs(workspace)
	for _, s := range all {
		if !validID(s.ID) {
			continue
		}
		sw, _ := filepath.Abs(s.Workspace)
		if sw == ws && len(s.Messages) > 0 {
			cp := s
			return &cp, nil
		}
	}
	return nil, os.ErrNotExist
}

func SaveMessages(workspace string, id string, msgs []llm.Message) error {
	return saveMessagesWithTitle(workspace, id, msgs, "")
}

func loadLocked(path, id string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.ID != id {
		return nil, errors.New("session id mismatch")
	}
	return &s, nil
}

func saveLocked(path string, s *Session) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

func updateSession(id, workspace string, create bool, mutate func(*Session) error) (*Session, error) {
	id = strings.TrimSuffix(strings.TrimSpace(id), ".json")
	if !validID(id) {
		return nil, errors.New("invalid session id")
	}
	if mutate == nil {
		return nil, errors.New("session update callback is required")
	}
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	path := filepath.Join(dir, id+".json")
	if !create {
		if _, err := os.Stat(path); err != nil {
			return nil, err
		}
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()

	s, err := loadLocked(path, id)
	if err != nil {
		if !create || !os.IsNotExist(err) {
			return nil, err
		}
		s = &Session{ID: id, Title: "New chat", Workspace: workspace}
	}
	if s.Workspace == "" {
		s.Workspace = workspace
	}
	if err := mutate(s); err != nil {
		return nil, err
	}
	s.ID = id
	s.Updated = time.Now().UTC()
	if err := saveLocked(path, s); err != nil {
		return nil, err
	}
	return s, nil
}

func validID(id string) bool {
	if id == "" || id == "." || id == ".." || len(id) > 200 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}
