package evolve

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/verify"
)

// Signal is the post-turn evidence used for reflection.
type Signal struct {
	Workspace     string
	UserPrompt    string
	AssistantText string
	FilesChanged  []string
	ToolRounds    int
	GoalDone      bool
	Verified      string
}

// Delta is what changed after a reflection pass.
type Delta struct {
	Habit    *Habit    `json:"habit,omitempty"`
	Playbook *Playbook `json:"playbook,omitempty"`
	Created  bool      `json:"created,omitempty"`
	Updated  bool      `json:"updated,omitempty"`
	Message  string    `json:"message,omitempty"`
	Skipped  string    `json:"skipped,omitempty"`
	Store    Store     `json:"-"`
}

type reflectOut struct {
	Skip     bool   `json:"skip"`
	Reason   string `json:"reason"`
	Habit    string `json:"habit"`
	Title    string `json:"title"`
	Class    string `json:"class"`
	Playbook string `json:"playbook"`
}

const reflectSystem = `Picogent self-evolution reviewer. Return ONLY JSON:
{"skip":true|false,"reason":"","habit":"","title":"","class":"","playbook":""}
Prefer skip. Habits ≤96 chars. Playbooks ≤240 chars, class-level. No secrets.`

// WorthReflecting gates extra work so we do not spend tokens (or even CPU)
// on tiny chat turns.
func WorthReflecting(sig Signal) bool {
	if sig.GoalDone {
		return true
	}
	if len(sig.FilesChanged) > 0 && verify.StatusFromEvidence(sig.Verified) == verify.StatusPass {
		return true
	}
	if len(sig.FilesChanged) >= 3 && sig.ToolRounds >= 3 {
		return true
	}
	return false
}

// Reflect updates the evolve store from a finished turn.
// Default path is heuristic-only (zero extra model tokens). Set
// PICOGENT_EVOLVE_LLM=1 to allow a rare light-model review.
func Reflect(ctx context.Context, client llm.Client, model string, sig Signal) (Delta, error) {
	if strings.TrimSpace(sig.Workspace) == "" {
		return Delta{Skipped: "no workspace"}, nil
	}
	if !WorthReflecting(sig) {
		return Delta{Skipped: "turn too small"}, nil
	}

	store, err := Load(sig.Workspace)
	if err != nil {
		return Delta{}, err
	}

	out := heuristicReflect(sig)
	usedLLM := false
	// LLM reflect is opt-in: it burns tokens; Picogent's default is 10× cheaper harness.
	if allowEvolveLLM() && (out.Skip || strings.TrimSpace(out.Habit) == "" && strings.TrimSpace(out.Playbook) == "") {
		if llmOut, ok := runReflectLLM(ctx, client, model, store, sig); ok {
			out = llmOut
			usedLLM = true
		}
	}
	if out.Skip && strings.TrimSpace(out.Habit) == "" && strings.TrimSpace(out.Playbook) == "" {
		reason := out.Reason
		if reason == "" {
			reason = "nothing durable"
		}
		return Delta{Skipped: reason, Store: store}, nil
	}

	d := Delta{Store: store}
	src := "heuristic"
	if usedLLM {
		src = "reflect"
	}

	if h := strings.TrimSpace(out.Habit); h != "" {
		var created bool
		var habit Habit
		store, habit, created = UpsertHabit(store, h, src)
		d.Habit = &habit
		if created {
			d.Created = true
		} else {
			d.Updated = true
		}
	}
	if body := strings.TrimSpace(out.Playbook); body != "" {
		title := strings.TrimSpace(out.Title)
		if title == "" {
			title = defaultPlaybookTitle(sig)
		}
		class := strings.TrimSpace(out.Class)
		if class == "" {
			class = inferClass(sig)
		}
		var created bool
		var pb Playbook
		store, pb, created = UpsertPlaybook(store, title, body, class, src)
		d.Playbook = &pb
		if created {
			d.Created = true
		} else {
			d.Updated = true
		}
	}

	if d.Habit == nil && d.Playbook == nil {
		return Delta{Skipped: "empty extract", Store: store}, nil
	}

	if err := Save(store); err != nil {
		return Delta{}, err
	}
	d.Store = store
	d.Message = formatDeltaMessage(d)
	return d, nil
}

func allowEvolveLLM() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("PICOGENT_EVOLVE_LLM")))
	return v == "1" || v == "true" || v == "yes"
}

func runReflectLLM(ctx context.Context, client llm.Client, model string, store Store, sig Signal) (reflectOut, bool) {
	if client == nil || strings.TrimSpace(model) == "" {
		return reflectOut{}, false
	}
	user := buildReflectUser(store, sig)
	resp, err := client.Chat(ctx, llm.ChatRequest{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Content: reflectSystem},
			{Role: "user", Content: user},
		},
		TaskMode:  "ask",
		ReadOnly:  true,
		Reasoning: llm.ReasonNone,
	})
	if err != nil {
		return reflectOut{}, false
	}
	out, ok := parseReflectOut(resp.Message.Content)
	if !ok {
		return reflectOut{}, false
	}
	return out, true
}

func buildReflectUser(store Store, sig Signal) string {
	var b strings.Builder
	fmt.Fprintf(&b, "prompt:%s\n", clip(sig.UserPrompt, 180))
	fmt.Fprintf(&b, "assistant:%s\n", clip(sig.AssistantText, 220))
	fmt.Fprintf(&b, "rounds:%d done:%v verified:%s\n", sig.ToolRounds, sig.GoalDone, clip(sig.Verified, 120))
	if len(sig.FilesChanged) > 0 {
		b.WriteString("files:")
		for i, f := range sig.FilesChanged {
			if i >= 6 {
				break
			}
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(filepath.Base(f))
		}
		b.WriteByte('\n')
	}
	b.WriteString("habits:")
	for i, h := range pickHabits(store, 3) {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(clip(h.Text, 60))
	}
	b.WriteString("\nclasses:")
	n := 0
	for _, p := range store.Playbooks {
		if p.Archived {
			continue
		}
		if n > 0 {
			b.WriteByte(',')
		}
		label := p.Class
		if label == "" {
			label = p.Title
		}
		b.WriteString(clip(label, 32))
		n++
		if n >= 4 {
			break
		}
	}
	b.WriteString("\nJSON only.")
	return b.String()
}

func parseReflectOut(raw string) (reflectOut, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return reflectOut{}, false
	}
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}
	var out reflectOut
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return reflectOut{}, false
	}
	out.Habit = strings.TrimSpace(out.Habit)
	out.Title = strings.TrimSpace(out.Title)
	out.Class = strings.TrimSpace(out.Class)
	out.Playbook = strings.TrimSpace(out.Playbook)
	out.Reason = strings.TrimSpace(out.Reason)
	if out.Skip && out.Habit == "" && out.Playbook == "" {
		return out, true
	}
	if out.Habit == "" && out.Playbook == "" {
		out.Skip = true
	}
	return out, true
}

func heuristicReflect(sig Signal) reflectOut {
	exts := map[string]int{}
	for _, f := range sig.FilesChanged {
		ext := strings.ToLower(filepath.Ext(f))
		if ext != "" {
			exts[ext]++
		}
	}
	verifyOK := verify.StatusFromEvidence(sig.Verified) == verify.StatusPass

	out := reflectOut{Skip: true}
	switch {
	case exts[".go"] > 0 && verifyOK:
		out.Skip = false
		out.Habit = "After Go edits, run go test ./... before finishing."
		out.Class = "go-tests"
		out.Title = "Verify Go changes"
		out.Playbook = "Edit targeted pkgs → go test ./... → fix failures → Changed/Run/Undo."
	case (exts[".ts"] > 0 || exts[".tsx"] > 0 || exts[".js"] > 0) && verifyOK:
		out.Skip = false
		out.Habit = "Use package.json scripts for test/lint after JS/TS edits."
		out.Class = "js-verify"
		out.Title = "Verify JS/TS changes"
		out.Playbook = "Find test/lint script → run smallest → fix → re-run → Changed/Run/Undo."
	case sig.GoalDone && len(sig.FilesChanged) > 0:
		// Marker/probe .txt goals are not reusable skills — skip noise.
		if !meaningfulEvolveFiles(sig.FilesChanged) {
			break
		}
		out.Skip = false
		out.Class = inferClass(sig)
		out.Title = defaultPlaybookTitle(sig)
		out.Playbook = "Explore → minimal edits → verify → Changed/Run/Undo."
	case len(sig.FilesChanged) >= 3 && verifyOK:
		out.Skip = false
		out.Habit = "Smallest useful edit, then verify before expanding scope."
	}
	return out
}

func inferClass(sig Signal) string {
	for _, f := range sig.FilesChanged {
		switch strings.ToLower(filepath.Ext(f)) {
		case ".go":
			return "go-edit"
		case ".ts", ".tsx", ".js", ".jsx":
			return "js-edit"
		case ".py":
			return "py-edit"
		case ".rs":
			return "rust-edit"
		}
	}
	if sig.GoalDone {
		return "goal-complete"
	}
	return "general-edit"
}

func defaultPlaybookTitle(sig Signal) string {
	if title := titleFromChangedExt(sig.FilesChanged); title != "" {
		return title
	}
	p := strings.TrimSpace(sig.UserPrompt)
	if p == "" {
		return "Repeatable edit loop"
	}
	// Prefer a short class-level label over dumping the raw user prompt.
	low := strings.ToLower(p)
	switch {
	case strings.Contains(low, "test"):
		return "Get tests green"
	case strings.Contains(low, "fix") || strings.Contains(low, "bug"):
		return "Fix a bug end-to-end"
	case strings.Contains(low, "refactor"):
		return "Refactor safely"
	}
	fields := strings.Fields(p)
	if len(fields) > 5 {
		fields = fields[:5]
	}
	title := strings.Join(fields, " ")
	return clip(title, 48)
}

func titleFromChangedExt(files []string) string {
	exts := map[string]bool{}
	for _, f := range files {
		exts[strings.ToLower(filepath.Ext(f))] = true
	}
	switch {
	case exts[".go"]:
		return "Ship a Go change"
	case exts[".ts"], exts[".tsx"], exts[".js"], exts[".jsx"]:
		return "Ship a JS/TS change"
	case exts[".py"]:
		return "Ship a Python change"
	case exts[".rs"]:
		return "Ship a Rust change"
	}
	return ""
}

// meaningfulEvolveFiles reports whether changed paths look like reusable
// engineering work worth remembering (not one-off marker/probe text files).
func meaningfulEvolveFiles(files []string) bool {
	for _, f := range files {
		base := filepath.Base(f)
		if base == "" || strings.HasPrefix(base, ".") {
			continue
		}
		switch strings.ToLower(filepath.Ext(f)) {
		case ".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs",
			".py", ".rs", ".java", ".kt", ".swift", ".rb", ".php",
			".c", ".cc", ".cpp", ".h", ".hpp", ".cs", ".vue", ".svelte",
			".css", ".scss", ".html", ".sql", ".toml", ".yaml", ".yml":
			return true
		}
	}
	return false
}

func formatDeltaMessage(d Delta) string {
	var bits []string
	if d.Habit != nil {
		bits = append(bits, "habit: "+d.Habit.Text)
	}
	if d.Playbook != nil {
		verb := "playbook"
		if d.Updated && !d.Created {
			verb = "updated playbook"
		}
		bits = append(bits, verb+": "+d.Playbook.Title)
	}
	if len(bits) == 0 {
		return ""
	}
	prefix := "Remembered"
	if d.Updated && !d.Created {
		prefix = "Updated memory"
	}
	return prefix + " — " + strings.Join(bits, "; ")
}

// LightModel picks a cheap model id for optional LLM reflection.
func LightModel(fallback string, eco string, allowFable bool) string {
	if m, ok := llm.CatalogSnapshot().ModelForTier(llm.Ecosystem(eco), llm.TierLight, allowFable); ok {
		return m.ID
	}
	return fallback
}
