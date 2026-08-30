package tools_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/saiaathish/picogent/internal/tools"
)

func TestWriteReadGlob(t *testing.T) {
	dir := t.TempDir()
	reg := tools.NewRegistry(tools.Context{Workspace: dir})
	write, _ := reg.Get("write_file")
	if _, err := write.Run(context.Background(), `{"path":"src/a.go","content":"package src\n"}`, reg.Ctx); err != nil {
		t.Fatal(err)
	}
	read, _ := reg.Get("read_file")
	got, err := read.Run(context.Background(), `{"path":"src/a.go"}`, reg.Ctx)
	if err != nil || !strings.Contains(got, "package src") {
		t.Fatalf("%q %v", got, err)
	}
	glob, _ := reg.Get("glob")
	list, err := glob.Run(context.Background(), `{"pattern":"**/*.go"}`, reg.Ctx)
	if err != nil || !strings.Contains(list, "src/a.go") {
		t.Fatalf("%q %v", list, err)
	}
}

func TestEditUnique(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(tools.Context{Workspace: dir})
	edit, _ := reg.Get("edit_file")
	if _, err := edit.Run(context.Background(), `{"path":"a.txt","old_string":"world","new_string":"picogent"}`, reg.Ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "hello picogent" {
		t.Fatalf("%q", got)
	}
}

func TestWriteToolReadersNeverObservePartialData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows path readers may deny an atomic rename while a handle is open")
	}
	dir := t.TempDir()
	reg := tools.NewRegistry(tools.Context{Workspace: dir})
	write, _ := reg.Get("write_file")
	payload := func(version int) string {
		return fmt.Sprintf("version-%d:%s", version, strings.Repeat("x", 4096))
	}
	if _, err := write.Run(context.Background(), fmt.Sprintf(`{"path":"state.txt","content":%q}`, payload(0)), reg.Ctx); err != nil {
		t.Fatal(err)
	}
	known := make(map[string]struct{}, 81)
	for version := 0; version <= 80; version++ {
		known[payload(version)] = struct{}{}
	}
	start := make(chan struct{})
	done := make(chan struct{})
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		<-start
		for version := 1; version <= 80; version++ {
			args := fmt.Sprintf(`{"path":"state.txt","content":%q}`, payload(version))
			if _, err := write.Run(context.Background(), args, reg.Ctx); err != nil {
				errs <- err
				return
			}
		}
	}()
	for reader := 0; reader < 3; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				select {
				case <-done:
					return
				default:
				}
				data, err := os.ReadFile(filepath.Join(dir, "state.txt"))
				if err != nil {
					errs <- err
					return
				}
				if _, ok := known[string(data)]; !ok {
					errs <- fmt.Errorf("reader observed partial/unknown state of %d bytes", len(data))
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
