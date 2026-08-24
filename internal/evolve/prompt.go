package evolve

import (
	"fmt"
	"sort"
	"strings"
)

// Hard token budget for the learned-memory system-prompt section.
// ~180 tokens worst case — self-evolution must not eat the harness savings.
const (
	maxPromptBytes  = 720
	maxPromptHabits = 3
	maxPromptBooks  = 1
	maxPromptHabit  = 96
	maxPromptBody   = 240
)

// Prompt builds a tiny system-prompt section (no relevance filter).
func Prompt(s Store) string {
	return PromptFor(s, "")
}

// PromptFor injects only the most relevant, size-capped memory for this turn.
func PromptFor(s Store, userHint string) string {
	habits := pickHabits(s, maxPromptHabits)
	books := pickPlaybooks(s, userHint, maxPromptBooks)
	failures := pickFailures(s, userHint, 1)
	routes := pickRoutes(s, userHint, 1)

	if len(habits) == 0 && len(books) == 0 && len(failures) == 0 && len(routes) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Learned (follow quietly; do not narrate):")
	if len(habits) > 0 {
		b.WriteString("\nHabits:")
		for _, h := range habits {
			line := "\n- " + clip(h.Text, maxPromptHabit)
			if b.Len()+len(line) > maxPromptBytes {
				break
			}
			b.WriteString(line)
		}
	}
	if len(books) > 0 {
		header := "\nPlaybook:"
		if b.Len()+len(header) < maxPromptBytes {
			b.WriteString(header)
			for _, p := range books {
				block := fmt.Sprintf("\n### %s\n%s", clip(p.Title, 48), clip(p.Body, maxPromptBody))
				if b.Len()+len(block) > maxPromptBytes {
					break
				}
				b.WriteString(block)
			}
		}
	}
	if len(failures) > 0 && b.Len() < maxPromptBytes {
		failure := failures[0]
		block := fmt.Sprintf("\nCausal check (hypothesis; verify now):\n- %s -> %s", clip(failure.Trigger, 96), clip(failure.Consequence, 180))
		if failure.Evidence != "" {
			block += "; evidence: " + clip(failure.Evidence, 180)
		}
		if b.Len()+len(block) <= maxPromptBytes {
			b.WriteString(block)
		}
	}
	if len(routes) > 0 && b.Len() < maxPromptBytes {
		route := routes[0]
		block := fmt.Sprintf("\nKnown verification route (recheck current repo): %s", route.Class)
		if len(route.Targets) > 0 {
			block += " → " + strings.Join(route.Targets, ", ")
		}
		if len(route.Stages) > 0 {
			block += " [" + strings.Join(route.Stages, " → ") + "]"
		}
		if b.Len()+len(block) <= maxPromptBytes {
			b.WriteString(block)
		}
	}
	out := b.String()
	if len(out) > maxPromptBytes {
		out = out[:maxPromptBytes-1] + "…"
	}
	return out
}

func pickHabits(s Store, n int) []Habit {
	active := make([]Habit, 0, len(s.Habits))
	for _, h := range s.Habits {
		if strings.TrimSpace(h.Text) == "" {
			continue
		}
		active = append(active, h)
	}
	sort.SliceStable(active, func(i, j int) bool {
		if active[i].Pinned != active[j].Pinned {
			return active[i].Pinned
		}
		if active[i].Hits != active[j].Hits {
			return active[i].Hits > active[j].Hits
		}
		return active[i].UpdatedAt.After(active[j].UpdatedAt)
	})
	if len(active) > n {
		active = active[:n]
	}
	return active
}

func pickPlaybooks(s Store, userHint string, n int) []Playbook {
	hint := normTitle(userHint)
	type scored struct {
		p Playbook
		s int
	}
	var list []scored
	for _, p := range s.Playbooks {
		if p.Archived || strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.Body) == "" {
			continue
		}
		score := p.Hits
		if p.Pinned {
			score += 50
		}
		if hint != "" {
			blob := normTitle(p.Title + " " + p.Class + " " + p.Body)
			if overlapScore(hint, blob) == 0 && p.Hits < 3 {
				continue // irrelevant + cold = skip (token win)
			}
			score += overlapScore(hint, blob) * 10
		}
		list = append(list, scored{p: p, s: score})
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].s != list[j].s {
			return list[i].s > list[j].s
		}
		return list[i].p.UpdatedAt.After(list[j].p.UpdatedAt)
	})
	out := make([]Playbook, 0, n)
	for _, x := range list {
		out = append(out, x.p)
		if len(out) >= n {
			break
		}
	}
	return out
}

func overlapScore(hint, blob string) int {
	if hint == "" || blob == "" {
		return 0
	}
	hit := 0
	for _, t := range strings.Fields(hint) {
		if len(t) < 3 {
			continue
		}
		if strings.Contains(blob, t) {
			hit++
		}
	}
	return hit
}

// Summary is a short human-readable dump for /memory and the overview UI.
func Summary(s Store) string {
	var parts []string
	activeBooks := 0
	for _, p := range s.Playbooks {
		if !p.Archived {
			activeBooks++
		}
	}
	if len(s.Habits) == 0 && activeBooks == 0 {
		return "Picogent has not learned habits or playbooks for this folder yet. Keep working — it remembers what works."
	}
	parts = append(parts, fmt.Sprintf("Self-evolution: %d habits, %d playbooks (prompt budget ≤%d chars)", len(s.Habits), activeBooks, maxPromptBytes))
	for i, h := range pickHabits(s, 4) {
		_ = i
		parts = append(parts, "• habit: "+h.Text)
	}
	n := 0
	for _, p := range s.Playbooks {
		if p.Archived {
			continue
		}
		parts = append(parts, "• playbook: "+p.Title)
		n++
		if n >= 4 {
			break
		}
	}
	return strings.Join(parts, "\n")
}
