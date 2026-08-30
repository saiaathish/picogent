package procenv

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"sync"
	"time"
)

const (
	// MaxOutputBytes is the maximum amount of output retained from an external
	// helper. The helper keeps draining after the limit so a noisy process
	// cannot deadlock its caller on a full pipe.
	MaxOutputBytes = 1 << 20

	// DefaultCommandTimeout bounds short-lived discovery and credential
	// commands when the caller has not supplied a tighter deadline.
	DefaultCommandTimeout = 10 * time.Second
)

// Result contains bounded subprocess output and whether the output cap was
// reached. A truncated result must not be parsed as a complete document.
type Result struct {
	Output    []byte
	Truncated bool
}

// Output runs a subprocess with the sanitized environment, retaining only
// bounded stdout. Stderr is deliberately discarded because callers usually
// parse stdout as data and must not mix diagnostics into it.
func Output(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error) {
	return run(ctx, timeout, false, name, args...)
}

// Combined runs a subprocess with the sanitized environment, retaining a
// bounded combination of stdout and stderr.
func Combined(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error) {
	return run(ctx, timeout, true, name, args...)
}

func run(ctx context.Context, timeout time.Duration, combined bool, name string, args ...string) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	commandCtx, cancel := boundedContext(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(commandCtx, name, args...)
	cmd.Env = Sanitized()
	var output cappedBuffer
	cmd.Stdout = &output
	if combined {
		cmd.Stderr = &output
	} else {
		cmd.Stderr = io.Discard
	}
	err := cmd.Run()
	return output.result(), err
}

func boundedContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
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
	return Result{Output: append([]byte(nil), b.data.Bytes()...), Truncated: b.truncated}
}
