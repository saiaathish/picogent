package extensions

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/mcpbridge"
)

// Pool manages on-demand extension activation — only loads what's needed per task.
type Pool struct {
	Workspace     string
	Essential     []string
	Transient     []string
	activatedUndo []UndoEntry
}

// NewPool creates a pool from config lists.
func NewPool(workspace string, essential, transient []string) *Pool {
	return &Pool{
		Workspace: workspace,
		Essential: append([]string(nil), essential...),
		Transient: append([]string(nil), transient...),
	}
}

// EnsureForPrompt is retained as a compatibility seam for callers that used
// to ask the pool to auto-load recommendations. Turn preparation must remain
// read-only: even a skill recommendation may require a Git clone and a config
// write, while an MCP activation may execute a command or contact an endpoint.
// Explicit extension actions are the only path that may call activate.
func (p *Pool) EnsureForPrompt(prompt string) ([]string, error) {
	if p == nil {
		return nil, nil
	}
	installed, err := InstalledSet(p.Workspace, nil)
	if err != nil {
		return nil, err
	}
	// Reset stale rollback bookkeeping without touching the filesystem. Rollback
	// entries are created only by explicit activation callers.
	p.activatedUndo = nil
	dismissed := map[string]bool{}
	recs := Recommend(prompt, installed, dismissed)

	// Search only an already-loaded/local Claude cache. This call intentionally
	// has no network or persistence fallback.
	claudeItems, _ := LoadClaudeLibraryCached()
	lower := strings.ToLower(prompt)
	for _, it := range claudeItems {
		if matchScore(lower, it.Keywords) >= 8 && !installed[it.ID] {
			recs = append(recs, Item{
				ID: it.ID, Name: it.Name, Kind: it.Kind,
				Description: it.Description, Keywords: it.Keywords,
			})
		}
	}
	// Keep the recommendation calculation here so callers receive the same
	// local-only discovery/error behavior as the GUI path. The old method
	// returned activated IDs, but reporting a recommendation as activated would
	// make callers persist state that was never installed.
	_ = recs
	return nil, nil
}

func autoActivationAllowed(it Item) bool {
	if strings.HasPrefix(it.ID, "claude:") {
		return false
	}
	return it.Kind == KindSkill
}

// RollbackActivated restores the external state changed by the most recent
// EnsureForPrompt call. It is used when a runtime replacement is rejected
// after activation, so an old skill or MCP entry is never deleted as a side
// effect of abandoning a candidate.
func (p *Pool) RollbackActivated() error {
	if p == nil {
		return nil
	}
	var errs []error
	remaining := make([]UndoEntry, 0, len(p.activatedUndo))
	for i := len(p.activatedUndo) - 1; i >= 0; i-- {
		entry := p.activatedUndo[i]
		// Undo may close its value's snapshot after a successful restore. Use an
		// owned clone so a retained entry remains a valid retry record if the
		// restore fails or a caller needs to inspect it after this attempt.
		if err := Undo(entry.Clone()); err != nil {
			errs = append(errs, err)
			remaining = append(remaining, entry)
		} else {
			p.Transient = removeExtensionID(p.Transient, entry.ExtID)
		}
	}
	// Keep failed entries so a caller can retry recovery or surface the exact
	// still-mutated state instead of making a one-shot rollback irreversible.
	for i, j := 0, len(remaining)-1; i < j; i, j = i+1, j-1 {
		remaining[i], remaining[j] = remaining[j], remaining[i]
	}
	p.activatedUndo = remaining
	return errors.Join(errs...)
}

func removeExtensionID(list []string, id string) []string {
	out := list[:0]
	for _, current := range list {
		if current != id {
			out = append(out, current)
		}
	}
	return out
}

// CleanupTransient deactivates extensions that aren't essential.
func (p *Pool) CleanupTransient() error {
	snapshot, err := CaptureState(p.Workspace, p.Transient)
	if err != nil {
		return err
	}
	defer snapshot.Close()
	var keep []string
	for _, id := range p.Transient {
		if p.isEssential(id) {
			keep = append(keep, id)
			continue
		}
		if err := p.deactivate(id); err != nil {
			rollbackErr := snapshot.Restore()
			return errors.Join(err, rollbackErr)
		}
	}
	p.Transient = keep
	return nil
}

func (p *Pool) isEssential(id string) bool {
	for _, e := range p.Essential {
		if e == id {
			return true
		}
	}
	return false
}

func (p *Pool) activate(it Item) (UndoEntry, error) {
	if strings.HasPrefix(it.ID, "claude:") {
		before, err := CaptureState(p.Workspace, []string{it.ID})
		if err != nil {
			return UndoEntry{}, err
		}
		if err := ActivateClaudePlugin(strings.TrimPrefix(it.ID, "claude:")); err != nil {
			rollbackErr := before.Restore()
			before.Close()
			return UndoEntry{}, errors.Join(err, rollbackErr)
		}
		return UndoEntry{ID: fmt.Sprintf("pool-%d", time.Now().UnixNano()), ExtID: it.ID, Kind: it.Kind, before: before}, nil
	}
	if it.Kind == KindMCP && it.MCP != nil {
		_, entry, err := Install(it, p.Workspace)
		return entry, err
	}
	if it.Kind == KindSkill {
		_, entry, err := Install(it, p.Workspace)
		return entry, err
	}
	return UndoEntry{ID: fmt.Sprintf("pool-%d", time.Now().UnixNano()), ExtID: it.ID, Kind: it.Kind}, nil
}

func (p *Pool) deactivate(id string) error {
	if strings.HasPrefix(id, "claude:") {
		name := strings.TrimPrefix(id, "claude:")
		return removeClaudePluginMCP(name)
	}
	it := ByID(id)
	if it == nil {
		return nil
	}
	if it.Kind == KindMCP {
		return mcpbridge.RemoveServer(mcpServerName(*it))
	}
	if it.Kind == KindSkill && it.SkillPath != "" {
		return removeSkill(it.SkillPath)
	}
	return nil
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// MarkEssential adds an extension to the always-keep list.
func MarkEssential(list []string, id string) []string {
	return appendUnique(list, id)
}

// ActiveStatus annotates browse results with pool state.
func ActiveStatus(items []SearchResult, essential, transient []string) []SearchResult {
	ess := map[string]bool{}
	tran := map[string]bool{}
	for _, id := range essential {
		ess[id] = true
	}
	for _, id := range transient {
		tran[id] = true
	}
	out := make([]SearchResult, len(items))
	for i, it := range items {
		out[i] = it
		out[i].Essential = ess[it.ID]
		out[i].Active = ess[it.ID] || tran[it.ID] || it.Installed
	}
	return out
}
