package ctxmgr

import (
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
)

func TestBudgetForModelCodex256k(t *testing.T) {
	cases := []string{
		"gpt-5.6-luna",
		"gpt-5.6-terra",
		"gpt-5.6-sol",
		"codex",
		"gpt-5.4",
		"",
	}
	for _, m := range cases {
		if got := BudgetForModel(m); got != 256_000 {
			t.Fatalf("BudgetForModel(%q)=%d want 256000", m, got)
		}
	}
	if got := BudgetForModel("claude-opus-5"); got != 200_000 {
		t.Fatalf("opus budget=%d", got)
	}
	if got := BudgetForModel("claude-haiku-4-5"); got != 128_000 {
		t.Fatalf("haiku budget=%d", got)
	}
}

func TestDigestStaleToolsKeepsRecent(t *testing.T) {
	long := strings.Repeat("noise line of tool output\n", 80)
	msgs := []llm.Message{
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "read_file", Content: long},
	}
	out := DigestStaleTools(msgs)
	if !strings.HasPrefix(out[0].Content, "[digested ") {
		t.Fatalf("stale tool should digest: %q", clip(out[0].Content, 60))
	}
	if strings.HasPrefix(out[4].Content, "[digested ") {
		t.Fatal("most recent tools must stay intact")
	}
}

func TestDigestToolRetainsRankedFailureSignals(t *testing.T) {
	content := strings.Join([]string{
		"running workspace checks",
		"noise that should be dropped",
		"internal/auth: TestLogin",
		"FAIL internal/auth",
		"panic: unexpected nil session",
		"exit status 1",
		"final unrelated line",
	}, "\n")

	got := digestTool("bash", content)
	for _, want := range []string{
		"untrusted tool output",
		"summary: running workspace checks",
		"signal: panic: unexpected nil session",
		"signal: exit status 1",
		"signal: FAIL internal/auth",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("digest missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "noise that should be dropped") || strings.Contains(got, "final unrelated line") {
		t.Fatalf("digest retained low-value lines: %q", got)
	}
	if len(got) > maxDigestedToolChars {
		t.Fatalf("digest length=%d, want <= %d", len(got), maxDigestedToolChars)
	}
}

func TestDigestToolFlattensMultilineSignalsAndBoundsLongLines(t *testing.T) {
	content := "command\nerror: " + strings.Repeat("x", maxDigestedToolLine*3) + "\nexit status 2"
	got := digestTool("verify", content)
	if strings.Contains(got, "\nsignal: error: "+strings.Repeat("x", 20)+"\n") {
		t.Fatal("multiline signal was not flattened and bounded")
	}
	if !strings.Contains(got, "signal: exit status 2") {
		t.Fatalf("exit metadata was lost: %q", got)
	}
	if len(got) > maxDigestedToolChars {
		t.Fatalf("digest length=%d, want <= %d", len(got), maxDigestedToolChars)
	}
}

func TestSoftFitHoldsUnderTarget(t *testing.T) {
	long := strings.Repeat("abcdefghijklmnopqrstuvwxyz\n", 400)
	msgs := []llm.Message{{Role: "system", Content: "sys"}}
	for i := 0; i < 12; i++ {
		msgs = append(msgs,
			llm.Message{Role: "assistant", Content: "working"},
			llm.Message{Role: "tool", Name: "bash", Content: long},
		)
	}
	before := EstimateTokens(msgs)
	soft := 8_000
	out, method := SoftFit(msgs, soft)
	after := EstimateTokens(out)
	if after >= before {
		t.Fatalf("SoftFit should shrink: before=%d after=%d method=%s", before, after, method)
	}
	if after > soft*2 {
		// allow some slack for system + recent tools, but must be far below raw dump
		t.Fatalf("SoftFit still too large: after=%d soft=%d method=%s", after, soft, method)
	}
	if method == "" {
		t.Fatal("expected a soft method label")
	}
}

func TestManageGrowsSlowlyUnderSoftTarget(t *testing.T) {
	long := strings.Repeat("line of exploration output that would blow the window\n", 200)
	msgs := []llm.Message{
		{Role: "system", Content: "you are picogent"},
		{Role: "user", Content: "explore the repo"},
	}
	for i := 0; i < 10; i++ {
		msgs = append(msgs,
			llm.Message{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{
					{ID: "c" + string(rune('a'+i)), Name: "bash", Arguments: `{"cmd":"ls"}`},
				},
			},
			llm.Message{Role: "tool", ToolCallID: "c" + string(rune('a'+i)), Name: "bash", Content: long},
		)
	}
	budget := BudgetForModel("gpt-5.6-terra")
	out, st, err := Manage(t.Context(), nil, "gpt-5.6-terra", msgs, budget)
	if err != nil {
		t.Fatal(err)
	}
	if budget != 256_000 {
		t.Fatalf("budget=%d", budget)
	}
	raw := EstimateTokens(msgs)
	if st.Tokens >= raw {
		t.Fatalf("Manage should compress: raw=%d got=%d method=%s", raw, st.Tokens, st.Method)
	}
	soft := SoftTarget(budget)
	// Live set should stay near soft target, nowhere near the 256k ceiling.
	if st.Tokens > soft*3 {
		t.Fatalf("working set too large vs soft target: tokens=%d soft=%d pct=%.2f method=%s", st.Tokens, soft, st.Pct, st.Method)
	}
	if st.Pct > 0.25 {
		t.Fatalf("ring should stay low on 256k ceiling: pct=%.2f method=%s msgs=%d", st.Pct, st.Method, len(out))
	}
	if !st.Compacted {
		t.Fatal("expected compacted=true for a bloated tool loop")
	}
}
