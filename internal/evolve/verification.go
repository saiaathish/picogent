package evolve

import (
	"sort"
	"strings"
	"unicode"
)

const maxVerificationTargets = 6

// VerificationTargets extracts a small set of workspace-relative paths from
// the relevant learned playbook. Playbooks are intentionally plain text, so
// this keeps the integration conservative: only path-like tokens are allowed
// through to the verifier and shell commands are never executed from memory.
// The caller still supplies the actual changed paths and the verifier remains
// responsible for detecting the runner and building safe commands.
func VerificationTargets(s Store, userHint string) []string {
	books := pickPlaybooks(s, userHint, maxPromptBooks)
	seen := make(map[string]struct{}, maxVerificationTargets)
	var out []string
	for _, book := range books {
		for _, field := range strings.Fields(book.Body) {
			target, ok := pathLikeTarget(field)
			if !ok {
				continue
			}
			if _, exists := seen[target]; exists {
				continue
			}
			seen[target] = struct{}{}
			out = append(out, target)
			if len(out) == maxVerificationTargets {
				break
			}
		}
		if len(out) == maxVerificationTargets {
			break
		}
	}
	// Keep the output deterministic even if a playbook is edited manually.
	sort.Strings(out)
	return out
}

func pathLikeTarget(raw string) (string, bool) {
	target := strings.TrimSpace(raw)
	target = strings.Trim(target, "`'\"()[]{}<>,;:!?")
	target = strings.ReplaceAll(target, "\\", "/")
	target = strings.TrimPrefix(target, "./")
	if target == "" || target == "." || target == "..." || strings.Contains(target, "...") || strings.HasPrefix(target, "/") {
		return "", false
	}
	if strings.Contains(target, "://") || strings.HasPrefix(target, "$") || strings.HasPrefix(target, "-") {
		return "", false
	}
	if strings.Contains(target, ":") || !strings.Contains(target, "/") {
		return "", false
	}
	for _, r := range target {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._/-", r) {
			continue
		}
		return "", false
	}
	if strings.HasPrefix(target, "../") || strings.Contains(target, "/../") || strings.HasSuffix(target, "/") {
		return "", false
	}
	return target, true
}
