package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInstallNPMUsesPrivatePrefixAndSanitizedEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PATH", filepath.Join(home, "attacker")+string(os.PathListSeparator)+"/usr/bin")
	for _, key := range []string{
		"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "AWS_SECRET_ACCESS_KEY",
		"GH_TOKEN", "BASH_ENV", "NODE_OPTIONS", "PYTHONPATH", "LD_PRELOAD", "NPM_CONFIG_USERCONFIG",
	} {
		t.Setenv(key, "sentinel-"+key)
	}

	oldLookup := execLookPath
	oldTrust := trustedExecutableFn
	oldElevated := setupElevatedFn
	oldRunner := runInstaller
	t.Cleanup(func() {
		execLookPath = oldLookup
		trustedExecutableFn = oldTrust
		setupElevatedFn = oldElevated
		runInstaller = oldRunner
	})
	fakeNPM := filepath.Join(home, "trusted", "npm")
	var gotName string
	var gotArgs []string
	var gotEnv []string
	execLookPath = func(name string) (string, error) {
		if name == "npm" {
			return fakeNPM, nil
		}
		return "", fmt.Errorf("%s not found", name)
	}
	trustedExecutableFn = func(_ string, path string) string { return path }
	setupElevatedFn = func() bool { return false }
	runInstaller = func(_ time.Duration, name string, args []string, env []string) (string, error) {
		gotName = name
		gotArgs = append([]string(nil), args...)
		gotEnv = append([]string(nil), env...)
		// This mirrors npm's local-prefix layout; do not derive it from the
		// production lookup constant or the test could mask a layout bug.
		binDir := filepath.Join(home, managedToolsDirName, "node_modules", ".bin")
		if err := os.MkdirAll(binDir, 0o700); err != nil {
			return "", err
		}
		mode := os.FileMode(0o700)
		if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\n"), mode); err != nil {
			return "", err
		}
		for _, manifest := range []struct {
			name string
			data []byte
		}{
			{name: "package.json", data: providerPackageJSON},
			{name: "package-lock.json", data: providerPackageLockJSON},
		} {
			got, err := os.ReadFile(filepath.Join(home, managedToolsDirName, manifest.name))
			if err != nil {
				return "", err
			}
			if !bytes.Equal(got, manifest.data) {
				return "", fmt.Errorf("materialized %s differs from reviewed source", manifest.name)
			}
		}
		return "", nil
	}

	var log string
	if err := installNPM(codexNPMSpec, func(s string) { log += s }); err != nil {
		t.Fatal(err)
	}
	if gotName != fakeNPM {
		t.Fatalf("npm path = %q, want %q", gotName, fakeNPM)
	}
	if hasInstallerArg(gotArgs, "-g") || hasInstallerArg(gotArgs, "--global") {
		t.Fatalf("installer unexpectedly requested global npm installation: %q", gotArgs)
	}
	if !hasInstallerArg(gotArgs, "--ignore-scripts") || !hasInstallerArg(gotArgs, "--no-audit") || !hasInstallerArg(gotArgs, "--no-fund") {
		t.Fatalf("installer missing lifecycle/network safety flags: %q", gotArgs)
	}
	if !hasInstallerArg(gotArgs, "ci") {
		t.Fatalf("installer did not use npm's frozen lockfile command: %q", gotArgs)
	}
	if hasInstallerArg(gotArgs, codexNPMSpec.Package) || hasInstallerArg(gotArgs, claudeNPMSpec.Package) {
		t.Fatalf("installer accepted an unreviewed command-line package spec: %q", gotArgs)
	}
	if got := installerArgValue(gotArgs, "--registry"); got != npmRegistry {
		t.Fatalf("registry = %q, want %q", got, npmRegistry)
	}
	if got := installerArgValue(gotArgs, "--prefix"); got != filepath.Join(home, managedToolsDirName) {
		t.Fatalf("prefix = %q", got)
	}
	for _, key := range []string{"--userconfig", "--globalconfig", "--cache"} {
		path := installerArgValue(gotArgs, key)
		if path == "" {
			t.Fatalf("missing %s in %q", key, gotArgs)
		}
		if !pathWithin(filepath.Join(home, managedToolsDirName), path) {
			t.Fatalf("%s escaped private tools directory: %q", key, path)
		}
	}

	env := installerEnvMap(gotEnv)
	if env["PATH"] == "" || env["HOME"] == "" || env["TMPDIR"] == "" {
		t.Fatalf("missing operational environment: %v", env)
	}
	if strings.Contains(env["PATH"], filepath.Join(home, "attacker")) {
		t.Fatalf("installer inherited an attacker-controlled PATH entry: %q", env["PATH"])
	}
	for _, key := range []string{
		"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "AWS_SECRET_ACCESS_KEY",
		"GH_TOKEN", "BASH_ENV", "NODE_OPTIONS", "PYTHONPATH", "LD_PRELOAD", "NPM_CONFIG_USERCONFIG",
	} {
		if _, ok := env[key]; ok {
			t.Fatalf("installer inherited %s: %v", key, env)
		}
	}
	if strings.Contains(log, "sentinel-") {
		t.Fatalf("installer log retained a sentinel: %q", log)
	}
	if got := managedBinaryPath("codex"); got == "" {
		t.Fatal("private codex binary was not recognized")
	}
}

func TestInstallNPMRejectsUnapprovedPackage(t *testing.T) {
	oldLookup := execLookPath
	oldTrust := trustedExecutableFn
	oldElevated := setupElevatedFn
	oldRunner := runInstaller
	t.Cleanup(func() {
		execLookPath = oldLookup
		trustedExecutableFn = oldTrust
		setupElevatedFn = oldElevated
		runInstaller = oldRunner
	})
	called := false
	execLookPath = func(string) (string, error) { return "/tmp/npm", nil }
	runInstaller = func(time.Duration, string, []string, []string) (string, error) {
		called = true
		return "", nil
	}
	err := installNPM(npmInstallSpec{Label: "attacker", Package: "evil-package", Binary: "evil"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "unapproved") {
		t.Fatalf("error = %v, want unapproved package rejection", err)
	}
	if called {
		t.Fatal("unapproved package reached the command runner")
	}
}

func TestInstallNPMRefusesElevatedProcess(t *testing.T) {
	oldElevated := setupElevatedFn
	oldRunner := runInstaller
	t.Cleanup(func() {
		setupElevatedFn = oldElevated
		runInstaller = oldRunner
	})
	called := false
	setupElevatedFn = func() bool { return true }
	runInstaller = func(time.Duration, string, []string, []string) (string, error) {
		called = true
		return "", nil
	}
	err := installNPM(codexNPMSpec, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "elevated") {
		t.Fatalf("error = %v, want elevated-process refusal", err)
	}
	if called {
		t.Fatal("elevated process reached the package-manager runner")
	}
}

func TestInstallNPMRedactsAndBoundsFailureOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	oldLookup := execLookPath
	oldTrust := trustedExecutableFn
	oldRunner := runInstaller
	t.Cleanup(func() {
		execLookPath = oldLookup
		trustedExecutableFn = oldTrust
		runInstaller = oldRunner
	})
	execLookPath = func(name string) (string, error) {
		if name == "npm" {
			return filepath.Join(home, "npm"), nil
		}
		return "", errors.New("not found")
	}
	trustedExecutableFn = func(_ string, path string) string { return path }
	setupElevatedFn = func() bool { return false }
	secret := "sk-live-installer-secret"
	runInstaller = func(time.Duration, string, []string, []string) (string, error) {
		return strings.Repeat("noise ", maxInstallerOutput), fmt.Errorf("npm failed")
	}
	err := installNPM(codexNPMSpec, func(string) {})
	if err == nil {
		t.Fatal("expected installer failure")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error retained secret: %v", err)
	}
	if got := redactInstallerOutput(`api_key="` + secret + `" token=token-secret Bearer bearer-secret https://x.test/?token=query-secret`); strings.Contains(got, secret) || strings.Contains(got, "token-secret") || strings.Contains(got, "bearer-secret") || strings.Contains(got, "query-secret") {
		t.Fatalf("redaction failed: %q", got)
	}
	var capped cappedBuffer
	_, _ = capped.Write([]byte(strings.Repeat("x", maxInstallerOutput*2)))
	if len(capped.String()) > maxInstallerOutput+64 || !strings.Contains(capped.String(), "output truncated") {
		t.Fatalf("capped output length/marker invalid: %d", len(capped.String()))
	}
}

func TestInstallerEnvSeparatesDesktopSessionCapabilities(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/tmp/picogent-test-bus")
	t.Setenv("XAUTHORITY", "/tmp/picogent-test-xauth")
	base := installerEnv("")
	baseMap := installerEnvMap(base)
	if _, ok := baseMap["DBUS_SESSION_BUS_ADDRESS"]; ok {
		t.Fatalf("package-manager environment inherited DBus session capability: %v", baseMap)
	}
	if _, ok := baseMap["XAUTHORITY"]; ok {
		t.Fatalf("package-manager environment inherited X11 authority: %v", baseMap)
	}
	interactive := installerEnvMap(interactiveEnv(""))
	if interactive["DBUS_SESSION_BUS_ADDRESS"] == "" || interactive["XAUTHORITY"] == "" {
		t.Fatalf("interactive environment lost desktop handoff variables: %v", interactive)
	}
}

func TestSetupDoesNotAutoExecuteRemoteProviderInstallers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	oldLookup := execLookPath
	t.Cleanup(func() { execLookPath = oldLookup })
	execLookPath = func(string) (string, error) { return "", errors.New("not found") }
	log, err := InstallCores()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log, "https://opencode.ai/docs/installation") || !strings.Contains(log, "official Antigravity") {
		t.Fatalf("missing manual provider guidance: %q", log)
	}
	if strings.Contains(strings.ToLower(log), "curl") || strings.Contains(log, "| bash") || strings.Contains(log, "brew install") {
		t.Fatalf("setup log still advertises a remote shell/package fallback: %q", log)
	}
}

func TestTrustedExternalExecutableRejectsUntrustedPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(home, ".picogent-installer-untrusted-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	npm := filepath.Join(root, "npm")
	if err := os.WriteFile(npm, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := trustedExternalExecutable("npm", npm); got != "" {
		t.Fatalf("untrusted npm path accepted: %q", got)
	}
	if got := trustedExternalExecutable("npm", "relative/npm"); got != "" {
		t.Fatalf("relative npm path accepted: %q", got)
	}
}

func TestTrustedExecutableRejectsWritableAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode checks do not represent Windows ACLs")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	npm := filepath.Join(binDir, "npm")
	if err := os.WriteFile(npm, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatal(err)
	}
	if got := trustedExecutableInRoots(npm, []string{root}); got != "" {
		t.Fatalf("writable executable root accepted: %q", got)
	}
}

func TestProviderPackageLockPinsRegistryIntegrity(t *testing.T) {
	var lock struct {
		LockfileVersion int `json:"lockfileVersion"`
		Packages        map[string]struct {
			Integrity string `json:"integrity"`
			Resolved  string `json:"resolved"`
			Version   string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(providerPackageLockJSON, &lock); err != nil {
		t.Fatalf("provider lock is not valid JSON: %v", err)
	}
	if lock.LockfileVersion != 3 {
		t.Fatalf("lockfile version = %d, want 3", lock.LockfileVersion)
	}
	if len(lock.Packages) < 3 {
		t.Fatalf("provider lock has too few package entries: %d", len(lock.Packages))
	}
	for path, pkg := range lock.Packages {
		if path == "" {
			continue
		}
		if pkg.Version == "" || !strings.HasPrefix(pkg.Resolved, npmRegistry) || !strings.HasPrefix(pkg.Integrity, "sha512-") {
			t.Errorf("package %q lacks pinned registry/integrity metadata: %#v", path, pkg)
		}
	}
	for _, spec := range []npmInstallSpec{codexNPMSpec, claudeNPMSpec} {
		at := strings.LastIndex(spec.Package, "@")
		if at <= 0 || !strings.Contains(string(providerPackageJSON), fmt.Sprintf("%q: %q", spec.Package[:at], spec.Package[at+1:])) {
			t.Errorf("provider manifest does not pin %s", spec.Package)
		}
	}
}

func TestManagedBinaryPathReturnsCanonicalTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed .bin symlink behavior differs on Windows")
	}
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	root := filepath.Join(home, managedToolsDirName)
	binDir := filepath.Join(root, managedBinDirName)
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "node_modules", "@openai", "codex", "bin", "codex.js")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("#!/usr/bin/env node\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(binDir, "codex")
	if err := os.Symlink(target, candidate); err != nil {
		t.Fatal(err)
	}
	want, ok := canonicalPath(target)
	if !ok {
		t.Fatalf("canonical target unavailable: %q", target)
	}
	if got := managedBinaryPath("codex"); got != want {
		t.Fatalf("managed binary path = %q, want canonical target %q", got, want)
	}
}

func TestRunTimedWithEnvRejectsUntrustedExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "npm")
	if _, err := runTimedWithEnv(time.Second, path, nil, nil); err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Fatalf("runTimedWithEnv error = %v, want untrusted executable rejection", err)
	}
}

func TestOpenInteractiveRejectsUntrustedExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider")
	if err := OpenInteractive(path); err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Fatalf("OpenInteractive error = %v, want untrusted executable rejection", err)
	}
}

func TestPrepareManagedToolsRejectsNodeModulesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires platform-specific Windows privileges")
	}
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	root := filepath.Join(home, managedToolsDirName)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "node_modules")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareManagedTools(); err == nil || !strings.Contains(err.Error(), "symlinked directory") {
		t.Fatalf("prepareManagedTools error = %v, want node_modules symlink rejection", err)
	}
}

func TestInstallerCommandUsesManagedPrefixAsWorkingDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	root := filepath.Join(home, managedToolsDirName)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd, err := installerCommand(filepath.Join(root, "npm"), []string{"ci", "--prefix", root, "--ignore-scripts"})
	if err != nil {
		t.Fatal(err)
	}
	want, ok := canonicalPath(root)
	if !ok || cmd.Dir != want {
		t.Fatalf("installer command Dir = %q, want %q", cmd.Dir, want)
	}
	for _, arg := range cmd.Args {
		if arg == "--prefix" || arg == root {
			t.Fatalf("installer command retained internal prefix argument: %q", cmd.Args)
		}
	}
}

func hasInstallerArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func installerArgValue(args []string, key string) string {
	for i, arg := range args {
		if i+1 >= len(args) {
			break
		}
		if arg == key {
			return args[i+1]
		}
	}
	return ""
}

func installerEnvMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}
