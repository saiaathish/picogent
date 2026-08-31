package projects

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/securefile"
	"gopkg.in/yaml.v3"
)

const maxRegistryBytes = 128 << 10

// ErrRegistryChanged means a compare-and-swap registry update observed a
// different on-disk value than the caller admitted. Callers must not restore
// their stale snapshot over the newer registry.
var ErrRegistryChanged = errors.New("project registry changed")

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
	_, err = securefile.ReadFileLimited(path, maxRegistryBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{}, nil
		}
		return Registry{}, err
	}
	// Probe first to preserve the no-state-directory behavior. The locked
	// reread is the value that gets decoded and cannot race Save's publication.
	unlock, err := acquireRegistryLock(path)
	if err != nil {
		return Registry{}, err
	}
	defer unlock()
	return loadLocked(path)
}

func loadLocked(path string) (Registry, error) {
	data, err := securefile.ReadFileLimited(path, maxRegistryBytes)
	if errors.Is(err, os.ErrNotExist) {
		return Registry{}, nil
	}
	if err != nil {
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
	if err := securefile.EnsureDir(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	unlock, err := acquireRegistryLock(path)
	if err != nil {
		return err
	}
	defer unlock()
	return saveLocked(path, reg)
}

// SaveIfCurrent publishes next only when the registry still exactly matches
// expected. The read, comparison, and atomic write share the registry lock so
// a caller can safely roll back a failed multi-step operation without
// overwriting a newer mutation from another process.
func SaveIfCurrent(expected, next Registry) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	if err := securefile.EnsureDir(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	unlock, err := acquireRegistryLock(path)
	if err != nil {
		return err
	}
	defer unlock()
	current, err := loadLocked(path)
	if err != nil {
		return err
	}
	if !registryEqual(current, expected) {
		return ErrRegistryChanged
	}
	return saveLocked(path, next)
}

func registryEqual(left, right Registry) bool {
	if len(left.Projects) != len(right.Projects) {
		return false
	}
	if len(left.Projects) == 0 {
		return left.Current == right.Current
	}
	return left.Current == right.Current &&
		reflect.DeepEqual(left.Projects, right.Projects)
}

func saveLocked(path string, reg Registry) error {
	data, err := yaml.Marshal(reg)
	if err != nil {
		return err
	}
	if len(data) > maxRegistryBytes {
		return fmt.Errorf("%w: registry %q exceeds %d bytes", securefile.ErrReadLimit, path, maxRegistryBytes)
	}
	return securefile.WriteAtomic(path, data, 0o644)
}

// updateRegistry keeps a registry read/modify/write transaction under one
// process and kernel-backed lock. Save's atomic publication protects each
// document, but only this transaction boundary prevents two mutators from
// losing one another's changes between those publications.
func updateRegistry(mutate func(*Registry) error) (Registry, error) {
	if mutate == nil {
		return Registry{}, errors.New("registry mutation is required")
	}
	path, err := registryPath()
	if err != nil {
		return Registry{}, err
	}
	if err := securefile.EnsureDir(filepath.Dir(path), 0o700); err != nil {
		return Registry{}, err
	}
	unlock, err := acquireRegistryLock(path)
	if err != nil {
		return Registry{}, err
	}
	defer unlock()
	reg, err := loadLocked(path)
	if err != nil {
		return Registry{}, err
	}
	if err := mutate(&reg); err != nil {
		return Registry{}, err
	}
	if err := saveLocked(path, reg); err != nil {
		return Registry{}, err
	}
	return reg, nil
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
	var project Project
	reg, err := updateRegistry(func(reg *Registry) error {
		var err error
		*reg, project, err = ensureInRegistry(*reg, abs)
		return err
	})
	if err != nil {
		return Registry{}, Project{}, err
	}
	return reg, project, nil
}

func ensureInRegistry(reg Registry, abs string) (Registry, Project, error) {
	id := IDForPath(abs)
	now := time.Now().UTC()
	for i := range reg.Projects {
		if reg.Projects[i].ID == id || filepath.Clean(reg.Projects[i].Path) == abs {
			reg.Projects[i].Path = abs
			reg.Projects[i].LastOpened = now
			id = reg.Projects[i].ID
			reg.Current = id
			return reg, reg.Projects[i], nil
		}
	}
	project := Project{
		ID:         id,
		Name:       NameFromPath(abs),
		Path:       abs,
		Created:    now,
		LastOpened: now,
	}
	reg.Projects = append(reg.Projects, project)
	reg.Current = id
	return reg, project, nil
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
	var project Project
	if _, err := updateRegistry(func(reg *Registry) error {
		var err error
		*reg, project, err = addToRegistry(*reg, name, abs)
		return err
	}); err != nil {
		return Project{}, err
	}
	return project, nil
}

// PrepareAdd validates and applies an add/select operation to an in-memory
// registry. The returned registry is not persisted; callers that need to
// coordinate another state transition can save it after that transition has
// succeeded.
func PrepareAdd(name, path string) (Registry, Project, error) {
	abs, err := normalizePath(path)
	if err != nil {
		return Registry{}, Project{}, err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		if err != nil {
			return Registry{}, Project{}, err
		}
		return Registry{}, Project{}, os.ErrNotExist
	}
	reg, err := Load()
	if err != nil {
		return Registry{}, Project{}, err
	}
	return addToRegistry(reg, name, abs)
}

func addToRegistry(reg Registry, name, abs string) (Registry, Project, error) {
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
			return reg, reg.Projects[i], nil
		}
	}
	p := Project{ID: id, Name: name, Path: abs, Created: now, LastOpened: now}
	reg.Projects = append(reg.Projects, p)
	reg.Current = id
	return reg, p, nil
}

func Switch(id string) (Project, error) {
	var project Project
	if _, err := updateRegistry(func(reg *Registry) error {
		var err error
		*reg, project, err = switchInRegistry(*reg, id)
		return err
	}); err != nil {
		return Project{}, err
	}
	return project, nil
}

// PrepareSwitch applies a select operation to an in-memory registry without
// persisting it. This is used by runtime owners that must commit the registry
// selection only after the replacement runtime is ready.
func PrepareSwitch(id string) (Registry, Project, error) {
	reg, err := Load()
	if err != nil {
		return Registry{}, Project{}, err
	}
	return switchInRegistry(reg, id)
}

func switchInRegistry(reg Registry, id string) (Registry, Project, error) {
	for i := range reg.Projects {
		if reg.Projects[i].ID == id {
			reg.Projects[i].LastOpened = time.Now().UTC()
			reg.Current = id
			return reg, reg.Projects[i], nil
		}
	}
	return Registry{}, Project{}, os.ErrNotExist
}

func Remove(id string) error {
	_, err := updateRegistry(func(reg *Registry) error {
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
		return nil
	})
	return err
}
