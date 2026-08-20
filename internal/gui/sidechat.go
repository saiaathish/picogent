package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/projects"
)

const sideSystemPrompt = `You are PicoChat Companion — a lightweight side assistant for Picogent (like Codex side chat).

You help with the project, the current main chat, running-task status/ETA, and how to use Picogent. You are NOT the coding agent: you never edit files or run tools.

Rules:
1. Use the LIVE STATUS block as ground truth for busy/idle, goal, task mode, elapsed time, and recent activity.
2. When asked about ETA or "how long", give an honest approximate estimate from elapsed time + activity.
3. Summarize the current main chat briefly when asked what's going on.
4. Answer product/how-to questions using PRODUCT GUIDE excerpts when relevant.
5. Be concise. Prefer short Markdown. LaTeX is fine when math helps ($...$ or $$...$$).
6. If the main agent is waiting on permission, say so clearly.
7. Never invent file edits or tool results that are not in LIVE STATUS / MAIN CHAT.`

type sideStatus struct {
	Busy         bool     `json:"busy"`
	SideBusy     bool     `json:"side_busy"`
	TaskMode     string   `json:"task_mode"`
	Goal         string   `json:"goal,omitempty"`
	Workspace    string   `json:"workspace"`
	Project      string   `json:"project"`
	Model        string   `json:"model"`
	SessionID    string   `json:"session_id"`
	TurnPrompt   string   `json:"turn_prompt,omitempty"`
	ElapsedSec   int      `json:"elapsed_sec,omitempty"`
	ETA          string   `json:"eta,omitempty"`
	Reads        int      `json:"reads"`
	Searches     int      `json:"searches"`
	Edits        int      `json:"edits"`
	PendingPerm  string   `json:"pending_perm,omitempty"`
	ChatSummary  string   `json:"chat_summary,omitempty"`
	ActivityLine string   `json:"activity_line,omitempty"`
	Starters     []string `json:"starters"`
}

func (s *server) sidechatAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"messages": s.sideMessages(),
			"prompts":  s.getPromptRecs("side", false),
		})
	case http.MethodPost:
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			http.Error(w, "empty prompt", 400)
			return
		}
		s.mu.Lock()
		if s.sideBusy {
			s.mu.Unlock()
			http.Error(w, "side chat busy", 409)
			return
		}
		s.sideBusy = true
		s.sideHist = append(s.sideHist, llm.Message{Role: "user", Content: prompt})
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		go s.runSideChat(prompt)
	default:
		http.Error(w, "GET or POST", 405)
	}
}

func (s *server) sideMessages() []transcriptLine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return messagesToTranscript(s.sideHist)
}

func (s *server) buildSideStatus() sideStatus {
	s.mu.Lock()
	cfg := s.cfg
	busy := s.busy
	sideBusy := s.sideBusy
	sessionID := s.sessionID
	hist := append([]llm.Message(nil), s.hist...)
	liveTask := s.liveTask
	turnPrompt := s.turnPrompt
	turnStarted := s.turnStarted
	reads, searches, edits := s.turnReads, s.turnSearches, s.turnEdits
	pend := s.pendingPerm
	ag := s.ag
	s.mu.Unlock()

	taskMode := string(liveTask)
	if taskMode == "" && ag != nil {
		taskMode = string(ag.TaskMode)
	}
	if taskMode == "" {
		taskMode = cfg.TaskMode
	}
	g, _ := goal.Load(cfg.Workspace)
	st := sideStatus{
		Busy:       busy,
		SideBusy:   sideBusy,
		TaskMode:   taskMode,
		Goal:       g,
		Workspace:  cfg.Workspace,
		Project:    projects.NameFromPath(cfg.Workspace),
		Model:      cfg.DisplayModel(),
		SessionID:  sessionID,
		TurnPrompt: turnPrompt,
		Reads:      reads,
		Searches:   searches,
		Edits:      edits,
		Starters: []string{
			"What's the agent doing?",
			"How long will this take?",
			"Summarize this chat",
			"What's my goal?",
			"How do Safe and Fast differ?",
		},
	}
	if pend.Tool != "" {
		st.PendingPerm = pend.Tool + ": " + pend.Summary
	}
	if busy && !turnStarted.IsZero() {
		st.ElapsedSec = int(time.Since(turnStarted).Seconds())
	}
	st.ActivityLine = formatActivityLine(reads, searches, edits)
	st.ETA = estimateETA(busy, st.ElapsedSec, reads, searches, edits, turnPrompt)
	st.ChatSummary = summarizeMainChat(hist, 6)
	return st
}

func formatActivityLine(reads, searches, edits int) string {
	parts := make([]string, 0, 3)
	if reads > 0 {
		parts = append(parts, fmt.Sprintf("%d read%s", reads, pluralS(reads)))
	}
	if searches > 0 {
		parts = append(parts, fmt.Sprintf("%d search%s", searches, pluralES(searches)))
	}
	if edits > 0 {
		parts = append(parts, fmt.Sprintf("%d edit%s", edits, pluralS(edits)))
	}
	if len(parts) == 0 {
		return "No tool activity yet this turn"
	}
	return strings.Join(parts, " · ")
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func pluralES(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

func estimateETA(busy bool, elapsed, reads, searches, edits int, prompt string) string {
	if !busy {
		return "Idle — no running turn"
	}
	if elapsed < 8 && reads+searches+edits == 0 {
		return "Just started — usually under a minute for a first reply, longer if tools are needed"
	}
	p := strings.ToLower(prompt)
	heavy := strings.Contains(p, "refactor") || strings.Contains(p, "migrate") ||
		strings.Contains(p, "implement") || strings.Contains(p, "fix all") ||
		strings.Contains(p, "test") || strings.Contains(p, "debug")
	switch {
	case edits > 0 && elapsed > 20:
		return fmt.Sprintf("~%d–%d more minutes (already editing)", max(1, 2-elapsed/60), max(2, 4-elapsed/60))
	case heavy && elapsed < 90:
		return "Roughly 2–6 minutes for a heavier coding/debug turn"
	case reads+searches > 8:
		return "Still exploring — often 1–3 more minutes before a solid answer"
	case elapsed > 180:
		return "This turn is running long — it may be stuck on tools or waiting for permission"
	default:
		remain := 60 - elapsed
		if remain < 20 {
			remain = 30
		}
		return fmt.Sprintf("About %d–%d seconds if it stays light; a few minutes if it keeps using tools", remain, remain+90)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func summarizeMainChat(hist []llm.Message, maxTurns int) string {
	if len(hist) == 0 {
		return "Main chat is empty."
	}
	var lines []string
	count := 0
	for i := len(hist) - 1; i >= 0 && count < maxTurns; i-- {
		m := hist[i]
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		text := strings.TrimSpace(m.Content)
		if text == "" {
			continue
		}
		if len(text) > 220 {
			text = text[:217] + "…"
		}
		lines = append([]string{fmt.Sprintf("%s: %s", m.Role, text)}, lines...)
		count++
	}
	return strings.Join(lines, "\n")
}

func (s *server) runSideChat(prompt string) {
	defer func() {
		s.mu.Lock()
		s.sideBusy = false
		s.mu.Unlock()
		s.emit(event{Type: "side_done"})
		// Refresh side chips after each exchange.
		go func() {
			_ = s.getPromptRecs("side", true)
			s.emit(event{Type: "prompts_refresh", Text: "side"})
		}()
	}()

	ctxBlock := s.sideContextBlock()
	s.mu.Lock()
	hist := append([]llm.Message(nil), s.sideHist...)
	s.mu.Unlock()

	msgs := []llm.Message{
		{Role: "system", Content: sideSystemPrompt + "\n\n" + ctxBlock},
	}
	for _, m := range hist {
		if m.Role == "user" || m.Role == "assistant" {
			msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
		}
	}

	answer, err := s.lightChat(context.Background(), msgs, true)
	if err != nil {
		answer = s.sideFallbackAnswer(prompt)
		if answer == "" {
			answer = "PicoChat couldn’t reach the model right now.\n\n" + s.deterministicStatusAnswer()
		}
	}

	s.mu.Lock()
	s.sideHist = append(s.sideHist, llm.Message{Role: "assistant", Content: answer})
	if len(s.sideHist) > 24 {
		s.sideHist = s.sideHist[len(s.sideHist)-24:]
	}
	s.mu.Unlock()
	s.emit(event{Type: "side", Text: answer})
}

func (s *server) sideContextBlock() string {
	st := s.buildSideStatus()
	var b strings.Builder
	b.WriteString("LIVE STATUS\n")
	fmt.Fprintf(&b, "project: %s (%s)\n", st.Project, st.Workspace)
	fmt.Fprintf(&b, "model: %s\n", st.Model)
	fmt.Fprintf(&b, "busy: %v\n", st.Busy)
	fmt.Fprintf(&b, "task_mode: %s\n", st.TaskMode)
	if st.Goal != "" {
		fmt.Fprintf(&b, "goal: %s\n", st.Goal)
	}
	if st.TurnPrompt != "" {
		fmt.Fprintf(&b, "current_turn: %s\n", st.TurnPrompt)
	}
	if st.Busy {
		fmt.Fprintf(&b, "elapsed_sec: %d\n", st.ElapsedSec)
	}
	fmt.Fprintf(&b, "activity: %s\n", st.ActivityLine)
	fmt.Fprintf(&b, "eta_hint: %s\n", st.ETA)
	if st.PendingPerm != "" {
		fmt.Fprintf(&b, "pending_permission: %s\n", st.PendingPerm)
	}
	b.WriteString("\nMAIN CHAT (recent)\n")
	b.WriteString(st.ChatSummary)
	b.WriteString("\n\nPRODUCT GUIDE (excerpts)\n")
	b.WriteString(sideGuideExcerpt(promptKeywords(st.TurnPrompt)))
	return b.String()
}

func (s *server) sideFallbackAnswer(prompt string) string {
	docs, err := loadHelpDocs()
	if err == nil {
		ans := answerHelp(docs, prompt)
		if ans.Matched && !strings.Contains(strings.ToLower(prompt), "how long") &&
			!strings.Contains(strings.ToLower(prompt), "status") &&
			!strings.Contains(strings.ToLower(prompt), "doing") {
			return ans.Answer
		}
	}
	return s.deterministicStatusAnswer()
}

func (s *server) deterministicStatusAnswer() string {
	st := s.buildSideStatus()
	var b strings.Builder
	if st.Busy {
		fmt.Fprintf(&b, "Main agent is **busy**")
		if st.TurnPrompt != "" {
			fmt.Fprintf(&b, " on: %s", st.TurnPrompt)
		}
		b.WriteString(".\n")
		fmt.Fprintf(&b, "- Elapsed: ~%ds\n", st.ElapsedSec)
		fmt.Fprintf(&b, "- Activity: %s\n", st.ActivityLine)
		fmt.Fprintf(&b, "- ETA: %s\n", st.ETA)
	} else {
		b.WriteString("Main agent is **idle** — no turn running.\n")
	}
	if st.Goal != "" {
		fmt.Fprintf(&b, "- Goal: %s\n", st.Goal)
	}
	fmt.Fprintf(&b, "- Project: %s\n", st.Project)
	fmt.Fprintf(&b, "- Mode: %s · model %s\n", st.TaskMode, st.Model)
	if st.PendingPerm != "" {
		fmt.Fprintf(&b, "- Waiting on permission: %s\n", st.PendingPerm)
	}
	return strings.TrimSpace(b.String())
}

func promptKeywords(s string) string {
	return strings.ToLower(s)
}

func sideGuideExcerpt(q string) string {
	docs, err := loadHelpDocs()
	if err != nil || len(docs.Topics) == 0 {
		return "Picogent is a tiny coding agent. Safe asks before writes; Fast auto-edits in-workspace."
	}
	scored := scoreHelpTopics(docs.Topics, q+" status mode model slash mcp")
	var parts []string
	n := 0
	for _, sc := range scored {
		if n >= 3 {
			break
		}
		body := sc.topic.Body
		if len(body) > 700 {
			body = body[:697] + "…"
		}
		parts = append(parts, body)
		n++
	}
	if len(parts) == 0 {
		parts = append(parts, docs.Topics[0].Body)
	}
	return strings.Join(parts, "\n\n")
}

func (s *server) noteTurnStart(prompt string) {
	s.mu.Lock()
	s.turnStarted = time.Now()
	s.turnPrompt = strings.TrimSpace(prompt)
	s.turnReads, s.turnSearches, s.turnEdits = 0, 0, 0
	s.mu.Unlock()
}

func (s *server) noteTurnActivity(kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch kind {
	case "read":
		s.turnReads++
	case "search":
		s.turnSearches++
	case "edit":
		s.turnEdits++
	}
}

func (s *server) clearTurnProgress() {
	s.mu.Lock()
	s.turnStarted = time.Time{}
	s.turnPrompt = ""
	s.mu.Unlock()
}

// projectLeaf is a tiny helper for tests / display.
func projectLeaf(path string) string {
	return filepath.Base(path)
}
