package verify

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Python discovery needs neither Node nor installed third-party packages for
// unittest and syntax checks. Never install dependencies during verification.
func detectPython(workspace string) (string, string, []string) {
	runner := "python3"
	if _, err := exec.LookPath(runner); err != nil {
		runner = "python"
	}
	var files []string
	usesUnittest, usesPytest := false, false
	_ = filepath.WalkDir(workspace, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".venv", "venv", "node_modules", "__pycache__":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(d.Name(), ".py") || len(files) >= 300 {
			return nil
		}
		rel, _ := filepath.Rel(workspace, path)
		files = append(files, rel)
		if strings.HasPrefix(d.Name(), "test") || strings.HasSuffix(d.Name(), "_test.py") {
			data, err := os.ReadFile(path)
			if err != nil || len(data) > 1<<20 {
				return nil
			}
			text := string(data)
			usesUnittest = usesUnittest || strings.Contains(text, "unittest")
			usesPytest = usesPytest || strings.Contains(text, "pytest") || strings.Contains(text, "def test_")
		}
		return nil
	})
	if usesUnittest {
		start := "."
		if info, err := os.Lstat(filepath.Join(workspace, "tests")); err == nil && info.IsDir() {
			start = "tests"
		}
		args := []string{"-I", "-c", `import os,sys,unittest; sys.path.insert(0,os.getcwd()); suite=unittest.defaultTestLoader.discover(sys.argv[1]); result=unittest.TextTestRunner(verbosity=2).run(suite); raise SystemExit(not result.wasSuccessful())`, start}
		return runner, runner + " -m unittest discover -s " + start, args
	}
	if usesPytest || fileExists(filepath.Join(workspace, "pytest.ini")) || fileExists(filepath.Join(workspace, "conftest.py")) {
		return runner, runner + " -m pytest -q", []string{"-m", "pytest", "-q"}
	}
	if len(files) > 0 {
		args := []string{"-I", "-c", pythonSyntaxScript}
		args = append(args, files...)
		return runner, "Python syntax check (no tests detected)", args
	}
	return "", "", nil
}

const pythonSyntaxScript = `import ast, os, pathlib, sys
root=os.path.realpath(os.getcwd())
for name in sys.argv[1:]:
 path=os.path.realpath(name)
 if os.path.commonpath([root,path])!=root: raise SystemExit("syntax check refused outside-project path")
 if os.path.getsize(path)>4*1024*1024: raise SystemExit("syntax check file exceeds size limit")
 ast.parse(pathlib.Path(path).read_bytes(),filename=name)
print("Syntax checked %d files; no tests were run" % len(sys.argv[1:]))
`
