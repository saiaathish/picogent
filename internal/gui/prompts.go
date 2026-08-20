package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/learn"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/projects"
)

type promptRec struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Prompt   string `json:"prompt"`
}

func (s *server) promptsAPI(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "main"
	}
	switch r.Method {
	case http.MethodGet:
		force := r.URL.Query().Get("refresh") == "1"
		recs := s.getPromptRecs(kind, force)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"kind":    kind,
			"prompts": recs,
		})
	case http.MethodPost:
		var req struct {
			Kind string `json:"kind"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Kind == "" {
			req.Kind = kind
		}
		recs := s.getPromptRecs(req.Kind, true)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"kind":    req.Kind,
			"prompts": recs,
		})
	default:
		http.Error(w, "GET or POST", 405)
	}
}

func (s *server) getPromptRecs(kind string, force bool) []promptRec {
	s.mu.Lock()
	ws := s.cfg.Workspace
	sid := s.sessionID
	var cached []promptRec
	var cachedAt time.Time
	if kind == "side" {
		cached = append([]promptRec(nil), s.sideRecs...)
		cachedAt = s.sideRecsAt
	} else {
		cached = append([]promptRec(nil), s.mainRecs...)
		cachedAt = s.mainRecsAt
	}
	s.mu.Unlock()

	if !force && len(cached) > 0 && time.Since(cachedAt) < 12*time.Minute {
		return cached
	}

	recs := s.generatePromptRecs(kind)
	s.mu.Lock()
	if kind == "side" {
		s.sideRecs = recs
		s.sideRecsAt = time.Now()
	} else {
		s.mainRecs = recs
		s.mainRecsAt = time.Now()
	}
	s.recsKey = ws + "|" + sid
	s.mu.Unlock()
	return recs
}

func (s *server) invalidatePromptRecs() {
	s.mu.Lock()
	s.mainRecs = nil
	s.sideRecs = nil
	s.mainRecsAt = time.Time{}
	s.sideRecsAt = time.Time{}
	s.recsKey = ""
	s.mu.Unlock()
}

func (s *server) generatePromptRecs(kind string) []promptRec {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	ctxBlock := s.promptRecContext(kind)
	raw, err := s.lightChat(ctx, []llm.Message{
		{Role: "system", Content: promptRecSystem(kind)},
		{Role: "user", Content: "Repo context:\n" + ctxBlock + "\n\nReturn JSON only."},
	}, false)
	if err == nil {
		if recs := parsePromptRecs(raw); len(recs) > 0 {
			return recs
		}
	}
	return heuristicPromptRecs(kind, ctxBlock, s.buildSideStatus())
}

func promptRecSystem(kind string) string {
	n := 4
	focus := "actionable coding tasks the user might want next in the main chat"
	if kind == "side" {
		n = 4
		focus = "short companion questions about status, the current goal/chat, or how to use Picogent for this repo"
	}
	return fmt.Sprintf(`You recommend the next prompts a developer should try — like “recommended for you” on a streaming app, but for coding tasks.

Return ONLY a JSON array of %d objects:
[{"title":"3-5 words","subtitle":"one short line","prompt":"full prompt the user would send"}]

Rules:
- Ground suggestions in the repo context (languages, files, goal, recent chat).
- No generic “Overview” / “Run tests” / “Review” unless the repo clearly needs that right now.
- Titles must be distinct and concrete.
- Prompts must be ready to send as-is.
- Focus: %s.
- Keep each prompt under 160 characters.`, n, focus)
}

func (s *server) promptRecContext(kind string) string {
	st := s.buildSideStatus()
	store, _ := learn.Load(st.Workspace)
	var b strings.Builder
	fmt.Fprintf(&b, "project: %s\nworkspace: %s\n", st.Project, st.Workspace)
	fmt.Fprintf(&b, "busy: %v task_mode: %s\n", st.Busy, st.TaskMode)
	if st.Goal != "" {
		fmt.Fprintf(&b, "goal: %s\n", st.Goal)
	}
	if st.TurnPrompt != "" {
		fmt.Fprintf(&b, "current_turn: %s\n", st.TurnPrompt)
	}
	if store.Knowledge > 0 {
		fmt.Fprintf(&b, "knowledge: %d%%\n", store.Knowledge)
	}
	if len(store.Overview) > 0 {
		b.WriteString("overview:\n")
		for i, line := range store.Overview {
			if i >= 8 {
				break
			}
			b.WriteString("- " + line + "\n")
		}
	}
	b.WriteString("top_files:\n")
	for _, f := range peekRepoFiles(st.Workspace, 14) {
		b.WriteString("- " + f + "\n")
	}
	if kind == "side" || kind == "main" {
		b.WriteString("\nrecent_chat:\n")
		b.WriteString(st.ChatSummary)
	}
	return b.String()
}

func peekRepoFiles(workspace string, limit int) []string {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			out = append(out, name+"/")
		} else {
			out = append(out, name)
		}
		if len(out) >= limit {
			break
		}
	}
	// Prefer interesting roots if present.
	for _, want := range []string{"README.md", "go.mod", "package.json", "Cargo.toml", "pyproject.toml", "cmd/", "src/", "internal/"} {
		path := filepath.Join(workspace, strings.TrimSuffix(want, "/"))
		if _, err := os.Stat(path); err == nil {
			found := false
			for _, x := range out {
				if x == want || x == strings.TrimSuffix(want, "/") || x == want+"/" {
					found = true
					break
				}
			}
			if !found {
				out = append([]string{want}, out...)
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func parsePromptRecs(raw string) []promptRec {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	// Strip markdown fences if the model wraps JSON.
	if i := strings.Index(raw, "["); i >= 0 {
		if j := strings.LastIndex(raw, "]"); j > i {
			raw = raw[i : j+1]
		}
	}
	var recs []promptRec
	if err := json.Unmarshal([]byte(raw), &recs); err != nil {
		return nil
	}
	out := make([]promptRec, 0, len(recs))
	seen := map[string]bool{}
	for _, r := range recs {
		r.Title = strings.TrimSpace(r.Title)
		r.Subtitle = strings.TrimSpace(r.Subtitle)
		r.Prompt = strings.TrimSpace(r.Prompt)
		if r.Title == "" || r.Prompt == "" {
			continue
		}
		key := strings.ToLower(r.Title)
		if seen[key] {
			continue
		}
		seen[key] = true
		if len(r.Title) > 42 {
			r.Title = r.Title[:39] + "…"
		}
		if len(r.Subtitle) > 72 {
			r.Subtitle = r.Subtitle[:69] + "…"
		}
		out = append(out, r)
		if len(out) >= 5 {
			break
		}
	}
	return out
}

func heuristicPromptRecs(kind string, ctx string, st sideStatus) []promptRec {
	lower := strings.ToLower(ctx + " " + st.Goal + " " + st.ChatSummary)
	var out []promptRec
	if kind == "side" {
		if st.Busy {
			out = append(out, promptRec{Title: "Current turn", Subtitle: "What is running right now", Prompt: "What's the agent doing right now?"})
			out = append(out, promptRec{Title: "Time left", Subtitle: "Rough ETA for this turn", Prompt: "How long will this take?"})
		} else if st.Goal != "" {
			out = append(out, promptRec{Title: "Goal status", Subtitle: "Where we are vs the goal", Prompt: "What's left to finish my goal?"})
		} else {
			out = append(out, promptRec{Title: "Chat summary", Subtitle: "Catch me up", Prompt: "Summarize this chat in a few bullets."})
		}
		out = append(out, promptRec{Title: "Safe vs Fast", Subtitle: "Permission modes", Prompt: "How do Safe and Fast differ in Picogent?"})
		out = append(out, promptRec{Title: "Next move", Subtitle: "What should I ask next", Prompt: "Based on this project, what should I ask the main agent to do next?"})
		return out[:min(4, len(out))]
	}

	if strings.Contains(lower, "go.mod") || strings.Contains(lower, ".go") {
		out = append(out, promptRec{Title: "Go tests", Subtitle: "Find and fix failures", Prompt: "Run the Go tests and fix anything that fails."})
	}
	if strings.Contains(lower, "package.json") {
		out = append(out, promptRec{Title: "Script check", Subtitle: "What npm scripts matter", Prompt: "List the important package.json scripts and what I should run first."})
	}
	if st.Goal != "" {
		out = append(out, promptRec{Title: "Finish goal", Subtitle: "Keep going on the active goal", Prompt: "Continue working on my current goal until it's done."})
	}
	if g, _ := goal.Load(st.Workspace); g != "" && st.Goal == "" {
		out = append(out, promptRec{Title: "Resume goal", Subtitle: g, Prompt: "Resume my goal: " + g})
	}
	name := projects.NameFromPath(st.Workspace)
	out = append(out, promptRec{
		Title:    "Repo pulse",
		Subtitle: "What matters in " + name,
		Prompt:   "Based on the files in this repo, tell me the highest-leverage thing to improve next and start on it.",
	})
	out = append(out, promptRec{
		Title:    "Quick win",
		Subtitle: "Small useful change",
		Prompt:   "Find one small, high-value improvement in this codebase and implement it.",
	})
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *server) lightModelID() string {
	s.mu.Lock()
	eco := llm.Ecosystem(s.cfg.RouterEcosystem())
	allow := s.cfg.FableAllowed()
	fallback := s.cfg.Model
	s.mu.Unlock()
	if m, ok := llm.CatalogSnapshot().ModelForTier(eco, llm.TierLight, allow); ok {
		return m.ID
	}
	return fallback
}

func (s *server) lightClient() llm.Client {
	s.mu.Lock()
	ag := s.ag
	s.mu.Unlock()
	if ag == nil || ag.LLM == nil {
		return nil
	}
	if r, ok := ag.LLM.(*llm.Router); ok && r.Backend != nil {
		return r.Backend
	}
	return ag.LLM
}

// lightChat calls the most token-efficient model directly (no router escalation).
func (s *server) lightChat(ctx context.Context, msgs []llm.Message, stream bool) (string, error) {
	client := s.lightClient()
	if client == nil {
		return "", fmt.Errorf("no llm")
	}
	model := s.lightModelID()
	var buf strings.Builder
	req := llm.ChatRequest{
		Model:     model,
		Messages:  msgs,
		TaskMode:  "ask",
		ReadOnly:  true,
		Reasoning: llm.ReasonNone,
	}
	if stream {
		req.OnDelta = func(delta string) {
			if delta == "" {
				return
			}
			buf.WriteString(delta)
			s.emit(event{Type: "side_delta", Text: delta})
		}
	}
	resp, err := client.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(resp.Message.Content)
	if text == "" {
		text = strings.TrimSpace(buf.String())
	}
	if text == "" {
		return "", fmt.Errorf("empty reply")
	}
	return text, nil
}
