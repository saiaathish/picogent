package setup

import (
	"bytes"
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
		"install",
		"--prefix", root,
		"--no-save",
		"--no-package-lock",
		"--ignore-scripts",
		"--no-audit",
		"--no-fund",
		"--no-update-notifier",
		"--registry", npmRegistry,
		"--userconfig", userConfigPath,
		"--globalconfig", globalConfigPath,
		"--cache", cacheDir,
		spec.Package,
	}
	say(fmt.Sprintf("installing %s into Picogent's private CLI folder (npm registry %s; lifecycle scripts disabled)", spec.Label, npmRegistry))
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
	for _, root := range trustedExecutableRoots() {
		if pathWithin(root, resolved) {
			return resolved
		}
	}
	return ""
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
	bin = filepath.Join(root, managedBinDirName)
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
	root, bin, err := managedPaths()
	if err != nil {
		return ""
	}
	for _, candidate := range managedBinaryCandidates(bin, name) {
		st, err := os.Stat(candidate)
		if err != nil || st.IsDir() {
			continue
		}
		if runtime.GOOS != "windows" && st.Mode().Perm()&0o111 == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil || !pathWithin(root, resolved) {
			// A pre-existing symlink in the managed folder must not turn
			// PATH lookup into an escape hatch to an arbitrary executable.
			continue
		}
		return candidate
	}
	return ""
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
	// cannot replace it while npm starts.
	if node := externalLook("node"); node != "" {
		addDir(filepath.Dir(node))
	}
	for _, root := range trustedExecutableRoots() {
		addDir(root)
		addDir(filepath.Join(root, "bin"))
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
	cmd := installerCommand(name, args)
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

func installerCommand(name string, args []string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".cmd" || ext == ".bat" {
			shell := externalLook("cmd.exe")
			if shell == "" {
				return exec.Command("", "/D", "/S", "/C", shellCommandLine(name, args...))
			}
			return exec.Command(shell, "/D", "/S", "/C", shellCommandLine(name, args...))
		}
	}
	return exec.Command(name, args...)
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
