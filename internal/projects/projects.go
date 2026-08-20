package projects

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"gopkg.in/yaml.v3"
)

type Project struct {
	ID         string    `yaml:"id" json:"id"`
	Name       string    `yaml:"name" json:"name"`
	Path       string    `yaml:"path" json:"path"`
	Created    time.Time `yaml:"created" json:"created"`
	LastOpened time.Time `yaml:"last_opened" json:"last_opened"`
}

type Registry struct {
	Current  string    `yaml:"current"`
	Projects []Project `yaml:"projects"`
}

func registryPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "projects.yaml"), nil
}

func Load() (Registry, error) {
	path, err := registryPath()
	if err != nil {
		return Registry{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{}, nil
		}
		return Registry{}, err
	}
	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return Registry{}, err
	}
	return reg, nil
}

func Save(reg Registry) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(reg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func IDForPath(absPath string) string {
	h := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(h[:8])
}

func NameFromPath(absPath string) string {
	base := filepath.Base(absPath)
	if base == "." || base == "/" || base == "" {
		return "Project"
	}
	return base
}

func normalizePath(p string) (string, error) {
	if p == "" {
		return "", os.ErrInvalid
	}
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~/"))
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// Ensure registers the workspace if missing and marks it as current.
func Ensure(workspace string) (Registry, Project, error) {
	abs, err := normalizePath(workspace)
	if err != nil {
		return Registry{}, Project{}, err
	}
	reg, err := Load()
	if err != nil {
		return Registry{}, Project{}, err
	}
	id := IDForPath(abs)
	now := time.Now().UTC()
	var found *Project
	for i := range reg.Projects {
		if reg.Projects[i].ID == id || filepath.Clean(reg.Projects[i].Path) == abs {
			reg.Projects[i].Path = abs
			reg.Projects[i].LastOpened = now
			found = &reg.Projects[i]
			id = reg.Projects[i].ID
			break
		}
	}
	if found == nil {
		p := Project{
			ID:         id,
			Name:       NameFromPath(abs),
			Path:       abs,
			Created:    now,
			LastOpened: now,
		}
		reg.Projects = append(reg.Projects, p)
		found = &reg.Projects[len(reg.Projects)-1]
	}
	reg.Current = id
	if err := Save(reg); err != nil {
		return Registry{}, Project{}, err
	}
	return reg, *found, nil
}

func List() ([]Project, string, error) {
	reg, err := Load()
	if err != nil {
		return nil, "", err
	}
	sort.Slice(reg.Projects, func(i, j int) bool {
		return reg.Projects[i].LastOpened.After(reg.Projects[j].LastOpened)
	})
	return reg.Projects, reg.Current, nil
}

func Add(name, path string) (Project, error) {
	abs, err := normalizePath(path)
	if err != nil {
		return Project{}, err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		if err != nil {
			return Project{}, err
		}
		return Project{}, os.ErrNotExist
	}
	reg, err := Load()
	if err != nil {
		return Project{}, err
	}
	id := IDForPath(abs)
	now := time.Now().UTC()
	if name == "" {
		name = NameFromPath(abs)
	}
	for i := range reg.Projects {
		if reg.Projects[i].ID == id {
			reg.Projects[i].Name = name
			reg.Projects[i].LastOpened = now
			reg.Current = id
			if err := Save(reg); err != nil {
				return Project{}, err
			}
			return reg.Projects[i], nil
		}
	}
	p := Project{ID: id, Name: name, Path: abs, Created: now, LastOpened: now}
	reg.Projects = append(reg.Projects, p)
	reg.Current = id
	if err := Save(reg); err != nil {
		return Project{}, err
	}
	return p, nil
}

func Switch(id string) (Project, error) {
	reg, err := Load()
	if err != nil {
		return Project{}, err
	}
	for i := range reg.Projects {
		if reg.Projects[i].ID == id {
			reg.Projects[i].LastOpened = time.Now().UTC()
			reg.Current = id
			if err := Save(reg); err != nil {
				return Project{}, err
			}
			return reg.Projects[i], nil
		}
	}
	return Project{}, os.ErrNotExist
}

func Remove(id string) error {
	reg, err := Load()
	if err != nil {
		return err
	}
	out := reg.Projects[:0]
	var removed bool
	for _, p := range reg.Projects {
		if p.ID == id {
			removed = true
			continue
		}
		out = append(out, p)
	}
	if !removed {
		return os.ErrNotExist
	}
	reg.Projects = out
	if reg.Current == id {
		reg.Current = ""
		if len(reg.Projects) > 0 {
			reg.Current = reg.Projects[0].ID
		}
	}
	return Save(reg)
}
