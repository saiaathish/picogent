package tools_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestFilesystemWritesHonorCanceledContext(t *testing.T) {
	workspace := t.TempDir()
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	write, _ := reg.Get("write_file")
	if _, err := write.Run(ctx, `{"path":"new.txt","content":"must not appear"}`, reg.Ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled write = %v, want context canceled", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("canceled write created a file: %v", err)
	}

	path := filepath.Join(workspace, "existing.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	edit, _ := reg.Get("edit_file")
	if _, err := edit.Run(ctx, `{"path":"existing.txt","old_string":"before","new_string":"after"}`, reg.Ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled edit = %v, want context canceled", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "before" {
		t.Fatalf("canceled edit changed file: %q, %v", got, err)
	}
}

func TestEditRequiresUniqueString(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("aa aa"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(tools.Context{Workspace: dir})
	tool, _ := reg.Get("edit_file")
	_, err := tool.Run(context.Background(), `{"path":"a.txt","old_string":"aa","new_string":"bb"}`, reg.Ctx)
	if err == nil {
		t.Fatal("expected uniqueness error")
	}
}

func TestFilesystemToolsClassifyOutsideSymlink(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		t.Fatal(err)
	}

	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	for _, tt := range []struct {
		name string
		args string
	}{
		{name: "read_file", args: `{"path":"escape/secret.txt"}`},
		{name: "write_file", args: `{"path":"escape/secret.txt","content":"owned"}`},
		{name: "edit_file", args: `{"path":"escape/secret.txt","old_string":"private","new_string":"owned"}`},
		{name: "list_dir", args: `{"path":"escape"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tool, ok := reg.Get(tt.name)
			if !ok {
				t.Fatalf("missing %s", tt.name)
			}
			req := tool.Permission(tt.args, reg.Ctx)
			if !req.OutsideWorkspace {
				t.Fatalf("permission request = %+v, want outside workspace", req)
			}
			gate := perm.New(config.ModeFast, workspace, nil)
			if got, err := gate.Check(context.Background(), req); err != nil || got != perm.Deny {
				t.Fatalf("Fast decision = %s, %v; want deny", got, err)
			}
		})
	}
}

func TestFilesystemToolsRejectOutsideSymlinkAtRun(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		t.Fatal(err)
	}

	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	for _, tt := range []struct {
		name string
		args string
	}{
		{name: "read_file", args: `{"path":"escape/secret.txt"}`},
		{name: "write_file", args: `{"path":"escape/secret.txt","content":"owned"}`},
		{name: "edit_file", args: `{"path":"escape/secret.txt","old_string":"private","new_string":"owned"}`},
		{name: "list_dir", args: `{"path":"escape"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tool, ok := reg.Get(tt.name)
			if !ok {
				t.Fatalf("missing %s", tt.name)
			}
			if _, err := tool.Run(context.Background(), tt.args, reg.Ctx); err == nil {
				t.Fatalf("%s followed an outside symlink", tt.name)
			}
		})
	}
	if got, err := os.ReadFile(secret); err != nil || string(got) != "private" {
		t.Fatalf("outside file changed: %q, %v", got, err)
	}
}

func TestUnixWorkspaceWriteDoesNotEscapeAncestorSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("descriptor-relative no-follow test is Unix-specific")
	}
	workspace := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(workspace, "parent")
	realParent := filepath.Join(workspace, "parent-real")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideTarget := filepath.Join(outside, "marker.txt")
	if err := os.WriteFile(outsideTarget, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	firstSwap := make(chan struct{})
	var firstSwapOnce sync.Once
	var swaps sync.WaitGroup
	var swapCount atomic.Int32
	swaps.Add(1)
	go func() {
		defer swaps.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := os.Rename(parent, realParent); err != nil {
				continue
			}
			if err := os.Symlink(outside, parent); err == nil {
				swapCount.Add(1)
				firstSwapOnce.Do(func() { close(firstSwap) })
				_ = os.Remove(parent)
			}
			_ = os.Rename(realParent, parent)
		}
	}()

	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	select {
	case <-firstSwap:
	case <-time.After(2 * time.Second):
		close(stop)
		swaps.Wait()
		t.Fatal("ancestor swap never completed")
	}
	write, _ := reg.Get("write_file")
	for i := 0; i < 250; i++ {
		_, _ = write.Run(context.Background(), `{"path":"parent/marker.txt","content":"workspace"}`, reg.Ctx)
	}
	close(stop)
	swaps.Wait()

	if got, err := os.ReadFile(outsideTarget); err != nil || string(got) != "private" {
		t.Fatalf("ancestor swap escaped workspace: %q, %v", got, err)
	}
	if swapCount.Load() == 0 {
		t.Fatal("ancestor swap never completed")
	}
	if _, err := write.Run(context.Background(), `{"path":"parent/marker.txt","content":"workspace"}`, reg.Ctx); err != nil {
		t.Fatalf("secure write after race failed: %v", err)
	}
}

func TestReadFileBoundsLargeFiles(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "large.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 300<<10)), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	read, _ := reg.Get("read_file")
	got, err := read.Run(context.Background(), `{"path":"large.txt"}`, reg.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "file larger than 256KiB") {
		t.Fatalf("missing truncation marker: %q", got[len(got)-min(len(got), 120):])
	}
	if len(got) > 260<<10 {
		t.Fatalf("read output was not bounded: %d bytes", len(got))
	}

	edit, _ := reg.Get("edit_file")
	if _, err := edit.Run(context.Background(), `{"path":"large.txt","old_string":"x","new_string":"y"}`, reg.Ctx); err == nil || !strings.Contains(err.Error(), "larger than 256KiB") {
		t.Fatalf("large edit error = %v", err)
	}
}
