package verify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPythonProjectChecks(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "slug.py"), []byte("def slug(x):\n return x.lower()\n"), 0600)
	result := Run(context.Background(), dir)
	if result.Status != StatusInconclusive {
		t.Fatalf("syntax became test pass: %+v", result)
	}
	os.Mkdir(filepath.Join(dir, "tests"), 0700)
	os.WriteFile(filepath.Join(dir, "tests/test_slug.py"), []byte("import unittest\nfrom slug import slug\nclass Test(unittest.TestCase):\n def test_slug(self): self.assertEqual(slug('HI'),'hi')\n"), 0600)
	result = Run(context.Background(), dir)
	if result.Status != StatusPass || result.Passed != 1 {
		t.Fatalf("unittest not detected: %+v", result)
	}
	broad := RunPipeline(context.Background(), dir, Options{})
	if broad.Status != StatusPass {
		t.Fatalf("broad checks skipped: %+v", broad)
	}
	pipeline := RunPipeline(context.Background(), dir, Options{Targets: []string{"slug.py"}})
	if pipeline.Status != StatusPass {
		t.Fatalf("targeted unittest blocked: %+v", pipeline)
	}
	os.WriteFile(filepath.Join(dir, "slug.py"), []byte("def slug(x):\n return x\n"), 0600)
	result = Run(context.Background(), dir)
	if result.Status != StatusFail {
		t.Fatalf("broken fixture passed: %+v", result)
	}
}

func TestUnittestWindowsLineEndings(t *testing.T) {
	passed, failed := count("test_x ... ok\r\n\r\nRan 1 test in 0.001s\r\n\r\nOK\r\n")
	if passed != 1 || failed != 0 {
		t.Fatalf("CRLF result: %d passed, %d failed", passed, failed)
	}
}
