package evolve

import (
	"sort"
	"strings"
	"time"
)

// Curate keeps the store tiny: archive stale playbooks, drop lowest-hit extras.
// Deterministic only — no LLM (Hermes curator prune pass, without the expensive consolidation).
func Curate(s Store) Store {
	now := time.Now().UTC()
	cutoff := now.Add(-staleDays * 24 * time.Hour)

	var habits []Habit
	for _, h := range s.Habits {
		h.Text = clip(h.Text, maxHabitLen)
		if h.Text == "" {
			continue
		}
		habits = append(habits, h)
	}
	habits = dedupeHabits(habits)
	sort.SliceStable(habits, func(i, j int) bool {
		if habits[i].Pinned != habits[j].Pinned {
			return habits[i].Pinned
		}
		if habits[i].Hits != habits[j].Hits {
			return habits[i].Hits > habits[j].Hits
		}
		return habits[i].UpdatedAt.After(habits[j].UpdatedAt)
	})
	if len(habits) > maxHabits {
		kept := make([]Habit, 0, maxHabits)
		for _, h := range habits {
			if h.Pinned || len(kept) < maxHabits {
				kept = append(kept, h)
			}
		}
		if len(kept) > maxHabits {
			kept = kept[:maxHabits]
		}
		habits = kept
	}
	s.Habits = habits

	var playbooks []Playbook
	for _, p := range s.Playbooks {
		p.Title = clip(p.Title, 72)
		p.Body = clip(p.Body, maxBodyLen)
		if p.Title == "" || p.Body == "" {
			continue
		}
		if !p.Pinned && !p.Archived && p.UpdatedAt.Before(cutoff) && p.Hits == 0 {
			p.Archived = true
		}
		playbooks = append(playbooks, p)
	}
	playbooks = dedupePlaybooks(playbooks)

	active := make([]Playbook, 0, len(playbooks))
	archived := make([]Playbook, 0)
	for _, p := range playbooks {
		if p.Archived {
			archived = append(archived, p)
			continue
		}
		active = append(active, p)
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
	if len(active) > maxPlaybooks {
		extra := active[maxPlaybooks:]
		active = active[:maxPlaybooks]
		for _, p := range extra {
			if p.Pinned {
				active = append(active, p)
				continue
			}
			p.Archived = true
			archived = append(archived, p)
		}
	}
	// Keep a small archive for recoverability, but don't inject archived into prompts.
	if len(archived) > 12 {
		sort.SliceStable(archived, func(i, j int) bool {
			return archived[i].UpdatedAt.After(archived[j].UpdatedAt)
		})
		archived = archived[:12]
	}
	s.Playbooks = append(active, archived...)
	s.Failures = curateFailures(s.Failures)
	s.VerificationRoutes = curateRoutes(s.VerificationRoutes)
	return s
}

func dedupeHabits(in []Habit) []Habit {
	seen := map[string]int{}
	var out []Habit
	for _, h := range in {
		key := normTitle(h.Text)
		if key == "" {
			continue
		}
		if i, ok := seen[key]; ok {
			if h.Hits > out[i].Hits || h.UpdatedAt.After(out[i].UpdatedAt) {
				out[i] = h
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, h)
	}
	return out
}

func dedupePlaybooks(in []Playbook) []Playbook {
	seen := map[string]int{}
	var out []Playbook
	for _, p := range in {
		key := normTitle(p.Title)
		if p.Class != "" {
			key = "class:" + normTitle(p.Class)
		}
		if key == "" {
			continue
		}
		if i, ok := seen[key]; ok {
			// Class-first: keep the richer / newer body.
			cur := out[i]
			if len(p.Body) > len(cur.Body) || p.UpdatedAt.After(cur.UpdatedAt) {
				p.Hits += cur.Hits
				if cur.Pinned {
					p.Pinned = true
				}
				out[i] = p
			} else {
				out[i].Hits += p.Hits
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, p)
	}
	return out
}

// UpsertHabit adds or refreshes a habit (prefer update over create).
func UpsertHabit(s Store, text, source string) (Store, Habit, bool) {
	text = clip(text, maxHabitLen)
	if text == "" {
		return s, Habit{}, false
	}
	now := time.Now().UTC()
	key := normTitle(text)
	for i := range s.Habits {
		if similar(normTitle(s.Habits[i].Text), key) {
			s.Habits[i].Hits++
			s.Habits[i].UpdatedAt = now
			if source != "" {
				s.Habits[i].Source = source
			}
			// Prefer the clearer phrasing when close.
			if len(text) > len(s.Habits[i].Text)+8 {
				s.Habits[i].Text = text
			}
			return s, s.Habits[i], false
		}
	}
	h := Habit{
		ID:        idFor("habit", text),
		Text:      text,
		Source:    source,
		Hits:      1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.Habits = append(s.Habits, h)
	return s, h, true
}

// UpsertPlaybook adds or patches a playbook by class/title (class-first).
func UpsertPlaybook(s Store, title, body, class, source string) (Store, Playbook, bool) {
	title = clip(title, 72)
	body = clip(body, maxBodyLen)
	class = clip(class, 64)
	if title == "" || body == "" {
		return s, Playbook{}, false
	}
	now := time.Now().UTC()
	classKey := normTitle(class)
	titleKey := normTitle(title)

	for i := range s.Playbooks {
		p := &s.Playbooks[i]
		if p.Archived {
			continue
		}
		match := false
		if classKey != "" && normTitle(p.Class) == classKey {
			match = true
		} else if similar(normTitle(p.Title), titleKey) {
			match = true
		}
		if !match {
			continue
		}
		p.Hits++
		p.UpdatedAt = now
		p.Body = body
		if title != "" {
			p.Title = title
		}
		if class != "" {
			p.Class = class
		}
		if source != "" {
			p.Source = source
		}
		return s, *p, false
	}

	pb := Playbook{
		ID:        idFor("playbook", class, title),
		Title:     title,
		Body:      body,
		Class:     class,
		Source:    source,
		Hits:      1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.Playbooks = append(s.Playbooks, pb)
	return s, pb, true
}

func similar(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return true
	}
	// Token overlap for short titles.
	as := strings.Fields(a)
	bs := strings.Fields(b)
	if len(as) == 0 || len(bs) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, t := range as {
		set[t] = true
	}
	hit := 0
	for _, t := range bs {
		if set[t] {
			hit++
		}
	}
	need := len(as)
	if len(bs) < need {
		need = len(bs)
	}
	return hit*2 >= need+1
}
