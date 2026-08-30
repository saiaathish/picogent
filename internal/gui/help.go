package gui

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

type helpDocs struct {
	Product  string      `json:"product"`
	Tagline  string      `json:"tagline"`
	Starters []string    `json:"starters"`
	Topics   []helpTopic `json:"topics"`
}

type helpTopic struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Blurb    string   `json:"blurb"`
	Keywords []string `json:"keywords"`
	Body     string   `json:"body"`
}

type helpAnswer struct {
	Answer   string      `json:"answer"`
	TopicIDs []string    `json:"topic_ids,omitempty"`
	Topics   []helpTopic `json:"topics,omitempty"`
	Matched  bool        `json:"matched"`
	Source   string      `json:"source"` // "docs"
}

var (
	helpOnce sync.Once
	helpData helpDocs
	helpErr  error
	tokenRe  = regexp.MustCompile(`[a-z0-9][a-z0-9+./_-]{1,}`)
)

func loadHelpDocs() (helpDocs, error) {
	helpOnce.Do(func() {
		raw, err := webFS.ReadFile("web/help-docs.json")
		if err != nil {
			helpErr = err
			return
		}
		helpErr = json.Unmarshal(raw, &helpData)
	})
	return helpData, helpErr
}

func (s *server) helpAPI(w http.ResponseWriter, r *http.Request) {
	docs, err := loadHelpDocs()
	if err != nil {
		writeGUIError(w, "help docs unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{
			"product":  docs.Product,
			"tagline":  docs.Tagline,
			"starters": docs.Starters,
			"topics":   docs.Topics,
		})
	case http.MethodPost:
		var req struct {
			Question string `json:"question"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeGUIError(w, "bad json", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(answerHelp(docs, req.Question))
	default:
		writeGUIError(w, "GET or POST", 405)
	}
}

func answerHelp(docs helpDocs, question string) helpAnswer {
	q := strings.TrimSpace(question)
	if q == "" {
		return helpAnswer{
			Answer:  docs.Tagline + "\n\nPick a topic below, or ask how Safe mode, models, slash commands, or MCP work.",
			Matched: true,
			Source:  "docs",
			Topics:  docs.Topics,
		}
	}

	scored := scoreHelpTopics(docs.Topics, q)
	if len(scored) == 0 || scored[0].score < 2 {
		return helpAnswer{
			Answer:  "I couldn’t find that in the Picogent guide.\n\nTry one of the topics on the left, or ask the main chat for help with your project. This help panel only answers from the built-in docs — it doesn’t spend model tokens.",
			Matched: false,
			Source:  "docs",
		}
	}

	top := scored[0]
	ids := []string{top.topic.ID}
	parts := []string{top.topic.Body}
	if len(scored) > 1 && scored[1].score >= scored[0].score-1 && scored[1].score >= 3 {
		ids = append(ids, scored[1].topic.ID)
		parts = append(parts, "\n---\n\n"+scored[1].topic.Body)
	}

	return helpAnswer{
		Answer:   strings.Join(parts, "\n"),
		TopicIDs: ids,
		Matched:  true,
		Source:   "docs",
		Topics:   []helpTopic{top.topic},
	}
}

type scoredTopic struct {
	topic helpTopic
	score int
}

func scoreHelpTopics(topics []helpTopic, question string) []scoredTopic {
	q := strings.ToLower(question)
	qTokens := tokenizeHelp(q)
	out := make([]scoredTopic, 0, len(topics))
	for _, t := range topics {
		score := 0
		title := strings.ToLower(t.Title)
		blurb := strings.ToLower(t.Blurb)
		body := strings.ToLower(t.Body)
		if strings.Contains(q, strings.ToLower(t.ID)) {
			score += 6
		}
		if strings.Contains(q, title) {
			score += 8
		}
		for _, kw := range t.Keywords {
			kw = strings.ToLower(kw)
			if kw == "" {
				continue
			}
			if strings.Contains(q, kw) {
				score += 4
			}
			for _, tok := range qTokens {
				if tok == kw || (strings.HasPrefix(kw, tok) && len(tok) >= 4) {
					score += 2
				}
			}
		}
		for _, tok := range qTokens {
			if len(tok) < 3 {
				continue
			}
			if strings.Contains(title, tok) {
				score += 3
			}
			if strings.Contains(blurb, tok) {
				score += 2
			}
			if strings.Contains(body, tok) {
				score += 1
			}
		}
		if score > 0 {
			out = append(out, scoredTopic{topic: t, score: score})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score == out[j].score {
			return out[i].topic.Title < out[j].topic.Title
		}
		return out[i].score > out[j].score
	})
	return out
}

func tokenizeHelp(s string) []string {
	raw := tokenRe.FindAllString(strings.ToLower(s), -1)
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, t := range raw {
		t = strings.TrimFunc(t, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
		if len(t) < 2 || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}
