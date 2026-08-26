package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		binDir := filepath.Join(home, managedToolsDirName, managedBinDirName)
		if err := os.MkdirAll(binDir, 0o700); err != nil {
			return "", err
		}
		mode := os.FileMode(0o700)
		if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\n"), mode); err != nil {
			return "", err
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
	root := t.TempDir()
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
