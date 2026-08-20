package extensions

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/mcpbridge"
)

// InstallResult describes what was installed and how to undo it.
type InstallResult struct {
	ID         string `json:"id"`
	ExtID      string `json:"ext_id"`
	Name       string `json:"name"`
	Kind       Kind   `json:"kind"`
	UndoID     string `json:"undo_id"`
	MCPName    string `json:"mcp_name,omitempty"`
	AuthNeeded bool   `json:"auth_needed,omitempty"`
	AuthHint   string `json:"auth_hint,omitempty"`
	Message    string `json:"message,omitempty"`
}

// UndoEntry records a reversible install action.
type UndoEntry struct {
	ID        string    `json:"id"`
	ExtID     string    `json:"ext_id"`
	Kind      Kind      `json:"kind"`
	MCPName   string    `json:"mcp_name,omitempty"`
	SkillPath string    `json:"skill_path,omitempty"`
	At        time.Time `json:"at"`
}

// InstalledSet returns which catalog IDs are currently installed.
func InstalledSet(workspace string, cfgSkills []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, s := range cfgSkills {
		out["skill:"+s] = true
	}
	servers, err := mcpbridge.LoadServers(workspace)
	if err != nil {
		return out, err
	}
	for _, it := range Catalog() {
		if it.Kind != KindMCP || it.MCP == nil {
			continue
		}
		name := mcpServerName(it)
		if _, ok := servers[name]; ok {
			out[it.ID] = true
		}
	}
	return out, nil
}

// ActivateMCPCatalog writes a catalog MCP server to config.
func ActivateMCPCatalog(it Item) error {
	if it.MCP == nil {
		return fmt.Errorf("no mcp config")
	}
	return mcpbridge.SaveServer(mcpServerName(it), *it.MCP)
}

func mcpServerName(it Item) string {
	return strings.TrimPrefix(it.ID, "mcp-")
}

// MCPServerName returns the MCP config key for a catalog item.
func MCPServerName(it Item) string {
	return mcpServerName(it)
}

// Install adds an extension. workspace is used for MCP path args when needed.
func Install(it Item, workspace string) (InstallResult, UndoEntry, error) {
	undoID := fmt.Sprintf("undo-%d", time.Now().UnixNano())
	res := InstallResult{
		ExtID: it.ID, Name: it.Name, Kind: it.Kind, UndoID: undoID,
	}
	entry := UndoEntry{ID: undoID, ExtID: it.ID, Kind: it.Kind, At: time.Now()}

	switch it.Kind {
	case KindMCP:
		if it.MCP == nil {
			return res, entry, fmt.Errorf("mcp config missing for %s", it.ID)
		}
		name := mcpServerName(it)
		cfg := *it.MCP
		if len(cfg.Args) > 0 && cfg.Args[len(cfg.Args)-1] == "." && workspace != "" {
			cfg.Args = append(append([]string{}, cfg.Args[:len(cfg.Args)-1]...), workspace)
		}
		if err := mcpbridge.SaveServer(name, cfg); err != nil {
			return res, entry, err
		}
		entry.MCPName = name
		res.MCPName = name
		res.AuthNeeded = it.AuthRequired
		res.AuthHint = it.AuthHint
		res.Message = fmt.Sprintf("Installed MCP server %q", it.Name)
		return res, entry, nil

	case KindSkill:
		dest, err := installSkill(it)
		if err != nil {
			return res, entry, err
		}
		entry.SkillPath = dest
		res.Message = fmt.Sprintf("Installed skill %q", it.Name)
		return res, entry, nil

	case KindPlugin:
		res.Message = fmt.Sprintf("Enabled plugin %q", it.Name)
		return res, entry, nil

	default:
		return res, entry, fmt.Errorf("unknown kind %q", it.Kind)
	}
}

// Undo reverses a prior install.
func Undo(entry UndoEntry) error {
	switch entry.Kind {
	case KindMCP:
		if entry.MCPName == "" {
			return fmt.Errorf("missing mcp name")
		}
		return mcpbridge.RemoveServer(entry.MCPName)
	case KindSkill:
		if entry.SkillPath != "" {
			return os.RemoveAll(entry.SkillPath)
		}
		return nil
	case KindPlugin:
		return nil
	default:
		return fmt.Errorf("unknown kind %q", entry.Kind)
	}
}

func installSkill(it Item) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	skillsRoot := filepath.Join(home, ".cursor", "skills-cursor")
	if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(skillsRoot, it.SkillPath)
	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}
	if it.SkillRepo == "" || it.SkillPath == "" {
		return "", fmt.Errorf("skill source not configured")
	}
	tmp, err := os.MkdirTemp("", "picogent-skill-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	repo := strings.TrimSuffix(it.SkillRepo, "/")
	cmd := exec.Command("git", "clone", "--depth", "1", "--filter=blob:none", "--sparse", repo, tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("clone skill: %w: %s", err, string(out))
	}
	sparse := exec.Command("git", "-C", tmp, "sparse-checkout", "set", it.SkillPath)
	if out, err := sparse.CombinedOutput(); err != nil {
		return "", fmt.Errorf("sparse checkout: %w: %s", err, string(out))
	}
	src := filepath.Join(tmp, it.SkillPath)
	if _, err := os.Stat(src); err != nil {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return "", err
		}
		skillMD := filepath.Join(dest, "SKILL.md")
		body := fmt.Sprintf("# %s\n\nSee %s/%s for the full skill.\n", it.Name, it.SkillRepo, it.SkillPath)
		if err := os.WriteFile(skillMD, []byte(body), 0o644); err != nil {
			return "", err
		}
		return dest, nil
	}
	if err := copyDir(src, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}
