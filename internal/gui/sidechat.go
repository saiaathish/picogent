package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
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

func (s *server) sidechatAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"messages": s.sideMessages(),
			"prompts":  s.cachedPromptRecs("side"),
		})
	case http.MethodPost:
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeGUIError(w, "bad json", 400)
			return
		}
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			writeGUIError(w, "empty prompt", 400)
			return
		}
		s.mu.Lock()
		if s.sideBusy {
			s.mu.Unlock()
			writeGUIError(w, "side chat busy", 409)
			return
		}
		s.sideBusy = true
		s.sideHist = append(s.sideHist, llm.Message{Role: "user", Content: prompt})
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		go s.runSideChat(prompt)
	default:
		writeGUIError(w, "GET or POST", 405)
	}
}

func (s *server) sideMessages() []transcriptLine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return messagesToTranscript(s.sideHist)
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
	st := s.buildAgentStatus()
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
	st := s.buildAgentStatus()
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
