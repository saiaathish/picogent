package gui

import (
	"fmt"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/projects"
)

type agentStatus struct {
	Busy         bool   `json:"busy"`
	TaskMode     string `json:"task_mode"`
	Goal         string `json:"goal,omitempty"`
	Workspace    string `json:"workspace"`
	Project      string `json:"project"`
	Model        string `json:"model"`
	SessionID    string `json:"session_id"`
	TurnPrompt   string `json:"turn_prompt,omitempty"`
	ElapsedSec   int    `json:"elapsed_sec,omitempty"`
	ETA          string `json:"eta,omitempty"`
	Reads        int    `json:"reads"`
	Searches     int    `json:"searches"`
	Edits        int    `json:"edits"`
	PendingPerm  string `json:"pending_perm,omitempty"`
	ChatSummary  string `json:"chat_summary,omitempty"`
	ActivityLine string `json:"activity_line,omitempty"`
}

func (s *server) buildAgentStatus() agentStatus {
	s.mu.Lock()
	cfg := s.cfg
	busy := s.busy
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
		taskMode = string(ag.TaskModeSnapshot())
	}
	if taskMode == "" {
		taskMode = cfg.TaskMode
	}
	g, _ := goal.Load(cfg.Workspace)
	st := agentStatus{
		Busy:       busy,
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
