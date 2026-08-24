package ctxmgr

import (
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
)

// DeduplicateToolResults keeps every tool-call/result pair valid while
// replacing older identical read results with a small fact. The newest result
// remains authoritative because a write may have happened between reads.
// This is transcript compaction only: it does not cache or skip tool work.
func DeduplicateToolResults(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)

	type callMeta struct {
		name string
		key  string
	}
	byID := make(map[string]callMeta)
	for _, message := range out {
		if message.Role != "assistant" {
			continue
		}
		for _, call := range message.ToolCalls {
			if call.ID == "" {
				continue
			}
			name := strings.ToLower(strings.TrimSpace(call.Name))
			key := ""
			switch name {
			case "read_file":
				if path := pathFromArgs(name, call.Arguments); path != "" {
					key = name + ":" + path
				}
			case "repo_map":
				key = name
			}
			if key != "" {
				byID[call.ID] = callMeta{name: name, key: key}
			}
		}
	}

	lastByKey := make(map[string]int)
	for i, message := range out {
		if message.Role != "tool" {
			continue
		}
		meta, ok := byID[message.ToolCallID]
		if !ok {
			continue
		}
		if previous, exists := lastByKey[meta.key]; exists {
			out[previous].Content = duplicateToolDigest(meta.name, meta.key, out[previous].Content)
		}
		lastByKey[meta.key] = i
	}
	return out
}

func duplicateToolDigest(name, key, content string) string {
	return fmt.Sprintf("[duplicate %s result omitted; latest result for %s retained; original %d chars]", name, clip(key, 120), len(content))
}

// toolPairSafeTail removes incomplete tool exchanges after a window has been
// selected. Providers reject or misinterpret a tool result whose call is no
// longer in the transcript, so context reduction must preserve this invariant.
func toolPairSafeTail(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}

	callAt := make(map[string]int)
	callIDs := make(map[int][]string)
	for i, message := range msgs {
		if message.Role != "assistant" || len(message.ToolCalls) == 0 {
			continue
		}
		for _, call := range message.ToolCalls {
			if call.ID == "" {
				continue
			}
			callAt[call.ID] = i
			callIDs[i] = append(callIDs[i], call.ID)
		}
	}

	resultCount := make(map[string]int)
	for i, message := range msgs {
		if message.Role != "tool" {
			continue
		}
		if call, ok := callAt[message.ToolCallID]; ok && call < i {
			resultCount[message.ToolCallID]++
		}
	}

	drop := make(map[int]bool)
	for assistant, ids := range callIDs {
		for _, id := range ids {
			if resultCount[id] == 0 {
				drop[assistant] = true
				break
			}
		}
	}

	seenResults := make(map[string]bool)
	for i, message := range msgs {
		if message.Role != "tool" {
			continue
		}
		assistant, ok := callAt[message.ToolCallID]
		if !ok || assistant >= i || drop[assistant] || seenResults[message.ToolCallID] {
			drop[i] = true
			continue
		}
		seenResults[message.ToolCallID] = true
	}

	kept := make([]llm.Message, 0, len(msgs))
	for i, message := range msgs {
		if !drop[i] {
			kept = append(kept, message)
		}
	}
	return kept
}

// backfillToolCallStart moves a tail boundary back to the assistant message
// that owns a retained tool result. The window may grow by one exchange, but
// never emits an orphaned result merely to satisfy a token/message count.
func backfillToolCallStart(msgs []llm.Message, start int) int {
	if start < 0 {
		start = 0
	}
	if start >= len(msgs) {
		return len(msgs)
	}
	for start > 0 && msgs[start].Role == "tool" {
		id := msgs[start].ToolCallID
		owner := -1
		for i := start - 1; i >= 0; i-- {
			if msgs[i].Role != "assistant" {
				continue
			}
			for _, call := range msgs[i].ToolCalls {
				if call.ID == id {
					owner = i
					break
				}
			}
			if owner >= 0 {
				break
			}
		}
		if owner < 0 {
			break
		}
		start = owner
	}
	return start
}
