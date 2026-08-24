// Package scope keeps the first step of a broad request small and predictable.
// It deliberately uses a few plain-language rules instead of another model
// call: safely broad requests begin with an understandable recommended boundary,
// while callers can still explicitly ask to clarify it.
package scope

import (
	"fmt"
	"strings"
)

// Choice is a user-facing boundary for a broad request. The copy is kept free
// of implementation details so the same choices can be shown in every UI.
type Choice struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Why         string `json:"why"`
	Recommended bool   `json:"recommended"`
}

// Prompt describes the small preflight available when a request leaves too much
// room for interpretation. There are always two or three choices and the
// first one is the recommended default.
type Prompt struct {
	Question string   `json:"question"`
	Choices  []Choice `json:"choices"`
}

// Analyze returns a preflight only for broad or ambiguous requests. Specific
// requests (especially those naming a file, symbol, or concrete behavior) are
// sent directly to the agent.
func Analyze(prompt string) (Prompt, bool) {
	p := strings.TrimSpace(prompt)
	if p == "" || strings.HasPrefix(p, "/") {
		return Prompt{}, false
	}
	lower := strings.ToLower(p)
	if explicitCompletionRequest(lower) {
		return Prompt{}, false
	}
	words := strings.Fields(p)
	if len(words) > 28 || hasConcreteTarget(lower) {
		return Prompt{}, false
	}

	kind := requestKind(lower)
	if kind == "" {
		if broadPhrase(lower) {
			kind = "general"
		} else {
			return Prompt{}, false
		}
	}

	// A short imperative with no target is the strongest signal that a quick
	// boundary will help. Explicitly broad phrases are also included even when
	// they contain a few more words.
	broad := len(words) <= 7 || broadPhrase(lower)
	if !broad {
		return Prompt{}, false
	}

	switch kind {
	case "build":
		return Prompt{
			Question: "How big should the first pass be?",
			Choices: []Choice{
				{ID: "small", Label: "A small working version", Why: "Best default: useful quickly, with a narrow change that is easy to verify.", Recommended: true},
				{ID: "full", Label: "A fuller implementation", Why: "I’ll cover more supporting pieces and edge cases up front.", Recommended: false},
				{ID: "plan", Label: "Plan it before building", Why: "I’ll map the pieces and tradeoffs first, then wait for your go-ahead.", Recommended: false},
			},
		}, true
	case "fix":
		return Prompt{
			Question: "What should I focus on first?",
			Choices: []Choice{
				{ID: "focused", Label: "A focused fix", Why: "Best default: I’ll change the smallest surface, then run the relevant checks.", Recommended: true},
				{ID: "cleanup", Label: "A broader cleanup", Why: "I’ll address nearby issues too, which may touch more files.", Recommended: false},
				{ID: "report", Label: "Explain what you find first", Why: "I’ll inspect and report before changing anything.", Recommended: false},
			},
		}, true
	default:
		return Prompt{
			Question: "What outcome do you want first?",
			Choices: []Choice{
				{ID: "focused", Label: "The smallest useful improvement", Why: "Best default: I’ll keep the change focused and verify it before expanding.", Recommended: true},
				{ID: "explore", Label: "Explore and report first", Why: "I’ll inspect the project and explain the best next step before changing anything.", Recommended: false},
				{ID: "broad", Label: "A broader pass", Why: "I’ll improve related areas together, which may take more time and files.", Recommended: false},
			},
		}, true
	}
}

// Recommended returns the recommended choice for a prompt. It is safe for
// callers to use as the headless default even if a malformed prompt is passed.
func Recommended(p Prompt) Choice {
	for _, c := range p.Choices {
		if c.Recommended {
			return c
		}
	}
	if len(p.Choices) > 0 {
		return p.Choices[0]
	}
	return Choice{}
}

// DefaultMessage explains an automatic recommendation without implying that
// the user explicitly picked it. UI surfaces can show this after the task has
// been accepted instead of interrupting a safely broad request.
func DefaultMessage(choice Choice) string {
	label := strings.TrimSpace(choice.Label)
	if label == "" {
		return "Starting with the recommended scope by default."
	}
	return fmt.Sprintf("Starting with %s by default.", strings.ToLower(label))
}

// TurnBoundary gives a selected scope precedence over a broader durable task
// for this turn only. The durable task remains useful for resumability, but it
// must not silently override either an automatic default or an explicit choice.
func TurnBoundary(choice Choice) string {
	label := strings.TrimSpace(choice.Label)
	if label == "" {
		label = "the recommended scope"
	}
	return fmt.Sprintf("For this turn, honor this scope boundary: %s. This temporary boundary takes precedence over any broader active goal or durable task; do not expand beyond it unless the user explicitly asks.", label)
}

// Select resolves a stable choice ID from a prompt. Callers that need both the
// user-message guidance and the per-turn precedence boundary should resolve
// the same validated choice before applying it.
func Select(p Prompt, choiceID string) (Choice, bool) {
	choiceID = strings.TrimSpace(strings.ToLower(choiceID))
	if choiceID == "" {
		return Choice{}, false
	}
	for _, choice := range p.Choices {
		if choice.ID == choiceID {
			return choice, true
		}
	}
	return Choice{}, false
}

// Apply attaches a selected or recommended boundary to the agent message. The
// instruction is intentionally short and is removed from the saved transcript
// by the UI surfaces after the turn completes.
func Apply(prompt string, p Prompt, choiceID string) (string, bool) {
	choice, found := Select(p, choiceID)
	if !found || strings.TrimSpace(prompt) == "" {
		return "", false
	}
	return fmt.Sprintf("%s\n\nPicogent scope choice: %s. %s Keep the work within this boundary and do not ask another scope question unless blocked.", strings.TrimSpace(prompt), choice.Label, choice.Why), true
}

func requestKind(p string) string {
	for _, phrase := range []string{"make it better", "improve this", "clean this up", "feel way better", "what should we do", "where do we start", "work on this", "take care of this", "deal with this", "help with this"} {
		if strings.Contains(p, phrase) {
			return "general"
		}
	}
	// An explicit broad repair remains a fix even when its success criterion
	// includes verbs such as "make CI green". Otherwise the generic build verb
	// list below would incorrectly choose a build-sized first pass.
	for _, phrase := range []string{"fix everything", "fix all", "fix every", "doesn't work", "doesn’t work", "does not work"} {
		if strings.Contains(p, phrase) {
			return "fix"
		}
	}
	for _, word := range []string{"build", "create", "make", "add", "implement", "write", "develop", "set up", "setup"} {
		if containsWord(p, word) {
			return "build"
		}
	}
	for _, word := range []string{"fix", "debug", "repair", "refactor", "clean", "improve", "update", "modernize", "change", "remove"} {
		if containsWord(p, word) {
			return "fix"
		}
	}
	return ""
}

func broadPhrase(p string) bool {
	for _, phrase := range []string{
		"make it better", "improve this", "clean this up", "fix everything", "fix all",
		"build something", "create something", "make an app", "make a website", "make me a website",
		"make it production ready", "feel way better", "work on this", "take care of this",
		"deal with this", "help with this", "what should we do", "where do we start",
	} {
		if strings.Contains(p, phrase) {
			return true
		}
	}
	return false
}

// explicitCompletionRequest is an outcome request, not a request to narrow
// the work. Let the agent inspect the project and determine what remains.
func explicitCompletionRequest(p string) bool {
	for _, phrase := range []string{"finish this project", "finish the project"} {
		if strings.Contains(p, phrase) {
			return true
		}
	}
	return false
}

func hasConcreteTarget(p string) bool {
	// Paths, common extensions, and quoted/symbol-like names make the request
	// concrete enough to skip a preflight. This stays intentionally conservative.
	if strings.ContainsAny(p, "/\\") || strings.ContainsAny(p, "`\"") {
		return true
	}
	for _, ext := range []string{".go", ".js", ".ts", ".tsx", ".jsx", ".py", ".rs", ".java", ".rb", ".md", ".yaml", ".yml", ".json", ".css", ".html", ".sql"} {
		if strings.Contains(p, ext) {
			return true
		}
	}
	return false
}

func containsWord(text, word string) bool {
	if strings.Contains(word, " ") {
		return strings.Contains(text, word)
	}
	for _, field := range strings.FieldsFunc(text, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if field == word {
			return true
		}
	}
	return false
}
