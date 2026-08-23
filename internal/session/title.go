package session

import (
	"context"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/llm"
)

const titlePrompt = `Write a very short chat title (2–5 words, lowercase ok).
Examples: "chat persistence fix", "browser mcp safety", "latex rendering".
Reply with ONLY the title — no quotes, no punctuation at the end.`

// NeedsAutoTitle reports whether a session still has a generic title.
func NeedsAutoTitle(s *Session) bool {
	if s == nil {
		return false
	}
	if s.Title != "" && s.Title != "New chat" {
		derived := deriveTitle(s.Messages)
		if s.Title != derived {
			return false
		}
	}
	for _, m := range s.Messages {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			return true
		}
	}
	return false
}

// GenerateTitle asks the model for a brief label from the first exchange.
func GenerateTitle(ctx context.Context, client llm.Client, model string, msgs []llm.Message) (string, error) {
	if client == nil {
		return "", nil
	}
	var user, assistant string
	for _, m := range msgs {
		switch m.Role {
		case "user":
			if user == "" {
				user = strings.TrimSpace(m.Content)
			}
		case "assistant":
			if assistant == "" && strings.TrimSpace(m.Content) != "" {
				assistant = strings.TrimSpace(m.Content)
			}
		}
		if user != "" && assistant != "" {
			break
		}
	}
	if user == "" {
		return "", nil
	}
	body := "User: " + clipTitleInput(user, 500)
	if assistant != "" {
		body += "\nAssistant: " + clipTitleInput(assistant, 300)
	}
	out, err := client.Chat(ctx, llm.ChatRequest{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Content: titlePrompt},
			{Role: "user", Content: body},
		},
	})
	if err != nil {
		return "", err
	}
	title := sanitizeTitle(out.Message.Content)
	if title == "" {
		return deriveTitle(msgs), nil
	}
	return title, nil
}

func sanitizeTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 48 {
		s = s[:45] + "…"
	}
	return s
}

func clipTitleInput(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// SetTitle saves an AI-generated title for a session.
func SetTitle(id, title string) error {
	s, err := Load(id)
	if err != nil {
		return err
	}
	title = sanitizeTitle(title)
	if title == "" {
		return nil
	}
	s.Title = title
	s.Updated = time.Now().UTC()
	return s.Save()
}

// SaveMessages preserves AI titles when saving history.
func saveMessagesWithTitle(workspace string, id string, msgs []llm.Message, titleOverride string) error {
	id = strings.TrimSuffix(strings.TrimSpace(id), ".json")
	s := &Session{ID: id, Workspace: workspace, Messages: msgs}
	if id == "" {
		s = New(workspace)
	}
	if prev, err := Load(s.ID); err == nil {
		s.Title = prev.Title
		s.Workspace = prev.Workspace
		if s.Workspace == "" {
			s.Workspace = workspace
		}
	}
	if titleOverride != "" {
		s.Title = sanitizeTitle(titleOverride)
	} else if s.Title == "" || s.Title == "New chat" {
		s.Title = deriveTitle(msgs)
	}
	s.Messages = msgs
	s.Updated = time.Now().UTC()
	if err := s.Save(); err != nil {
		return err
	}
	return Prune(workspace)
}
