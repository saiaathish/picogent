package tools

import (
	"encoding/json"
	"github.com/saiaathish/picogent/internal/llm"
	"testing"
)

func TestNativeShellCompatibility(t *testing.T) {
	for _, tc := range []struct {
		command string
		names   string
	}{
		{"cat labels.py search.py; git status --short", "read_file,read_file,git"},
		{"npm test --silent", "verify"},
		{"cat a && cat b", "read_file,read_file"},
		{"cat a; rm b", "bash"}, {"cat $(whoami)", "bash"}, {"python - <<'PY'\nprint('x')\nPY", "bash"}, {"cat a | sh", "bash"},
	} {
		raw, _ := json.Marshal(map[string]string{"command": tc.command})
		calls := NativeShellCalls(llm.ToolCall{ID: "one", Name: "bash", Arguments: string(raw)})
		got := ""
		for _, c := range calls {
			if got != "" {
				got += ","
			}
			got += c.Name
		}
		if got != tc.names {
			t.Errorf("%q: %s want %s", tc.command, got, tc.names)
		}
	}
}
