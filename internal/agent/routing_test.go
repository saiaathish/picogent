package agent

import "testing"

func TestClassifyToolKind(t *testing.T) {
	cases := map[string]string{
		"read_file":  "read",
		"grep":       "read",
		"write_file": "write",
		"edit_file":  "write",
		"bash":       "shell",
		"git":        "shell",
		"todo_write": "other",
	}
	for name, want := range cases {
		if got := classifyToolKind(name); got != want {
			t.Fatalf("%s: got %q want %q", name, got, want)
		}
	}
}
