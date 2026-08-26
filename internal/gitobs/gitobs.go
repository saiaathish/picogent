// Package gitobs provides bounded, non-interactive Git subprocesses for
// repository observation and controlled Git actions.
package gitobs

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/saiaathish/picogent/internal/redact"
)

const (
	// MaxOutputBytes bounds bytes retained from any Git subprocess.
	MaxOutputBytes = 1 << 20
	defaultTimeout = 10 * time.Second
)

// Result contains bounded subprocess output and whether the bound was hit.
type Result struct {
	Output    string
	Truncated bool
}

// Command constructs a Git command with repository-controlled helpers and
// inherited Git configuration overrides disabled. The caller may use the
// returned command with StdoutPipe for streaming, as repo-map inventory does.
func Command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmdArgs := safeArgs(args)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = dir
	cmd.Env = sanitizedEnv()
	return cmd
}

// Combined runs Git while retaining bounded stdout and stderr. It is used for
// user-visible command results, which must not be allowed to grow without
// limit.
func Combined(ctx context.Context, dir string, args ...string) (Result, error) {
	commandCtx, cancel := boundedContext(ctx)
	defer cancel()
	cmd := Command(commandCtx, dir, args...)
	var output cappedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	result := output.result()
	result.Output = redact.Text(result.Output)
	return result, err
}

// Output runs Git with bounded stdout and discarded stderr. It is used when
// callers parse machine-readable Git output and must not mix diagnostics into
// that stream.
func Output(ctx context.Context, dir string, args ...string) (Result, error) {
	commandCtx, cancel := boundedContext(ctx)
	defer cancel()
	cmd := Command(commandCtx, dir, args...)
	var output cappedBuffer
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return output.result(), err
}

func boundedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, defaultTimeout)
}

func safeArgs(args []string) []string {
	global := []string{
		"--no-pager",
		"--no-optional-locks",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=",
		"-c", "core.pager=cat",
		"-c", "color.ui=false",
		"-c", "credential.helper=",
		"-c", "core.askPass=",
		"-c", "core.sshCommand=",
		"-c", "commit.gpgSign=false",
		"-c", "diff.external=",
	}
	out := append(global, args...)
	if len(args) == 0 {
		return out
	}
	switch args[0] {
	case "diff":
		out = append(global, "diff", "--no-ext-diff", "--no-textconv")
		out = append(out, args[1:]...)
	case "commit":
		out = append(global, "commit", "--no-verify", "--no-gpg-sign")
		out = append(out, args[1:]...)
	}
	return out
}

func sanitizedEnv() []string {
	env := make([]string, 0, len(os.Environ())+8)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || unsafeEnvKey(key) {
			continue
		}
		env = append(env, entry)
	}
	// Keep Git from loading system/global configuration or waiting for a
	// credential prompt. Repository-local configuration is still read for
	// ordinary metadata, while command-line flags above disable helpers.
	env = append(env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PAGER=cat",
		"GIT_EXTERNAL_DIFF=",
	)
	return env
}

func unsafeEnvKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if upper == "" || strings.HasPrefix(upper, "GIT_") {
		return true
	}
	for _, marker := range []string{
		"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "AUTH", "COOKIE", "CREDENTIAL", "PRIVATE", "OAUTH",
	} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	switch upper {
	case "PAGER", "LESS", "LV", "SSH_ASKPASS", "SSH_AUTH_SOCK", "GIT_ASKPASS":
		return true
	default:
		return false
	}
}

type cappedBuffer struct {
	mu        sync.Mutex
	data      bytes.Buffer
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := MaxOutputBytes - b.data.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.data.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.data.Write(p)
	return len(p), nil
}

func (b *cappedBuffer) result() Result {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Result{Output: b.data.String(), Truncated: b.truncated}
}
