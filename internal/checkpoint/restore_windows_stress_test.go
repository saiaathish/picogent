//go:build windows

package checkpoint_test

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/checkpoint"
)

func TestRestoreDoesNotWriteThroughReplacementHardlinkDuringWindowsStress(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(workspace, "state.txt")
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}

	cp, err := checkpoint.Capture(workspace, []string{"state.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	started := make(chan struct{})
	var stopOnce sync.Once
	var firstReplacement sync.Once
	var replacements atomic.Int32
	var attacker sync.WaitGroup
	attacker.Add(1)
	go func() {
		defer attacker.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := os.Remove(target); err != nil {
				continue
			}
			if err := os.Link(secret, target); err != nil {
				continue
			}
			replacements.Add(1)
			firstReplacement.Do(func() { close(started) })
			// Keep the outside-linked name present long enough for Restore to
			// observe it, then recreate only the in-workspace test value.
			time.Sleep(100 * time.Microsecond)
			if err := os.Remove(target); err == nil {
				_ = os.WriteFile(target, []byte("after"), 0o600)
			}
		}
	}()

	stopAttacker := func() {
		stopOnce.Do(func() { close(stop) })
		attacker.Wait()
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		stopAttacker()
		t.Fatal("replacement hardlink was never created")
	}

	type restoreOutcome struct {
		result checkpoint.RestoreResult
		err    error
	}
	outcomes := make(chan restoreOutcome, 1)
	go func() {
		result, restoreErr := cp.Restore()
		outcomes <- restoreOutcome{result: result, err: restoreErr}
	}()

	select {
	case <-time.After(5 * time.Second):
		stopAttacker()
		t.Fatal("checkpoint restore did not finish during replacement stress")
	case outcome := <-outcomes:
		_ = outcome.result
		_ = outcome.err
	}
	stopAttacker()

	if replacements.Load() == 0 {
		t.Fatal("replacement stress did not create a hardlink")
	}
	if got, readErr := os.ReadFile(secret); readErr != nil || string(got) != "private" {
		t.Fatalf("restore changed outside file through replacement hardlink: %q, %v", got, readErr)
	}
}
