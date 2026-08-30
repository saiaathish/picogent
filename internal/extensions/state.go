package extensions

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/saiaathish/picogent/internal/mcpbridge"
)

const (
	maxSnapshotBytes   = 16 << 20
	maxSnapshotEntries = 512
)

// StateSnapshot is an in-memory rollback record for extension-owned external
// state. It deliberately captures only the writable MCP layer and the skill
// paths that an extension operation can touch; unrelated configuration is
// never replaced during restore.
type StateSnapshot struct {
	mu         sync.Mutex
	mcp        map[string]mcpbridge.ServerSnapshot
	skillPaths []*pathSnapshot
}

// CaptureState records the current external state for the supplied extension
// IDs before an install, activation, or cleanup mutation.
func CaptureState(_ string, ids []string) (*StateSnapshot, error) {
	snapshot := &StateSnapshot{
		mcp: make(map[string]mcpbridge.ServerSnapshot),
	}
	seenIDs := make(map[string]bool)
	seenSkills := make(map[string]bool)
	var totalBytes int
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seenIDs[id] {
			continue
		}
		seenIDs[id] = true

		if strings.HasPrefix(id, "claude:") {
			name := strings.TrimPrefix(id, "claude:")
			if name == "" {
				return nil, errors.New("claude plugin name is empty")
			}
			keys, err := claudeMCPServerKeys(name)
			if err != nil {
				return nil, err
			}
			for _, key := range keys {
				if _, ok := snapshot.mcp[key]; ok {
					continue
				}
				state, err := mcpbridge.SnapshotServer(key)
				if err != nil {
					return nil, err
				}
				snapshot.mcp[key] = state
			}
			continue
		}

		it := ByID(id)
		if it == nil {
			continue
		}
		switch it.Kind {
		case KindMCP:
			if it.MCP == nil {
				continue
			}
			name := mcpServerName(*it)
			if _, ok := snapshot.mcp[name]; ok {
				continue
			}
			state, err := mcpbridge.SnapshotServer(name)
			if err != nil {
				return nil, err
			}
			snapshot.mcp[name] = state
		case KindSkill:
			if it.SkillPath == "" {
				continue
			}
			path, err := skillDestination(it.SkillPath)
			if err != nil {
				return nil, err
			}
			if seenSkills[path] {
				continue
			}
			seenSkills[path] = true
			state, bytes, err := capturePath(path)
			if err != nil {
				return nil, err
			}
			totalBytes += bytes
			if totalBytes > maxSnapshotBytes {
				return nil, fmt.Errorf("skill rollback snapshot exceeds %d bytes", maxSnapshotBytes)
			}
			snapshot.skillPaths = append(snapshot.skillPaths, state)
		}
	}
	return snapshot, nil
}

// Restore returns all captured state to its pre-mutation contents.
func (s *StateSnapshot) Restore() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	if err := mcpbridge.RestoreServers(s.mcp); err != nil {
		errs = append(errs, err)
	}
	for _, state := range s.skillPaths {
		if err := state.restore(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Clone returns an independent rollback record. Snapshot data is immutable, but
// the explicit copy gives a caller its own ownership when an UndoEntry is
// retained while another copy is being restored.
func (s *StateSnapshot) Clone() *StateSnapshot {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := &StateSnapshot{
		mcp: make(map[string]mcpbridge.ServerSnapshot, len(s.mcp)),
	}
	for name, snapshot := range s.mcp {
		clone.mcp[name] = cloneServerSnapshot(snapshot)
	}
	clone.skillPaths = make([]*pathSnapshot, 0, len(s.skillPaths))
	for _, path := range s.skillPaths {
		clone.skillPaths = append(clone.skillPaths, clonePathSnapshot(path))
	}
	return clone
}

func cloneServerSnapshot(snapshot mcpbridge.ServerSnapshot) mcpbridge.ServerSnapshot {
	cfg := snapshot.Config
	cfg.Args = append([]string(nil), cfg.Args...)
	if cfg.Env != nil {
		cfg.Env = make(map[string]string, len(cfg.Env))
		for key, value := range snapshot.Config.Env {
			cfg.Env[key] = value
		}
	}
	if snapshot.Config.Enabled != nil {
		enabled := *snapshot.Config.Enabled
		cfg.Enabled = &enabled
	}
	snapshot.Config = cfg
	return snapshot
}

func clonePathSnapshot(path *pathSnapshot) *pathSnapshot {
	if path == nil {
		return nil
	}
	clone := *path
	clone.data = append([]byte(nil), path.data...)
	clone.entries = make([]snapshotEntry, len(path.entries))
	for i, entry := range path.entries {
		clone.entries[i] = entry
		clone.entries[i].data = append([]byte(nil), entry.data...)
	}
	return &clone
}

// Clone returns an UndoEntry whose rollback snapshot has independent
// ownership. It is useful for callers that retain one entry while attempting
// a restore through another value copy.
func (e UndoEntry) Clone() UndoEntry {
	e.before = e.before.Clone()
	return e
}

// Close is kept for transaction-call-site compatibility. Snapshots are
// immutable and may be shallow-copied inside UndoEntry; clearing the pointed-to
// data here would invalidate those copies after a failed replacement. The Go
// garbage collector reclaims the snapshot once all entry copies are gone.
func (s *StateSnapshot) Close() {
}

type snapshotKind uint8

const (
	snapshotDir snapshotKind = iota + 1
	snapshotFile
	snapshotSymlink
)

type snapshotEntry struct {
	rel  string
	kind snapshotKind
	mode fs.FileMode
	data []byte
	link string
}

type pathSnapshot struct {
	path    string
	exists  bool
	kind    snapshotKind
	mode    fs.FileMode
	data    []byte
	link    string
	entries []snapshotEntry
}

func capturePath(path string) (*pathSnapshot, int, error) {
	state := &pathSnapshot{path: path}
	rel, err := skillRelativePath(path)
	if err != nil {
		return nil, 0, err
	}
	root, _, err := openSkillsRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return state, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer root.Close()
	info, err := root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return state, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	state.exists = true
	state.mode = info.Mode()
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return nil, 0, fmt.Errorf("skill destination %q is a symbolic link", path)
	case info.IsDir():
		state.kind = snapshotDir
		bytes := 0
		relSlash := filepath.ToSlash(rel)
		err = fs.WalkDir(root.FS(), relSlash, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if current == relSlash {
				return nil
			}
			currentOS := filepath.FromSlash(current)
			info, err := root.Lstat(currentOS)
			if err != nil {
				return err
			}
			entryRel, err := filepath.Rel(rel, currentOS)
			if err != nil {
				return err
			}
			if len(state.entries) >= maxSnapshotEntries {
				return fmt.Errorf("skill rollback snapshot exceeds %d entries", maxSnapshotEntries)
			}
			item := snapshotEntry{rel: entryRel, mode: info.Mode()}
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				return fmt.Errorf("skill contains unsupported symbolic link %q", entryRel)
			case info.IsDir():
				item.kind = snapshotDir
			case info.Mode().IsRegular():
				item.kind = snapshotFile
				item.data, err = readBoundedRoot(root, currentOS, maxSnapshotBytes-bytes)
				bytes += len(item.data)
			default:
				return fmt.Errorf("skill contains unsupported file type %s", rel)
			}
			if err != nil {
				return err
			}
			state.entries = append(state.entries, item)
			return nil
		})
		return state, bytes, err
	case info.Mode().IsRegular():
		state.kind = snapshotFile
		state.data, err = readBoundedRoot(root, rel, maxSnapshotBytes)
		return state, len(state.data), err
	default:
		return nil, 0, fmt.Errorf("skill destination has unsupported file type")
	}
}

func readBoundedRoot(root *os.Root, path string, limit int) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("rollback snapshot byte limit exceeded")
	}
	in, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	data, err := io.ReadAll(io.LimitReader(in, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("rollback snapshot exceeds %d bytes", limit)
	}
	return data, nil
}

func (s *pathSnapshot) restore() error {
	if s == nil {
		return nil
	}
	rel, err := skillRelativePath(s.path)
	if err != nil {
		return err
	}
	root, _, err := openSkillsRoot(true)
	if err != nil {
		return err
	}
	defer root.Close()
	if !s.exists {
		if err := root.RemoveAll(rel); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := root.RemoveAll(rel); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	switch s.kind {
	case snapshotSymlink:
		if err := validateSkillSymlinkTarget(rel, s.link); err != nil {
			return err
		}
		if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			return err
		}
		return root.Symlink(s.link, rel)
	case snapshotFile:
		if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			return err
		}
		if err := root.WriteFile(rel, s.data, s.mode.Perm()); err != nil {
			return err
		}
		return chmodRoot(root, rel, s.mode.Perm())
	case snapshotDir:
		if err := root.MkdirAll(rel, s.mode.Perm()); err != nil {
			return err
		}
		if err := chmodRoot(root, rel, s.mode.Perm()); err != nil {
			return err
		}
		dirs := append([]snapshotEntry(nil), s.entries...)
		sort.Slice(dirs, func(i, j int) bool {
			return strings.Count(dirs[i].rel, string(filepath.Separator)) < strings.Count(dirs[j].rel, string(filepath.Separator))
		})
		for _, entry := range dirs {
			if entry.kind != snapshotDir {
				continue
			}
			target := filepath.Join(rel, entry.rel)
			if err := root.MkdirAll(target, entry.mode.Perm()); err != nil {
				return err
			}
			if err := chmodRoot(root, target, entry.mode.Perm()); err != nil {
				return err
			}
		}
		for _, entry := range s.entries {
			if entry.kind == snapshotDir {
				continue
			}
			target := filepath.Join(rel, entry.rel)
			if err := root.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			switch entry.kind {
			case snapshotSymlink:
				if err := validateSkillSymlinkTarget(target, entry.link); err != nil {
					return err
				}
				if err := root.Symlink(entry.link, target); err != nil {
					return err
				}
			case snapshotFile:
				if err := root.WriteFile(target, entry.data, entry.mode.Perm()); err != nil {
					return err
				}
				if err := chmodRoot(root, target, entry.mode.Perm()); err != nil {
					return err
				}
			}
		}
		return nil
	default:
		return errors.New("invalid rollback snapshot")
	}
}

func skillDestination(skillPath string) (string, error) {
	clean, err := normalizeSkillPath(skillPath)
	if err != nil {
		return "", err
	}
	root, err := skillRoot()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(root, clean)
	if err := validateSkillAbsolutePath(dest); err != nil {
		return "", err
	}
	return dest, nil
}

// SkillConfigPath returns the normalized path stored in Config.Extensions for
// a skill destination or catalog path. Undo entries keep the absolute
// destination so external rollback is independent of the current working
// directory; the GUI needs the corresponding relative path when removing the
// entry from configuration.
func SkillConfigPath(skillPath string) (string, error) {
	cleanInput := strings.TrimSpace(strings.ReplaceAll(skillPath, "\\", "/"))
	if cleanInput == "" {
		return "", errors.New("skill path is empty")
	}
	rel := filepath.FromSlash(cleanInput)
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		root, err := skillRoot()
		if err != nil {
			return "", err
		}
		if err := validateSkillAbsolutePath(rel); err != nil {
			return "", err
		}
		rel, err = filepath.Rel(root, filepath.Clean(rel))
		if err != nil {
			return "", err
		}
	} else {
		var err error
		cleanInput, err = normalizeSkillPath(cleanInput)
		if err != nil {
			return "", err
		}
		rel = cleanInput
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
		return "", fmt.Errorf("skill path %q escapes the skills directory", skillPath)
	}
	// Validate relative inputs too. This rejects parent/symlink escapes before
	// a caller can use the normalized value to mutate configuration or files.
	if _, err := skillDestination(filepath.ToSlash(clean)); err != nil {
		return "", err
	}
	return filepath.ToSlash(clean), nil
}

func skillRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cursor", "skills-cursor"), nil
}

// openSkillsRoot opens a descriptor-backed root for all skill reads and
// mutations. The returned path is only a display/configuration path; callers
// must use the Root for filesystem access so a checked parent cannot be
// swapped for a symlink between validation and use.
func openSkillsRoot(create bool) (*os.Root, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}
	homeRoot, err := os.OpenRoot(home)
	if err != nil {
		return nil, "", err
	}
	defer homeRoot.Close()
	rel := filepath.Join(".cursor", "skills-cursor")
	if create {
		if err := homeRoot.MkdirAll(rel, 0o755); err != nil {
			return nil, "", err
		}
	}
	root, err := homeRoot.OpenRoot(rel)
	if err != nil {
		return nil, "", err
	}
	return root, filepath.Join(home, rel), nil
}

func normalizeSkillPath(skillPath string) (string, error) {
	cleanInput := strings.TrimSpace(strings.ReplaceAll(skillPath, "\\", "/"))
	if cleanInput == "" {
		return "", errors.New("skill path is empty")
	}
	rel := filepath.FromSlash(cleanInput)
	clean := filepath.Clean(rel)
	if strings.ContainsRune(clean, 0) || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("skill path %q escapes the skills directory", skillPath)
	}
	return clean, nil
}

func skillRelativePath(path string) (string, error) {
	root, err := skillRoot()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("skill path %q is outside the skills directory", path)
	}
	return filepath.Clean(rel), nil
}

func chmodRoot(root *os.Root, path string, mode fs.FileMode) error {
	file, err := root.Open(path)
	if err != nil {
		return err
	}
	chmodErr := file.Chmod(mode.Perm())
	closeErr := file.Close()
	return errors.Join(chmodErr, closeErr)
}

func validateSkillSymlinkTarget(base, link string) error {
	target := filepath.FromSlash(link)
	if strings.TrimSpace(link) == "" || filepath.IsAbs(target) || filepath.VolumeName(target) != "" {
		return fmt.Errorf("skill symlink target %q is outside the skills directory", link)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(base), target))
	if resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) || filepath.IsAbs(resolved) {
		return fmt.Errorf("skill symlink target %q is outside the skills directory", link)
	}
	return nil
}

func validSkillAtRoot(root *os.Root, rel string) (bool, error) {
	info, err := root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, nil
	}
	skillMD := filepath.Join(rel, "SKILL.md")
	info, err = root.Lstat(skillMD)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, nil
	}
	body, err := readBoundedRoot(root, skillMD, 64<<10)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(body)) != "", nil
}

func validateSkillAbsolutePath(path string) error {
	root, err := skillRoot()
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("skill path %q is outside the skills directory", path)
	}
	return validateSkillParents(root, filepath.Dir(path))
}

func validateSkillParents(root, parent string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	rootRel, err := filepath.Rel(home, root)
	if err != nil || rootRel == ".." || strings.HasPrefix(rootRel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("skills root %q is outside the home directory", root)
	}
	rel, err := filepath.Rel(root, parent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("skill path %q is outside the skills directory", parent)
	}
	current := home
	if err := checkSkillParent(current); err != nil {
		return err
	}
	for _, part := range strings.Split(rootRel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill path contains symbolic link %q", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("skill path parent %q is not a directory", current)
		}
	}
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill path contains symbolic link %q", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("skill path parent %q is not a directory", current)
		}
	}
	return nil
}

func checkSkillParent(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("skill path contains symbolic link %q", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("skills root %q is not a directory", path)
	}
	return nil
}

func removeSkill(skillPath string) error {
	rel, err := normalizeSkillPath(skillPath)
	if err != nil {
		return err
	}
	root, _, err := openSkillsRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.RemoveAll(rel); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func removeSkillAbsolute(skillPath string) error {
	rel, err := skillRelativePath(skillPath)
	if err != nil {
		return err
	}
	return removeSkill(rel)
}
