// Package procenv builds a bounded environment for subprocesses that inspect
// or verify a user's workspace. It keeps ordinary runtime settings while
// removing credentials, loader hooks, shell startup files, and tool-control
// variables that can execute code or redirect a tool outside its contract.
package procenv

import (
	"os"
	"strings"
)

// Sanitized returns the inherited environment with variables that can expose
// credentials or change subprocess behavior through an implicit hook removed.
// PATH remains available so the configured toolchain can be found.
func Sanitized() []string {
	out := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || UnsafeKey(key) {
			continue
		}
		out = append(out, entry)
	}
	if path, ok := os.LookupEnv("PATH"); ok && !hasKey(out, "PATH") {
		out = append(out, "PATH="+path)
	}
	return out
}

// UnsafeKey reports whether an inherited variable is not safe to pass to a
// workspace-controlled subprocess.
func UnsafeKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if upper == "" {
		return true
	}
	// Pager variables are tool-control inputs. Several clients use prefixed
	// variants such as GH_PAGER or MANPAGER, and all can redirect output to an
	// inherited executable or interactive process.
	if strings.HasSuffix(upper, "PAGER") {
		return true
	}
	for _, marker := range []string{
		"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "AUTH", "COOKIE", "CREDENTIAL", "PRIVATE", "OAUTH",
	} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	for _, exact := range []string{
		// Shell startup and command interpretation.
		"BASH_ENV", "ENV", "CDPATH", "PROMPT_COMMAND", "SHELLOPTS", "BASHOPTS", "PS4", "ZDOTDIR", "KSH_ENV",
		"PAGER", "LESS", "LV", "SSH_ASKPASS", "SSH_AUTH_SOCK", "GIT_ASKPASS",
		// Dynamic/module loading and language startup.
		"LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "DYLD_LIBRARY_PATH", "PYTHONPATH", "PYTHONHOME", "PYTHONSTARTUP",
		"RUBYOPT", "RUBYLIB", "PERL5OPT", "PERL5LIB", "NODE_OPTIONS", "NODE_PATH", "JAVA_TOOL_OPTIONS", "_JAVA_OPTIONS", "JDK_JAVA_OPTIONS",
		// Package-manager and compiler configuration that can select hooks,
		// wrappers, registries, or alternate configuration files.
		"GIT_EXEC_PATH", "GOENV", "GOFLAGS", "GOMOD", "GOWORK", "GOTOOLCHAIN", "RUSTC_WRAPPER", "RUSTFLAGS", "RUSTDOCFLAGS",
		"CARGO_HOME", "PIP_CONFIG_FILE", "PIP_INDEX_URL", "PIP_EXTRA_INDEX_URL", "MAVEN_OPTS", "GRADLE_OPTS", "COMSPEC",
		"RIPGREP_CONFIG_PATH", "GREP_OPTIONS",
	} {
		if upper == exact {
			return true
		}
	}
	if strings.HasPrefix(upper, "GIT_") ||
		strings.HasPrefix(upper, "NPM_CONFIG_") ||
		strings.HasPrefix(upper, "YARN_") ||
		strings.HasPrefix(upper, "BUN_") ||
		strings.HasPrefix(upper, "CARGO_") ||
		strings.HasPrefix(upper, "RUSTUP_") ||
		strings.HasPrefix(upper, "DYLD_") ||
		strings.HasPrefix(upper, "LD_") {
		return true
	}
	return false
}

func hasKey(env []string, want string) bool {
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, want) {
			return true
		}
	}
	return false
}
