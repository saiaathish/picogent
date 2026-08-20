package mcpbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// LoadServers merges MCP config from user + project (later sources override).
// Sources: ~/.cursor/mcp.json → ~/.picogent/mcp.yaml → {workspace}/.cursor/mcp.json → {workspace}/.mcp.json
func LoadServers(workspace string) (map[string]ServerConfig, error) {
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
	if workspace != "" {
		ws, err := filepath.Abs(workspace)
		if err == nil {
			layers = append(layers,
				filepath.Join(ws, ".cursor", "mcp.json"),
				filepath.Join(ws, ".mcp.json"),
			)
		}
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
	data, err := os.ReadFile(path)
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
	data, err := os.ReadFile(path)
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
