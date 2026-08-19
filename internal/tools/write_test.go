package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
