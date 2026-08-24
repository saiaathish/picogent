package evolve

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/verify"
)

const causalStaleDays = 90

var sensitiveEvidence = regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|authorization)\s*[:=]\s*[^\s,;]+`)

// RecordFailure stores a compact causal hypothesis from verification evidence.
// The evidence is scrubbed and capped before it can reach durable memory.
func RecordFailure(s Store, userHint, evidence string) Store {
	now := time.Now().UTC()
	class := memoryClass(userHint)
	trigger := failureTrigger(evidence)
	cleanEvidence := scrubEvidence(evidence)
	id := idFor("failure", class, trigger)
	for i := range s.Failures {
		if s.Failures[i].ID != id {
			continue
		}
		s.Failures[i].Hits++
		s.Failures[i].Failures++
		s.Failures[i].LastSeen = now
		if cleanEvidence != "" {
			s.Failures[i].Evidence = cleanEvidence
		}
		return Curate(s)
	}
	s.Failures = append(s.Failures, FailureMemory{
		ID:          id,
		Class:       class,
		Trigger:     trigger,
		Consequence: failureConsequence(trigger),
		Evidence:    cleanEvidence,
		Confidence:  "medium",
		Hits:        1,
		Failures:    1,
		LastSeen:    now,
	})
	return Curate(s)
}

// RecordVerificationRoute stores only a passing route. It never stores or
// executes a command from memory; verify still detects and builds commands.
func RecordVerificationRoute(s Store, userHint string, targets []string, evidence string) Store {
	if verify.StatusFromEvidence(evidence) != verify.StatusPass {
		return s
	}
	now := time.Now().UTC()
	class := memoryClass(userHint)
	cleanTargets := routeTargets(targets)
	stages := routeStages(evidence)
	id := idFor("route", class, strings.Join(cleanTargets, ","), strings.Join(stages, ","))
	for i := range s.VerificationRoutes {
		if s.VerificationRoutes[i].ID != id {
			continue
		}
		s.VerificationRoutes[i].Hits++
		s.VerificationRoutes[i].Passes++
		s.VerificationRoutes[i].LastUsed = now
		resolveLatestFailure(&s, class, now)
		return Curate(s)
	}
	s.VerificationRoutes = append(s.VerificationRoutes, VerificationRoute{
		ID:       id,
		Class:    class,
		Targets:  cleanTargets,
		Stages:   stages,
		Hits:     1,
		Passes:   1,
		LastUsed: now,
	})
	resolveLatestFailure(&s, class, now)
	return Curate(s)
}

func resolveLatestFailure(s *Store, class string, now time.Time) {
	if s == nil {
		return
	}
	best := -1
	for i := range s.Failures {
		if s.Failures[i].Class != class || s.Failures[i].LastResolved != nil {
			continue
		}
		if best < 0 || s.Failures[i].LastSeen.After(s.Failures[best].LastSeen) {
			best = i
		}
	}
	if best >= 0 {
		s.Failures[best].Resolutions++
		s.Failures[best].LastResolved = &now
	}
}

func failureTrigger(evidence string) string {
	low := strings.ToLower(evidence)
	switch {
	case strings.Contains(low, "command not found"), strings.Contains(low, "executable file not found"), strings.Contains(low, "no test runner"):
		return "verification runner unavailable"
	case strings.Contains(low, "inconclusive"), strings.Contains(low, "skipped"), strings.Contains(low, "permission denied"), strings.Contains(low, "deadline exceeded"), strings.Contains(low, "timeout"):
		return "verification proof unavailable"
	case strings.Contains(low, "old_string not found"), strings.Contains(low, "stale"), strings.Contains(low, "file does not exist"):
		return "edit context became stale"
	case strings.Contains(low, "fail"), strings.Contains(low, "error"), strings.Contains(low, "panic"):
		return "verification failed"
	default:
		return "verification produced an unexpected result"
	}
}

func failureConsequence(trigger string) string {
	switch trigger {
	case "verification runner unavailable":
		return "classify the environment and expected runner before changing code"
	case "verification proof unavailable":
		return "do not claim completion; resolve the missing proof or report the boundary"
	case "edit context became stale":
		return "reread the narrow region and recompute the edit"
	default:
		return "inspect the smallest responsible area, repair once, then rerun targeted and broader checks"
	}
}

func memoryClass(hint string) string {
	low := strings.ToLower(hint)
	switch {
	case strings.Contains(low, "security"), strings.Contains(low, "auth"), strings.Contains(low, "secret"), strings.Contains(low, "permission"):
		return "security"
	case strings.Contains(low, "ui"), strings.Contains(low, "gui"), strings.Contains(low, "browser"), strings.Contains(low, "frontend"), strings.Contains(low, "responsive"):
		return "ui"
	case strings.Contains(low, "performance"), strings.Contains(low, "latency"), strings.Contains(low, "startup"), strings.Contains(low, "memory"):
		return "performance"
	case strings.Contains(low, "test"), strings.Contains(low, "bug"), strings.Contains(low, "broken"), strings.Contains(low, "failure"), strings.Contains(low, "fix"):
		return "bug"
	default:
		return "general"
	}
}

func routeStages(evidence string) []string {
	low := strings.ToLower(evidence)
	var stages []string
	if strings.Contains(low, "targeted") {
		stages = append(stages, "targeted")
	}
	if strings.Contains(low, "broader") {
		stages = append(stages, "broader")
	}
	if len(stages) == 0 {
		stages = []string{"verification"}
	}
	return stages
}

func routeTargets(targets []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, target := range targets {
		target = strings.TrimSpace(strings.ReplaceAll(target, "\\", "/"))
		target = strings.TrimPrefix(target, "./")
		if target == "" || strings.HasPrefix(target, "../") || strings.Contains(target, "/../") || strings.HasPrefix(target, "/") || strings.Contains(target, ":") {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		out = append(out, target)
		if len(out) == 6 {
			break
		}
	}
	sort.Strings(out)
	return out
}

func scrubEvidence(evidence string) string {
	evidence = sensitiveEvidence.ReplaceAllString(evidence, "[sensitive evidence redacted]")
	evidence = strings.Join(strings.Fields(evidence), " ")
	return clip(evidence, 600)
}

func pickFailures(s Store, userHint string, n int) []FailureMemory {
	hint := normTitle(userHint)
	list := append([]FailureMemory(nil), s.Failures...)
	sort.SliceStable(list, func(i, j int) bool {
		scoreI := list[i].Hits + overlapScore(hint, normTitle(list[i].Class+" "+list[i].Trigger+" "+list[i].Consequence))*8
		scoreJ := list[j].Hits + overlapScore(hint, normTitle(list[j].Class+" "+list[j].Trigger+" "+list[j].Consequence))*8
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return list[i].LastSeen.After(list[j].LastSeen)
	})
	if n < 0 {
		n = 0
	}
	if len(list) > n {
		list = list[:n]
	}
	return list
}

func pickRoutes(s Store, userHint string, n int) []VerificationRoute {
	hint := normTitle(userHint)
	list := append([]VerificationRoute(nil), s.VerificationRoutes...)
	sort.SliceStable(list, func(i, j int) bool {
		scoreI := list[i].Hits + overlapScore(hint, normTitle(list[i].Class+" "+strings.Join(list[i].Targets, " ")))*8
		scoreJ := list[j].Hits + overlapScore(hint, normTitle(list[j].Class+" "+strings.Join(list[j].Targets, " ")))*8
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return list[i].LastUsed.After(list[j].LastUsed)
	})
	if n < 0 {
		n = 0
	}
	if len(list) > n {
		list = list[:n]
	}
	return list
}

func curateFailures(in []FailureMemory) []FailureMemory {
	cutoff := time.Now().UTC().Add(-causalStaleDays * 24 * time.Hour)
	kept := make([]FailureMemory, 0, len(in))
	for _, failure := range in {
		failure.Trigger = clip(failure.Trigger, 120)
		failure.Consequence = clip(failure.Consequence, 220)
		failure.Evidence = scrubEvidence(failure.Evidence)
		if failure.Trigger == "" || failure.Consequence == "" {
			continue
		}
		if !failure.LastSeen.IsZero() && failure.LastSeen.Before(cutoff) && failure.Failures <= failure.Resolutions {
			continue
		}
		kept = append(kept, failure)
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].Hits != kept[j].Hits {
			return kept[i].Hits > kept[j].Hits
		}
		return kept[i].LastSeen.After(kept[j].LastSeen)
	})
	if len(kept) > maxFailures {
		kept = kept[:maxFailures]
	}
	return kept
}

func curateRoutes(in []VerificationRoute) []VerificationRoute {
	cutoff := time.Now().UTC().Add(-causalStaleDays * 24 * time.Hour)
	kept := make([]VerificationRoute, 0, len(in))
	for _, route := range in {
		route.Targets = routeTargets(route.Targets)
		if route.ID == "" || route.Hits <= 0 || (!route.LastUsed.IsZero() && route.LastUsed.Before(cutoff)) {
			continue
		}
		kept = append(kept, route)
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].Hits != kept[j].Hits {
			return kept[i].Hits > kept[j].Hits
		}
		return kept[i].LastUsed.After(kept[j].LastUsed)
	})
	if len(kept) > maxRoutes {
		kept = kept[:maxRoutes]
	}
	return kept
}
