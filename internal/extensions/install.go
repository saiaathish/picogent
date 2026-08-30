package extensions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/gitobs"
	"github.com/saiaathish/picogent/internal/mcpbridge"
)

const (
	maxSkillInstallBytes   = 16 << 20
	maxSkillInstallEntries = 512
	skillGitTimeout        = 2 * time.Minute
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
	before    *StateSnapshot
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
	skillRootHandle, _, _ := openSkillsRoot(false)
	if skillRootHandle != nil {
		defer skillRootHandle.Close()
	}
	for _, it := range Catalog() {
		switch it.Kind {
		case KindMCP:
			if it.MCP != nil {
				name := mcpServerName(it)
				if _, ok := servers[name]; ok {
					out[it.ID] = true
				}
			}
		case KindSkill:
			if it.SkillPath == "" {
				continue
			}
			if skillRootHandle != nil {
				if rel, pathErr := normalizeSkillPath(it.SkillPath); pathErr == nil {
					if valid, statErr := validSkillAtRoot(skillRootHandle, rel); statErr == nil && valid {
						out[it.ID] = true
					}
				}
			}
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
	before, err := CaptureState(workspace, []string{it.ID})
	if err != nil {
		return res, entry, err
	}
	entry.before = before
	rollbackInstall := func(installErr error) (InstallResult, UndoEntry, error) {
		rollbackErr := before.Restore()
		before.Close()
		entry.before = nil
		return res, entry, errors.Join(installErr, rollbackErr)
	}

	if strings.HasPrefix(it.ID, "claude:") {
		if err := ActivateClaudePlugin(strings.TrimPrefix(it.ID, "claude:")); err != nil {
			return rollbackInstall(err)
		}
		res.AuthNeeded = it.AuthRequired
		res.AuthHint = it.AuthHint
		res.Message = fmt.Sprintf("Activated Claude plugin %q", it.Name)
		return res, entry, nil
	}

	switch it.Kind {
	case KindMCP:
		if it.MCP == nil {
			return rollbackInstall(fmt.Errorf("mcp config missing for %s", it.ID))
		}
		name := mcpServerName(it)
		cfg := *it.MCP
		if len(cfg.Args) > 0 && cfg.Args[len(cfg.Args)-1] == "." && workspace != "" {
			cfg.Args = append(append([]string{}, cfg.Args[:len(cfg.Args)-1]...), workspace)
		}
		if err := mcpbridge.SaveServer(name, cfg); err != nil {
			return rollbackInstall(err)
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
			return rollbackInstall(err)
		}
		entry.SkillPath = dest
		res.Message = fmt.Sprintf("Installed skill %q", it.Name)
		return res, entry, nil

	case KindPlugin:
		return rollbackInstall(fmt.Errorf("plugin %q is not supported by this Picogent build", it.Name))

	default:
		return rollbackInstall(fmt.Errorf("unknown kind %q", it.Kind))
	}
}

// Undo reverses a prior install.
func Undo(entry UndoEntry) error {
	if entry.before != nil {
		if err := entry.before.Restore(); err != nil {
			return err
		}
		entry.before.Close()
		return nil
	}
	switch entry.Kind {
	case KindMCP:
		if entry.MCPName == "" {
			return fmt.Errorf("missing mcp name")
		}
		return mcpbridge.RemoveServer(entry.MCPName)
	case KindSkill:
		if entry.SkillPath != "" {
			if !filepath.IsAbs(entry.SkillPath) {
				return removeSkill(entry.SkillPath)
			}
			return removeSkillAbsolute(entry.SkillPath)
		}
		return nil
	case KindPlugin:
		if strings.HasPrefix(entry.ExtID, "claude:") {
			return removeClaudePluginMCP(strings.TrimPrefix(entry.ExtID, "claude:"))
		}
		return fmt.Errorf("plugin %q is not supported by this Picogent build", entry.ExtID)
	default:
		return fmt.Errorf("unknown kind %q", entry.Kind)
	}
}

func installSkill(it Item) (dest string, err error) {
	rel, err := normalizeSkillPath(it.SkillPath)
	if err != nil {
		return "", err
	}
	dest, err = skillDestination(it.SkillPath)
	if err != nil {
		return "", err
	}
	root, _, err := openSkillsRoot(true)
	if err != nil {
		return "", err
	}
	defer root.Close()
	created := false
	defer func() {
		if err != nil && created {
			if cleanupErr := root.RemoveAll(rel); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()
	if info, err := root.Lstat(rel); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("skill destination %q is a symbolic link", it.SkillPath)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("skill destination %q is not a directory", it.SkillPath)
		}
		valid, err := validSkillAtRoot(root, rel)
		if err != nil {
			return "", err
		}
		if !valid {
			return "", fmt.Errorf("skill destination %q is empty or missing a non-empty SKILL.md", it.SkillPath)
		}
		return dest, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if it.SkillRepo == "" || it.SkillPath == "" {
		return "", fmt.Errorf("skill source not configured")
	}
	repo := strings.TrimSuffix(strings.TrimSpace(it.SkillRepo), "/")
	if err := validateSkillRepo(repo); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "picogent-skill-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	gitCtx, cancel := context.WithTimeout(context.Background(), skillGitTimeout)
	defer cancel()
	result, err := gitobs.Combined(gitCtx, filepath.Dir(tmp), "clone", "--depth", "1", "--filter=blob:none", "--sparse", "--no-recurse-submodules", repo, tmp)
	if err != nil {
		return "", fmt.Errorf("clone skill: %w", err)
	}
	if result.Truncated {
		return "", fmt.Errorf("clone skill output exceeded the safety limit")
	}
	result, err = gitobs.Combined(gitCtx, tmp, "sparse-checkout", "set", filepath.ToSlash(rel))
	if err != nil {
		return "", fmt.Errorf("sparse checkout: %w", err)
	}
	if result.Truncated {
		return "", fmt.Errorf("sparse checkout output exceeded the safety limit")
	}
	src := filepath.Join(tmp, rel)
	if info, err := os.Lstat(src); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			return "", err
		}
		if err := root.Mkdir(rel, 0o755); err != nil {
			return "", fmt.Errorf("create skill destination %q: %w", it.SkillPath, err)
		}
		created = true
		skillMD := filepath.Join(rel, "SKILL.md")
		body := fmt.Sprintf("# %s\n\nSee %s/%s for the full skill.\n", it.Name, it.SkillRepo, it.SkillPath)
		if err := root.WriteFile(skillMD, []byte(body), 0o644); err != nil {
			return "", err
		}
		valid, err := validSkillAtRoot(root, rel)
		if err != nil {
			return "", err
		}
		if !valid {
			return "", fmt.Errorf("cloned skill %q is empty or missing a non-empty SKILL.md", it.SkillPath)
		}
		return dest, nil
	}
	if valid, err := validSkillSource(src); err != nil {
		return "", err
	} else if !valid {
		return "", fmt.Errorf("cloned skill %q is empty or missing a non-empty SKILL.md", it.SkillPath)
	}
	tmpRoot, err := os.OpenRoot(tmp)
	if err != nil {
		return "", err
	}
	defer tmpRoot.Close()
	sourceRoot, err := tmpRoot.OpenRoot(rel)
	if err != nil {
		return "", fmt.Errorf("open cloned skill %q: %w", it.SkillPath, err)
	}
	defer sourceRoot.Close()
	if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		return "", err
	}
	if err := root.Mkdir(rel, 0o755); err != nil {
		return "", fmt.Errorf("create skill destination %q: %w", it.SkillPath, err)
	}
	created = true
	if err := copyDir(root, sourceRoot, rel); err != nil {
		return "", err
	}
	valid, err := validSkillAtRoot(root, rel)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", fmt.Errorf("cloned skill %q is empty or missing a non-empty SKILL.md", it.SkillPath)
	}
	return dest, nil
}

func validateSkillRepo(repo string) error {
	if repo == "" || strings.HasPrefix(repo, "-") || strings.ContainsAny(repo, "\x00\r\n") {
		return fmt.Errorf("skill repository is invalid")
	}
	switch {
	case strings.HasPrefix(repo, "https://"),
		strings.HasPrefix(repo, "ssh://"),
		strings.HasPrefix(repo, "git@"),
		strings.HasPrefix(repo, "file://"):
		return nil
	default:
		return fmt.Errorf("skill repository must use HTTPS, SSH, or a local file URL")
	}
}

func validSkillSource(src string) (bool, error) {
	info, err := os.Lstat(src)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, nil
	}
	info, err = os.Lstat(filepath.Join(src, "SKILL.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() == 0 {
		return false, nil
	}
	return true, nil
}

func copyDir(root, src *os.Root, dst string) error {
	var totalBytes int64
	var totalEntries int
	return fs.WalkDir(src.FS(), ".", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		sourcePath := filepath.FromSlash(path)
		info, err := src.Lstat(sourcePath)
		if err != nil {
			return err
		}
		totalEntries++
		if totalEntries > maxSkillInstallEntries {
			return fmt.Errorf("skill contains more than %d entries", maxSkillInstallEntries)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill contains unsupported symbolic link %q", path)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("skill contains unsupported file type %q", path)
		}
		rel := sourcePath
		target := dst
		if rel != "." {
			target = filepath.Join(dst, rel)
		}
		if info.IsDir() {
			return root.MkdirAll(target, info.Mode().Perm())
		}
		remaining := int64(maxSkillInstallBytes) - totalBytes
		if remaining < 0 || info.Size() > remaining {
			return fmt.Errorf("skill exceeds the %d-byte install limit", maxSkillInstallBytes)
		}
		in, err := src.Open(sourcePath)
		if err != nil {
			return err
		}
		if err := root.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			_ = in.Close()
			return err
		}
		out, err := root.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = in.Close()
			return err
		}
		written, copyErr := io.Copy(out, io.LimitReader(in, remaining+1))
		if copyErr == nil && written > remaining {
			copyErr = fmt.Errorf("skill exceeds the %d-byte install limit", maxSkillInstallBytes)
		}
		if copyErr == nil {
			totalBytes += written
		}
		closeOutErr := out.Close()
		closeInErr := in.Close()
		return errors.Join(copyErr, closeOutErr, closeInErr)
	})
}
