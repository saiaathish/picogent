package mcpbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/securefile"
	"gopkg.in/yaml.v3"
)

// ServerConfig describes one MCP server (stdio or HTTP).
type ServerConfig struct {
	Command string            `yaml:"command" json:"command"`
	Args    []string          `yaml:"args" json:"args"`
	Env     map[string]string `yaml:"env" json:"env"`
	URL     string            `yaml:"url" json:"url"`
	Type    string            `yaml:"type" json:"type"`
	Enabled *bool             `yaml:"enabled" json:"enabled"`
}

// ServerSnapshot is the state of one server in Picogent's writable MCP
// layer. The effective configuration may also come from Cursor's read-only
// layer; restoring an absent snapshot therefore removes only Picogent's
// override and reveals that lower layer again.
type ServerSnapshot struct {
	Config  ServerConfig
	Present bool
}

func (s ServerConfig) enabled() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

type fileYAML struct {
	Servers map[string]ServerConfig `yaml:"servers"`
}

type fileCursor struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// LoadServers merges only user-owned MCP config (later sources override).
// Connecting a server can execute a command or contact an endpoint before an
// MCP tool reaches the permission gate, so workspace config is never autoloaded.
// Sources: ~/.cursor/mcp.json → ~/.picogent/mcp.yaml
func LoadServers(_ string) (map[string]ServerConfig, error) {
	home, err := picogentHome()
	if err != nil {
		return nil, err
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	merged := map[string]ServerConfig{}

	layers := []string{
		filepath.Join(userHome, ".cursor", "mcp.json"),
		filepath.Join(home, "mcp.yaml"),
	}

	for _, path := range layers {
		batch, err := loadFile(path)
		if err != nil {
			return nil, err
		}
		for name, srv := range batch {
			merged[name] = srv
		}
	}

	filtered := map[string]ServerConfig{}
	for name, srv := range merged {
		if srv.enabled() {
			filtered[name] = srv
		}
	}
	return filtered, nil
}

func loadFile(path string) (map[string]ServerConfig, error) {
	switch filepath.Ext(path) {
	case ".yaml", ".yml":
		return loadYAML(path)
	default:
		return loadCursorJSON(path)
	}
}

func loadYAML(path string) (map[string]ServerConfig, error) {
	data, err := securefile.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var f fileYAML
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.Servers == nil {
		return nil, nil
	}
	return f.Servers, nil
}

func loadCursorJSON(path string) (map[string]ServerConfig, error) {
	data, err := securefile.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var f fileCursor
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.MCPServers == nil {
		return nil, nil
	}
	return f.MCPServers, nil
}

func picogentHome() (string, error) {
	if v := os.Getenv("PICOGENT_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".picogent"), nil
}

func cloneServerConfig(cfg ServerConfig) ServerConfig {
	cfg.Args = append([]string(nil), cfg.Args...)
	if cfg.Env != nil {
		env := make(map[string]string, len(cfg.Env))
		for key, value := range cfg.Env {
			env[key] = value
		}
		cfg.Env = env
	}
	if cfg.Enabled != nil {
		enabled := *cfg.Enabled
		cfg.Enabled = &enabled
	}
	return cfg
}

func userServersPath() (string, error) {
	home, err := picogentHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "mcp.yaml"), nil
}

// withUserServers holds the MCP config lock across the complete read-modify-
// write transaction. The callback must not retain or mutate servers after it
// returns. Reads use the same lock so snapshot boundaries are consistent with
// mutations, while LoadServers can safely read the atomically replaced file
// without taking this lock.
func withUserServers(fn func(path string, servers map[string]ServerConfig) error) error {
	if fn == nil {
		return errors.New("mcp config callback is nil")
	}
	path, err := userServersPath()
	if err != nil {
		return err
	}
	if err := securefile.EnsureDir(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	unlock, err := acquireMCPConfigLock(path)
	if err != nil {
		return err
	}
	defer unlock()

	servers, err := loadYAML(path)
	if err != nil {
		return err
	}
	if servers == nil {
		servers = map[string]ServerConfig{}
	}
	return fn(path, servers)
}

func userServers() (string, map[string]ServerConfig, error) {
	var path string
	var servers map[string]ServerConfig
	err := withUserServers(func(lockedPath string, lockedServers map[string]ServerConfig) error {
		path = lockedPath
		servers = lockedServers
		return nil
	})
	return path, servers, err
}

// writeUserServers atomically replaces the config file. Callers must
// hold the lock returned by acquireMCPConfigLock for path.
func writeUserServers(path string, servers map[string]ServerConfig) error {
	if len(servers) == 0 {
		err := securefile.RemoveFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var f fileYAML
	f.Servers = servers
	data, err := yaml.Marshal(&f)
	if err != nil {
		return err
	}

	mode := os.FileMode(0o600)
	if info, statErr := os.Lstat(path); statErr == nil {
		// os.WriteFile preserves the mode of an existing file. Preserve that
		// behavior while replacing atomically; never copy a symlink's synthetic
		// 0777 mode into the new regular config file.
		if info.Mode()&os.ModeSymlink == 0 {
			mode = info.Mode().Perm()
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}

	return writeMCPFile(path, data, mode)
}

// SnapshotServer captures the writable Picogent entry for name.
func SnapshotServer(name string) (ServerSnapshot, error) {
	if strings.TrimSpace(name) == "" {
		return ServerSnapshot{}, errors.New("mcp server name is empty")
	}
	_, servers, err := userServers()
	if err != nil {
		return ServerSnapshot{}, err
	}
	cfg, ok := servers[name]
	return ServerSnapshot{Config: cloneServerConfig(cfg), Present: ok}, nil
}

// SnapshotServersWithPrefix captures every writable entry whose name starts
// with prefix. It is used for Claude plugins, which can contribute more than
// one MCP server under one activation.
func SnapshotServersWithPrefix(prefix string) (map[string]ServerSnapshot, error) {
	if strings.TrimSpace(prefix) == "" {
		return nil, errors.New("mcp server prefix is empty")
	}
	_, servers, err := userServers()
	if err != nil {
		return nil, err
	}
	out := make(map[string]ServerSnapshot)
	for name, cfg := range servers {
		if strings.HasPrefix(name, prefix) {
			out[name] = ServerSnapshot{Config: cloneServerConfig(cfg), Present: true}
		}
	}
	return out, nil
}

// RestoreServers restores individual entries without disturbing unrelated
// user configuration.
func RestoreServers(snapshots map[string]ServerSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	return withUserServers(func(path string, servers map[string]ServerConfig) error {
		for name, snapshot := range snapshots {
			if snapshot.Present {
				servers[name] = cloneServerConfig(snapshot.Config)
			} else {
				delete(servers, name)
			}
		}
		return writeUserServers(path, servers)
	})
}

// RestoreServersWithPrefix removes current entries under prefix and restores
// exactly the entries captured by SnapshotServersWithPrefix.
func RestoreServersWithPrefix(prefix string, snapshots map[string]ServerSnapshot) error {
	if strings.TrimSpace(prefix) == "" {
		return errors.New("mcp server prefix is empty")
	}
	return withUserServers(func(path string, servers map[string]ServerConfig) error {
		for name := range servers {
			if strings.HasPrefix(name, prefix) {
				delete(servers, name)
			}
		}
		for name, snapshot := range snapshots {
			if snapshot.Present {
				servers[name] = cloneServerConfig(snapshot.Config)
			}
		}
		return writeUserServers(path, servers)
	})
}

func sanitizePart(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '.' || r == '/':
			b.WriteByte('_')
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	if out == "" {
		return "x"
	}
	return out
}

func publicName(server, tool string) string {
	return "mcp_" + sanitizePart(server) + "_" + sanitizePart(tool)
}

// SaveServer writes or updates a server in ~/.picogent/mcp.yaml.
func SaveServer(name string, cfg ServerConfig) error {
	return withUserServers(func(path string, existing map[string]ServerConfig) error {
		existing[name] = cfg
		return writeUserServers(path, existing)
	})
}

// RemoveServer deletes a server from ~/.picogent/mcp.yaml.
func RemoveServer(name string) error {
	return withUserServers(func(path string, existing map[string]ServerConfig) error {
		delete(existing, name)
		return writeUserServers(path, existing)
	})
}

// RemoveServersWithPrefix deletes servers whose names start with prefix.
func RemoveServersWithPrefix(prefix string) error {
	return withUserServers(func(path string, existing map[string]ServerConfig) error {
		changed := false
		for name := range existing {
			if strings.HasPrefix(name, prefix) {
				delete(existing, name)
				changed = true
			}
		}
		if !changed {
			return nil
		}
		return writeUserServers(path, existing)
	})
}
