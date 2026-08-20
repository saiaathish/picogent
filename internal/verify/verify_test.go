package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectGo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner, cmd, args := Detect(dir)
	if runner != "go" || cmd != "go test ./..." || len(args) < 2 {
		t.Fatalf("%s %s %v", runner, cmd, args)
	}
}

func TestDetectNone(t *testing.T) {
	runner, _, _ := Detect(t.TempDir())
	if runner != "" {
		t.Fatal(runner)
	}
}

func TestRunNoneIsInconclusive(t *testing.T) {
	res := Run(t.Context(), t.TempDir())
	if res.OK || res.Runner != "none" {
		t.Fatalf("%+v", res)
	}
	got := Format(res)
	if !strings.Contains(got, "INCONCLUSIVE") {
		t.Fatalf("format: %q", got)
	}
}
