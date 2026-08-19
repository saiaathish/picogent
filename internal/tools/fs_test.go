package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/tools"
)

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
