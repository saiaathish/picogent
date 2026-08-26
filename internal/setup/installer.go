package setup

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/config"
)

const (
	npmRegistry         = "https://registry.npmjs.org/"
	managedToolsDirName = "tools"
	managedBinDirName   = "node_modules/.bin"
	maxInstallerOutput  = 64 * 1024
)

// execLookPath is a test seam for adversarial setup tests. Production uses the
// standard PATH lookup, while every installer still receives an absolute path
// selected before it is executed.
var execLookPath = exec.LookPath

// trustedExecutableFn is a test seam for path-provenance cases. Production
// package-manager execution accepts only an absolute executable resolved under
// an OS-appropriate system or user-managed runtime prefix.
var trustedExecutableFn = trustedExternalExecutable

var setupElevatedFn = setupRunningElevated

var (
	// These files are reviewed source artifacts, not user-provided npm state.
	// They are materialized into the private tools prefix before npm runs.
	//
	//go:embed provider-package.json
	providerPackageJSON []byte
	//go:embed provider-package-lock.json
	providerPackageLockJSON []byte
)

type npmInstallSpec struct {
	Label   string
	Package string
	Binary  string
}

var (
	codexNPMSpec = npmInstallSpec{
		Label:   "Codex CLI",
		Package: "@openai/codex@0.149.1",
		Binary:  "codex",
	}
	claudeNPMSpec = npmInstallSpec{
		Label:   "Claude Code CLI",
		Package: "@anthropic-ai/claude-code@2.1.246",
		Binary:  "claude",
	}
)

// runInstaller is deliberately narrow: setup may execute only the explicitly
// resolved package-manager binary with a caller-provided argument vector and a
// sanitized environment. Tests replace it to prove the command boundary
// without contacting a registry.
var runInstaller = runTimedWithEnv

func installNPM(spec npmInstallSpec, say func(string)) error {
	if !allowedNPMInstall(spec) {
		return fmt.Errorf("refusing unapproved npm package %q", spec.Package)
	}
	if setupElevatedFn() {
		return fmt.Errorf("refusing automatic CLI installation from an elevated process; rerun Picogent as the normal user")
	}
	npm := externalLook("npm")
	if npm == "" {
		return fmt.Errorf("npm is not installed, cannot install %s", spec.Label)
	}

	root, _, err := prepareManagedTools()
	if err != nil {
		return fmt.Errorf("prepare private CLI directory: %w", err)
	}
	if err := rejectManagedNPMConfig(root); err != nil {
		return err
	}
	if err := writeProviderManifests(root); err != nil {
		return fmt.Errorf("write pinned provider manifest: %w", err)
	}
	userConfig, err := os.CreateTemp(root, ".npm-userconfig-")
	if err != nil {
		return fmt.Errorf("create npm user config: %w", err)
	}
	userConfigPath := userConfig.Name()
	if err := userConfig.Close(); err != nil {
		_ = os.Remove(userConfigPath)
		return fmt.Errorf("close npm user config: %w", err)
	}
	defer os.Remove(userConfigPath)

	globalConfig, err := os.CreateTemp(root, ".npm-globalconfig-")
	if err != nil {
		return fmt.Errorf("create npm global config: %w", err)
	}
	globalConfigPath := globalConfig.Name()
	if err := globalConfig.Close(); err != nil {
		_ = os.Remove(globalConfigPath)
		return fmt.Errorf("close npm global config: %w", err)
	}
	defer os.Remove(globalConfigPath)

	cacheDir := filepath.Join(root, "cache")
	if err := ensurePrivateDirectory(cacheDir); err != nil {
		return fmt.Errorf("prepare npm cache: %w", err)
	}

	// Keep installation user-local and prevent package lifecycle scripts from
	// gaining execution during first-run setup. The package names and registry
	// are compile-time allowlisted above; user npm config cannot redirect them.
	args := []string{
		"ci",
		"--prefix", root,
		"--ignore-scripts",
		"--no-audit",
		"--no-fund",
		"--no-update-notifier",
		"--registry", npmRegistry,
		"--userconfig", userConfigPath,
		"--globalconfig", globalConfigPath,
		"--cache", cacheDir,
	}
	say(fmt.Sprintf("installing pinned provider CLIs into Picogent's private CLI folder (npm registry %s; lockfile integrity enforced; lifecycle scripts disabled)", npmRegistry))
	out, err := runInstaller(4*time.Minute, npm, args, installerEnv(npm))
	if err != nil {
		if detail := redactInstallerOutput(out); detail != "" {
			return fmt.Errorf("%w\n%s", err, detail)
		}
		return err
	}

	path := managedBinaryPath(spec.Binary)
	if path == "" {
		return fmt.Errorf("npm finished but `%s` was not created inside Picogent's private CLI folder", spec.Binary)
	}
	say("ok  " + spec.Binary + " (private Picogent CLI folder)")
	return nil
}

func allowedNPMInstall(spec npmInstallSpec) bool {
	return spec == codexNPMSpec || spec == claudeNPMSpec
}

func trustedExternalExecutable(name, path string) string {
	if name == "" || path == "" || !filepath.IsAbs(path) {
		return ""
	}
	return trustedExecutableInRoots(path, trustedExecutableRoots())
}

func trustedExecutableInRoots(path string, roots []string) string {
	if path == "" || !filepath.IsAbs(path) {
		return ""
	}
	resolved, ok := canonicalPath(path)
	if !ok {
		return ""
	}
	st, err := os.Stat(resolved)
	if err != nil || st.IsDir() {
		return ""
	}
	if runtime.GOOS != "windows" {
		if st.Mode().Perm()&0o111 == 0 || st.Mode().Perm()&0o022 != 0 {
			return ""
		}
	}
	for _, root := range roots {
		if pathWithin(root, resolved) && executableAncestorsProtected(root, resolved) {
			return resolved
		}
	}
	return ""
}

func trustedManagedExecutable(path string) string {
	root, _, err := managedPaths()
	if err != nil {
		return ""
	}
	if !trustedManagedRoot(root) {
		return ""
	}
	return trustedExecutableInRoots(path, []string{root})
}

func trustedManagedRoot(root string) bool {
	home, err := config.Dir()
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	homeAbs, err := filepath.Abs(home)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(homeAbs), filepath.Clean(abs))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	for current := filepath.Clean(abs); ; current = filepath.Dir(current) {
		st, err := os.Lstat(current)
		if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return false
		}
		if runtime.GOOS != "windows" {
			if current == filepath.Clean(abs) && st.Mode().Perm()&0o077 != 0 {
				return false
			}
			if current != filepath.Clean(abs) && st.Mode().Perm()&0o022 != 0 {
				return false
			}
		}
		if samePath(current, filepath.Clean(homeAbs)) {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
	}
	return true
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

// validateLaunchExecutable repeats the trust decision immediately before a
// command is started. The executable is returned in canonical form so an
// already-validated symlink cannot be swapped at the managed PATH entry after
// lookup. This narrows pathname races; an OS-level attacker that can rewrite a
// trusted executable between this check and CreateProcess/execve remains out
// of scope for this portable path-based launcher.
func validateLaunchExecutable(path string) (string, error) {
	if resolved := trustedManagedExecutable(path); resolved != "" {
		return resolved, nil
	}
	if resolved := trustedExternalExecutable(filepath.Base(path), path); resolved != "" {
		return resolved, nil
	}
	return "", fmt.Errorf("executable is not trusted: %q", path)
}

func trustedExecutableRoots() []string {
	if runtime.GOOS == "windows" {
		roots := []string{os.Getenv("SystemRoot"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA")}
		return nonEmptyPaths(roots)
	}
	roots := []string{
		"/usr/bin", "/usr/local/bin", "/usr/local/lib", "/opt/homebrew", "/opt/local/bin", "/opt/local/lib",
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots,
			filepath.Join(home, ".nvm"), filepath.Join(home, ".volta"), filepath.Join(home, ".asdf"),
			filepath.Join(home, ".bun"), filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".opencode", "bin"), filepath.Join(home, ".npm-global", "bin"),
		)
	}
	return nonEmptyPaths(roots)
}

func nonEmptyPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			out = append(out, path)
		}
	}
	return out
}

func prepareManagedTools() (root, bin string, err error) {
	home, err := config.Dir()
	if err != nil {
		return "", "", err
	}
	if err := ensurePrivateDirectory(home); err != nil {
		return "", "", err
	}
	root = filepath.Join(home, managedToolsDirName)
	if err := ensurePrivateDirectory(root); err != nil {
		return "", "", err
	}
	nodeModules := filepath.Join(root, "node_modules")
	if err := ensurePrivateDirectory(nodeModules); err != nil {
		return "", "", err
	}
	bin = filepath.Join(nodeModules, ".bin")
	if err := ensurePrivateDirectory(bin); err != nil {
		return "", "", err
	}
	return root, bin, nil
}

func managedPaths() (root, bin string, err error) {
	home, err := config.Dir()
	if err != nil {
		return "", "", err
	}
	root = filepath.Join(home, managedToolsDirName)
	return root, filepath.Join(root, managedBinDirName), nil
}

func managedBinaryPath(name string) string {
	_, bin, err := managedPaths()
	if err != nil {
		return ""
	}
	for _, candidate := range managedBinaryCandidates(bin, name) {
		resolved := trustedManagedExecutable(candidate)
		if resolved == "" {
			// A pre-existing symlink in the managed folder must not turn
			// PATH lookup into an escape hatch to an arbitrary executable.
			continue
		}
		return resolved
	}
	return ""
}

func executableAncestorsProtected(root, target string) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	root, ok := canonicalPath(root)
	if !ok {
		return false
	}
	target, ok = canonicalPath(target)
	if !ok {
		return false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	for current := root; ; current = filepath.Dir(current) {
		rootInfo, err := os.Stat(current)
		if err != nil || !rootInfo.IsDir() || (rootInfo.Mode().Perm()&0o022 != 0 && rootInfo.Mode()&os.ModeSticky == 0) {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	current := root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		st, err := os.Stat(current)
		if err != nil {
			return false
		}
		if i == len(parts)-1 {
			continue
		}
		if !st.IsDir() || (st.Mode().Perm()&0o022 != 0 && st.Mode()&os.ModeSticky == 0) {
			return false
		}
	}
	return true
}

func managedBinaryCandidates(binDir, name string) []string {
	candidates := []string{filepath.Join(binDir, name)}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, filepath.Join(binDir, name+".cmd"), filepath.Join(binDir, name+".exe"))
	}
	return candidates
}

func pathWithin(root, target string) bool {
	root, ok := canonicalPath(root)
	if !ok {
		return false
	}
	target, ok = canonicalPath(target)
	if !ok {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func canonicalPath(path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved), true
	}
	dir, base := filepath.Split(abs)
	if dir == "" {
		dir = "."
	}
	resolvedDir, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return "", false
	}
	return filepath.Clean(filepath.Join(resolvedDir, base)), true
}

func ensurePrivateDirectory(path string) error {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		st, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlinked directory %s", path)
	}
	if !st.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("restrict %s: %w", path, err)
		}
	}
	return nil
}

func writeProviderManifests(root string) error {
	for _, manifest := range []struct {
		name string
		data []byte
	}{
		{name: "package.json", data: providerPackageJSON},
		{name: "package-lock.json", data: providerPackageLockJSON},
	} {
		path := filepath.Join(root, manifest.name)
		st, err := os.Lstat(path)
		if err == nil {
			if st.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing symlinked provider manifest %s", path)
			}
			if st.IsDir() {
				return fmt.Errorf("provider manifest %s is a directory", path)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := replacePrivateFile(path, manifest.data); err != nil {
			return fmt.Errorf("replace %s: %w", path, err)
		}
	}
	return nil
}

func rejectManagedNPMConfig(root string) error {
	path := filepath.Join(root, ".npmrc")
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect managed npm config: %w", err)
	}
	if st.Mode()&os.ModeSymlink != 0 || st.IsDir() {
		return fmt.Errorf("refusing managed npm config path %s", path)
	}
	return fmt.Errorf("refusing unreviewed managed npm config %s", path)
}

func replacePrivateFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".picogent-manifest-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}

	// Windows does not replace an existing file with Rename. Remove only the
	// exact manifest path, never a recursively selected target, then retry the
	// same atomic rename. If another writer races this narrow window, Rename
	// fails closed instead of following a replacement symlink.
	if st, statErr := os.Lstat(path); statErr != nil {
		if !os.IsNotExist(statErr) {
			return statErr
		}
	} else if st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("manifest destination is not a regular file")
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmpPath, path)
}

// installerEnv is an explicit allowlist. In particular, API keys, auth
// cookies, loader hooks, npm configuration overrides, shell startup hooks, and
// untrusted PATH entries never cross into a package manager or login terminal.
func installerEnv(program string) []string {
	env := make([]string, 0, 16)
	add := func(key, value string) {
		if value == "" {
			return
		}
		for _, existing := range env {
			if strings.HasPrefix(existing, key+"=") {
				return
			}
		}
		env = append(env, key+"="+value)
	}

	if home, err := os.UserHomeDir(); err == nil {
		add("HOME", home)
		add("USERPROFILE", home)
	}
	add("TMPDIR", os.TempDir())
	add("TMP", os.TempDir())
	add("TEMP", os.TempDir())
	for _, key := range []string{
		"SystemRoot", "WINDIR", "APPDATA", "LOCALAPPDATA",
		"LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES", "TERM", "COLORTERM",
	} {
		add(key, os.Getenv(key))
	}
	add("PATH", installerPath(program))
	return env
}

func interactiveEnv(program string) []string {
	env := installerEnv(program)
	for _, key := range []string{"DISPLAY", "WAYLAND_DISPLAY", "DBUS_SESSION_BUS_ADDRESS", "XAUTHORITY"} {
		value := os.Getenv(key)
		if value == "" {
			continue
		}
		env = append(env, key+"="+value)
	}
	return env
}

func installerPath(program string) string {
	dirs := make([]string, 0, 12)
	addDir := func(path string) {
		if path == "" {
			return
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		for _, existing := range dirs {
			if existing == path {
				return
			}
		}
		dirs = append(dirs, path)
	}
	if program != "" {
		addDir(filepath.Dir(program))
		if resolved, ok := canonicalPath(program); ok {
			addDir(filepath.Dir(resolved))
		}
	}
	// A node-based CLI commonly uses /usr/bin/env in its shebang. Resolve the
	// trusted node runtime separately so an attacker-controlled PATH entry
	// cannot replace it while npm starts. Do not append fallback trust roots:
	// doing so would let an untrusted node in one of those roots win shebang
	// lookup when no trusted node was found.
	if node := externalLook("node"); node != "" {
		addDir(filepath.Dir(node))
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}

type cappedBuffer struct {
	buf       bytes.Buffer
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len() < maxInstallerOutput {
		remaining := maxInstallerOutput - b.buf.Len()
		if len(p) > remaining {
			_, _ = b.buf.Write(p[:remaining])
			b.truncated = true
		} else {
			_, _ = b.buf.Write(p)
		}
	} else {
		b.truncated = true
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	out := b.buf.String()
	if b.truncated {
		out += "\n[installer output truncated]"
	}
	return out
}

func runTimedWithEnv(d time.Duration, name string, args []string, env []string) (string, error) {
	if setupElevatedFn() {
		return "", fmt.Errorf("refusing command execution from an elevated process; rerun Picogent as the normal user")
	}
	validated, err := validateLaunchExecutable(name)
	if err != nil {
		return "", err
	}
	cmd, err := installerCommand(validated, args)
	if err != nil {
		return "", err
	}
	cmd.Env = append([]string(nil), env...)
	var buf cappedBuffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return buf.String(), err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case err := <-done:
		return buf.String(), err
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-done
		return buf.String(), fmt.Errorf("timed out")
	}
}

func installerCommand(name string, args []string) (*exec.Cmd, error) {
	workingDir, filteredArgs, err := installerWorkingDirectory(args)
	if err != nil {
		return nil, err
	}
	args = filteredArgs
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".cmd" || ext == ".bat" {
			shell := externalLook("cmd.exe")
			if shell == "" {
				return nil, fmt.Errorf("cmd.exe is not trusted")
			}
			validated, err := validateLaunchExecutable(shell)
			if err != nil {
				return nil, err
			}
			cmd = exec.Command(validated, "/D", "/S", "/C", shellCommandLine(name, args...))
		} else {
			cmd = exec.Command(name, args...)
		}
	} else {
		cmd = exec.Command(name, args...)
	}
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	return cmd, nil
}

func installerWorkingDirectory(args []string) (string, []string, error) {
	for i, arg := range args {
		if arg != "--prefix" {
			continue
		}
		if i+1 >= len(args) || args[i+1] == "" {
			return "", nil, fmt.Errorf("installer prefix is missing")
		}
		root, _, err := managedPaths()
		if err != nil {
			return "", nil, err
		}
		resolvedRoot, ok := canonicalPath(root)
		if !ok {
			return "", nil, fmt.Errorf("installer prefix is unavailable")
		}
		resolvedPrefix, ok := canonicalPath(args[i+1])
		if !ok || !pathWithin(resolvedRoot, resolvedPrefix) || !pathWithin(resolvedPrefix, resolvedRoot) {
			return "", nil, fmt.Errorf("installer prefix is outside Picogent's private CLI folder")
		}
		filtered := make([]string, 0, len(args)-2)
		filtered = append(filtered, args[:i]...)
		filtered = append(filtered, args[i+2:]...)
		return resolvedRoot, filtered, nil
	}
	return "", args, nil
}

var (
	redactInstallerAssignment = regexp.MustCompile(`(?i)(\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|token|secret|password|passwd|authorization|cookie|private[_-]?key)\b[\s"]*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;}&]+)`)
	redactInstallerQuery      = regexp.MustCompile(`(?i)([?&](?:api[_-]?key|access[_-]?token|refresh[_-]?token|token|secret|password|key)=)[^&#\s]+`)
	redactInstallerBearer     = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	redactInstallerKnown      = regexp.MustCompile(`(?i)\b(?:sk-[A-Za-z0-9_-]{8,}|gh[pousr]_[A-Za-z0-9_]{8,}|xox[baprs]-[A-Za-z0-9-]{8,}|AIza[A-Za-z0-9_-]{16,})\b`)
)

func redactInstallerOutput(out string) string {
	out = redactInstallerAssignment.ReplaceAllString(out, "$1[REDACTED]")
	out = redactInstallerQuery.ReplaceAllString(out, "$1[REDACTED]")
	out = redactInstallerBearer.ReplaceAllString(out, "Bearer [REDACTED]")
	out = redactInstallerKnown.ReplaceAllString(out, "[REDACTED]")
	return strings.TrimSpace(out)
}
