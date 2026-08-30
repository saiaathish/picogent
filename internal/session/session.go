package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/saiaathish/picogent/internal/redact"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
)

type Session struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Workspace string        `json:"workspace"`
	Updated   time.Time     `json:"updated"`
	Messages  []llm.Message `json:"messages"`
}

const (
	MaxSessions = 60

	// MaxSessionBytes is a hard upper bound for one JSON session record. It
	// protects both disk usage and restart-time parsing from unbounded chat
	// history growth.
	MaxSessionBytes = 256 << 10
	// MaxSessionMessages bounds the number of normalized messages retained in
	// one session after a save. A lower byte limit may retain fewer messages.
	MaxSessionMessages = 128

	maxSessionTitleBytes     = 256
	maxSessionWorkspaceBytes = 4096
	maxSessionContentBytes   = 8 << 10
	maxSessionToolBytes      = 4 << 10
	maxSessionPartTextBytes  = 2 << 10
	maxSessionPartDataBytes  = 8 << 10
	maxSessionParts          = 4
	maxSessionToolCalls      = 8
)

// ErrSessionTooLarge reports a session record that cannot be safely loaded or
// represented within the durable session boundary.
var ErrSessionTooLarge = errors.New("session record exceeds size limit")

func deriveTitle(msgs []llm.Message) string {
	for _, m := range msgs {
		if m.Role == "user" {
			t := strings.TrimSpace(m.Content)
			if t != "" {
				if len(t) > 56 {
					return t[:56] + "…"
				}
				return t
			}
		}
	}
	return "New chat"
}

// Meta is a lightweight session summary for list views (no message bodies).
type Meta struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Updated time.Time `json:"updated"`
}

func New(workspace string) *Session {
	now := time.Now().UTC()
	id := now.Format("20060102-150405")
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err == nil {
		id += "-" + hex.EncodeToString(suffix[:])
	} else {
		id += "-" + fmt.Sprintf("%x", now.UnixNano())
	}
	return &Session{
		ID:        id,
		Title:     "New chat",
		Workspace: workspace,
		Updated:   now,
	}
}

func Dir() (string, error) {
	root, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "sessions"), nil
}

func (s *Session) Path() (string, error) {
	if s == nil || !validID(s.ID) {
		return "", errors.New("invalid session id")
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, s.ID+".json"), nil
}

func (s *Session) Save() error {
	if s == nil || !validID(s.ID) {
		return errors.New("invalid session id")
	}
	path, err := s.Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	unlock, err := acquireSessionsLock(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer unlock()
	s.Updated = time.Now().UTC()
	return saveLocked(path, s)
}

func Load(id string) (*Session, error) {
	id = strings.TrimSuffix(strings.TrimSpace(id), ".json")
	if !validID(id) {
		return nil, errors.New("invalid session id")
	}
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, id+".json")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return loadLocked(path, id)
}

func List() ([]Session, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	_, err = os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return listLocked(dir)
}

func listLocked(dir string) ([]Session, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		s, err := loadLocked(filepath.Join(dir, e.Name()), id)
		if err != nil {
			continue
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Updated.After(out[j].Updated)
	})
	return out, nil
}

func ListMeta(workspace string) ([]Meta, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	_, err = os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return listMetaLocked(dir, workspace, MaxSessions)
}

func listMetaLocked(dir, workspace string, limit int) ([]Meta, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	ws, _ := filepath.Abs(workspace)
	var out []Meta
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		s, err := loadLocked(filepath.Join(dir, e.Name()), id)
		if err != nil {
			continue
		}
		sw, _ := filepath.Abs(s.Workspace)
		if ws != "" && sw != ws {
			continue
		}
		title := s.Title
		if title == "" {
			title = deriveTitle(s.Messages)
		}
		out = append(out, Meta{ID: s.ID, Title: title, Updated: s.Updated})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Updated.After(out[j].Updated)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func Prune(workspace string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	all, err := listMetaLocked(dir, workspace, 0)
	if err != nil {
		return err
	}
	if len(all) <= MaxSessions {
		return nil
	}
	for _, m := range all[MaxSessions:] {
		if validID(m.ID) {
			if err := os.Remove(filepath.Join(dir, m.ID+".json")); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func Delete(id string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	id = strings.TrimSuffix(id, ".json")
	if !validID(id) {
		return errors.New("invalid session id")
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return os.ErrNotExist
		}
		return err
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	return os.Remove(filepath.Join(dir, id+".json"))
}

func Latest(workspace string) (*Session, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	ws, _ := filepath.Abs(workspace)
	for _, s := range all {
		if !validID(s.ID) {
			continue
		}
		sw, _ := filepath.Abs(s.Workspace)
		if sw == ws && len(s.Messages) > 0 {
			cp := s
			return &cp, nil
		}
	}
	return nil, os.ErrNotExist
}

func SaveMessages(workspace string, id string, msgs []llm.Message) error {
	return saveMessagesWithTitle(workspace, id, msgs, "")
}

func loadLocked(path, id string) (*Session, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxSessionBytes {
		return nil, ErrSessionTooLarge
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxSessionBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxSessionBytes {
		return nil, ErrSessionTooLarge
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.ID != id {
		return nil, errors.New("session id mismatch")
	}
	if err := boundSession(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveLocked(path string, s *Session) error {
	if err := boundSession(s); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > MaxSessionBytes {
		return ErrSessionTooLarge
	}
	return writeAtomic(path, data)
}

func boundSession(s *Session) error {
	if s == nil {
		return errors.New("session is nil")
	}
	if len(s.Workspace) > maxSessionWorkspaceBytes {
		return ErrSessionTooLarge
	}
	s.Title = sessionText(s.Title, maxSessionTitleBytes)
	s.Messages = boundedMessages(s.Messages, *s)
	return nil
}

func boundedMessages(messages []llm.Message, base Session) []llm.Message {
	if len(messages) == 0 {
		return nil
	}
	normalized := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		// The agent regenerates its system prompt on the next turn. Do not
		// persist that internal prompt into a user-resumable chat record.
		if message.Role == "system" {
			continue
		}
		normalized = append(normalized, boundMessage(message))
	}
	if len(normalized) == 0 {
		return nil
	}

	turns := splitTurns(normalized)
	if len(turns) == 0 {
		return newestUnits(nil, normalized, base)
	}
	selected := make([][]llm.Message, 0, len(turns))
	latest := newestUnits(turns[len(turns)-1], nil, base)
	if len(latest) > 0 {
		selected = append(selected, latest)
	}
	for i := len(turns) - 2; i >= 0 && len(selected) < MaxSessionMessages; i-- {
		candidate := make([][]llm.Message, 0, len(selected)+1)
		candidate = append(candidate, turns[i])
		candidate = append(candidate, selected...)
		flat := flattenTurns(candidate)
		if len(flat) > MaxSessionMessages || !sessionFits(base, flat) {
			break
		}
		selected = candidate
	}
	flat := flattenTurns(selected)
	if len(flat) > MaxSessionMessages {
		flat = newestUnits(nil, flat, base)
	}
	if !sessionFits(base, flat) {
		flat = newestUnits(nil, flat, base)
	}
	if len(flat) > MaxSessionMessages {
		flat = flat[len(flat)-MaxSessionMessages:]
	}
	return flat
}

func boundMessage(message llm.Message) llm.Message {
	message.Role = clipSessionText(message.Role, 32)
	message.Content = sessionText(message.Content, maxSessionContentBytes)
	message.ToolCallID = clipSessionText(message.ToolCallID, 128)
	message.Name = clipSessionText(message.Name, 128)
	if len(message.Parts) > maxSessionParts {
		message.Parts = message.Parts[:maxSessionParts]
	}
	if len(message.Parts) > 0 {
		parts := make([]llm.Part, 0, len(message.Parts))
		for _, part := range message.Parts {
			part.Type = clipSessionText(part.Type, 32)
			part.Text = sessionText(part.Text, maxSessionPartTextBytes)
			part.MIME = clipSessionText(part.MIME, 128)
			part.Name = clipSessionText(part.Name, 256)
			if len(part.Data) > maxSessionPartDataBytes {
				part.Data = nil
				if part.Text == "" {
					part.Text = "[attachment data omitted from saved session]"
				}
			} else if len(part.Data) > 0 {
				part.Data = append([]byte(nil), part.Data...)
			}
			parts = append(parts, part)
		}
		message.Parts = parts
	}
	if len(message.ToolCalls) > maxSessionToolCalls {
		message.ToolCalls = message.ToolCalls[:maxSessionToolCalls]
	}
	if len(message.ToolCalls) > 0 {
		calls := make([]llm.ToolCall, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			call.ID = clipSessionText(call.ID, 128)
			call.ItemID = clipSessionText(call.ItemID, 128)
			call.Name = clipSessionText(call.Name, 128)
			call.Arguments = sessionText(call.Arguments, maxSessionToolBytes)
			calls = append(calls, call)
		}
		message.ToolCalls = calls
	}
	return message
}

// sessionText applies the shared credential redactor before enforcing the
// per-field history bound. A saved transcript can contain user text, model
// text, tool arguments, and tool results, so transcript-bearing values must
// cross the same persistence boundary.
func sessionText(value string, limit int) string {
	return clipSessionText(redact.Text(value), limit)
}

func splitTurns(messages []llm.Message) [][]llm.Message {
	var turns [][]llm.Message
	start := 0
	for i := 1; i < len(messages); i++ {
		if messages[i].Role == "user" {
			turns = append(turns, append([]llm.Message(nil), messages[start:i]...))
			start = i
		}
	}
	if start < len(messages) {
		turns = append(turns, append([]llm.Message(nil), messages[start:]...))
	}
	return turns
}

func newestUnits(turn []llm.Message, fallback []llm.Message, base Session) []llm.Message {
	if len(turn) == 0 {
		turn = fallback
	}
	if len(turn) == 0 {
		return nil
	}
	units := messageUnits(turn)
	if len(units) == 0 {
		return nil
	}
	selected := make([]int, 0, len(units))
	for i := len(units) - 1; i >= 0; i-- {
		candidateIndexes := append(append([]int(nil), selected...), i)
		candidate := flattenUnits(units, candidateIndexes)
		if len(candidate) > MaxSessionMessages || !sessionFits(base, candidate) {
			continue
		}
		selected = append(selected, i)
	}
	return flattenUnits(units, selected)
}

func messageUnits(messages []llm.Message) [][]llm.Message {
	units := make([][]llm.Message, 0, len(messages))
	for i := 0; i < len(messages); {
		// A tool result without its matching assistant tool call cannot be
		// replayed safely, so do not persist it as an orphaned unit.
		if messages[i].Role == "tool" {
			i++
			continue
		}
		end := i + 1
		if messages[i].Role == "assistant" && len(messages[i].ToolCalls) > 0 {
			ids := make(map[string]struct{}, len(messages[i].ToolCalls))
			for _, call := range messages[i].ToolCalls {
				if call.ID != "" {
					ids[call.ID] = struct{}{}
				}
			}
			for end < len(messages) && messages[end].Role == "tool" {
				if _, ok := ids[messages[end].ToolCallID]; !ok {
					break
				}
				end++
			}
		}
		units = append(units, append([]llm.Message(nil), messages[i:end]...))
		i = end
	}
	return units
}

func flattenUnits(units []([]llm.Message), indexes []int) []llm.Message {
	selected := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		selected[index] = struct{}{}
	}
	var out []llm.Message
	for i, unit := range units {
		if _, ok := selected[i]; ok {
			out = append(out, unit...)
		}
	}
	return out
}

func flattenTurns(turns [][]llm.Message) []llm.Message {
	var out []llm.Message
	for _, turn := range turns {
		out = append(out, turn...)
	}
	return out
}

func sessionFits(base Session, messages []llm.Message) bool {
	base.Messages = messages
	data, err := json.MarshalIndent(&base, "", "  ")
	return err == nil && len(data) <= MaxSessionBytes
}

func clipSessionText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	const marker = "\n… [session history truncated] …"
	if limit <= len(marker) {
		return utf8Prefix(value, limit)
	}
	return utf8Prefix(value, limit-len(marker)) + marker
}

func utf8Prefix(value string, limit int) string {
	if limit >= len(value) {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}

func updateSession(id, workspace string, create bool, mutate func(*Session) error) (*Session, error) {
	id = strings.TrimSuffix(strings.TrimSpace(id), ".json")
	if !validID(id) {
		return nil, errors.New("invalid session id")
	}
	if mutate == nil {
		return nil, errors.New("session update callback is required")
	}
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	path := filepath.Join(dir, id+".json")
	if !create {
		if _, err := os.Stat(path); err != nil {
			return nil, err
		}
	}
	unlock, err := acquireSessionsLock(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()

	s, err := loadLocked(path, id)
	if err != nil {
		if !create || !os.IsNotExist(err) {
			return nil, err
		}
		s = &Session{ID: id, Title: "New chat", Workspace: workspace}
	}
	if s.Workspace == "" {
		s.Workspace = workspace
	}
	if err := mutate(s); err != nil {
		return nil, err
	}
	s.ID = id
	s.Updated = time.Now().UTC()
	if err := saveLocked(path, s); err != nil {
		return nil, err
	}
	return s, nil
}

func validID(id string) bool {
	if id == "" || id == "." || id == ".." || len(id) > 200 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}
