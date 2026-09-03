package ctxmgr

import (
	"sort"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/retention"
)

// contextRetentionCandidate keeps a live message unit beside the structural
// projection used by the retention contract. Message content never enters the
// ranking input or its assessment.
type contextRetentionCandidate struct {
	messages []llm.Message
	unit     retention.Unit
	turn     int
	index    int
}

// ValueAwareWindow selects a bounded live-context window by the same
// provider-independent retention contract used for durable session saves.
// The system prompt is preserved separately, the newest user request and
// newest complete turn are anchors, and older complete units fill remaining
// capacity by deterministic structural rank. If the contract input bound is
// exceeded, the existing recency-only tail is retained as the safe fallback.
func ValueAwareWindow(msgs []llm.Message, keep int) []llm.Message {
	if len(msgs) == 0 || keep <= 0 {
		return nil
	}
	if len(msgs) <= keep {
		return toolPairSafeTail(msgs)
	}

	var system *llm.Message
	body := msgs
	if msgs[0].Role == "system" {
		copyOfSystem := msgs[0]
		system = &copyOfSystem
		body = msgs[1:]
	}
	if len(body) == 0 {
		if system == nil {
			return nil
		}
		return []llm.Message{*system}
	}

	candidates, turns := contextRetentionCandidates(body)
	if len(candidates) == 0 {
		return TruncateTail(msgs, keep)
	}

	capacity := keep
	if system != nil {
		capacity--
	}
	if capacity <= 0 {
		if system == nil {
			return nil
		}
		return []llm.Message{*system}
	}

	latestUser := -1
	for index := len(candidates) - 1; index >= 0; index-- {
		if contextCandidateRole(candidates[index]) == retention.RoleUser {
			latestUser = index
			break
		}
	}

	latestCompleteTurn := -1
	for index := len(turns) - 1; index >= 0; index-- {
		if contextRetentionTurnComplete(turns[index]) {
			latestCompleteTurn = index
			break
		}
	}

	// The S contract deliberately bounds structural input. Preserve the
	// explicit anchors plus the newest bounded candidate window before ranking;
	// this makes the observation boundary visible instead of silently claiming
	// that an arbitrarily old unit was considered.
	candidates = boundContextRetentionCandidates(candidates, turns, latestUser, latestCompleteTurn)
	if len(candidates) == 0 {
		return TruncateTail(msgs, keep)
	}
	for index := range candidates {
		candidates[index].unit.Position = index
		candidates[index].unit.OriginalIndex = index
	}

	byIndex := make(map[int]contextRetentionCandidate, len(candidates))
	for _, candidate := range candidates {
		byIndex[candidate.index] = candidate
	}

	selected := make([]contextRetentionCandidate, 0, capacity)
	selectedIndexes := make(map[int]struct{}, capacity)
	messageCount := 0
	appendCandidate := func(candidate contextRetentionCandidate) bool {
		if _, exists := selectedIndexes[candidate.index]; exists {
			return false
		}
		if messageCount+len(candidate.messages) > capacity {
			return false
		}
		selected = append(selected, candidate)
		selectedIndexes[candidate.index] = struct{}{}
		messageCount += len(candidate.messages)
		return true
	}

	// Keep the current request visible even when its turn is still waiting for
	// a tool result. A pending assistant/tool fragment is not an eligible unit.
	if latestUser >= 0 {
		if candidate, ok := byIndex[latestUser]; ok {
			appendCandidate(candidate)
		}
	}
	if latestCompleteTurn >= 0 {
		// Admit the newest units first if an unusually large complete turn must
		// fit into the bounded live window. The final sort restores transcript
		// order without splitting any selected tool exchange.
		for index := len(turns[latestCompleteTurn]) - 1; index >= 0; index-- {
			if candidate, ok := byIndex[turns[latestCompleteTurn][index].index]; ok {
				appendCandidate(candidate)
			}
		}
	}

	latestTurn := len(turns) - 1
	rankInput := make([]retention.Unit, 0, len(candidates))
	rankCandidates := make([]contextRetentionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.turn >= latestTurn {
			continue
		}
		if _, selected := selectedIndexes[candidate.index]; selected {
			continue
		}
		rankInput = append(rankInput, candidate.unit)
		rankCandidates = append(rankCandidates, candidate)
	}
	ranked, err := retention.Rank(rankInput)
	if err == nil {
		for _, rankedCandidate := range ranked {
			if !rankedCandidate.Assessment.IsEligible() {
				continue
			}
			appendCandidate(rankCandidates[rankedCandidate.InputIndex])
		}
	}

	sort.SliceStable(selected, func(i, j int) bool {
		return selected[i].index < selected[j].index
	})
	flat := make([]llm.Message, 0, messageCount+1)
	if system != nil {
		flat = append(flat, *system)
	}
	for _, candidate := range selected {
		flat = append(flat, candidate.messages...)
	}
	if len(flat) == 0 {
		return TruncateTail(msgs, keep)
	}
	return flat
}

func boundContextRetentionCandidates(candidates []contextRetentionCandidate, turns [][]contextRetentionCandidate, latestUser, latestCompleteTurn int) []contextRetentionCandidate {
	if len(candidates) <= retention.MaxUnits {
		return candidates
	}

	required := make(map[int]struct{})
	if latestUser >= 0 {
		required[latestUser] = struct{}{}
	}
	if latestCompleteTurn >= 0 {
		for _, candidate := range turns[latestCompleteTurn] {
			required[candidate.index] = struct{}{}
		}
	}

	keep := make(map[int]struct{}, retention.MaxUnits)
	if len(required) > retention.MaxUnits {
		if latestUser >= 0 {
			keep[latestUser] = struct{}{}
		}
		if latestCompleteTurn >= 0 {
			for index := len(turns[latestCompleteTurn]) - 1; index >= 0 && len(keep) < retention.MaxUnits; index-- {
				keep[turns[latestCompleteTurn][index].index] = struct{}{}
			}
		}
	} else {
		for index := range required {
			keep[index] = struct{}{}
		}
	}
	for index := len(candidates) - 1; index >= 0 && len(keep) < retention.MaxUnits; index-- {
		keep[candidates[index].index] = struct{}{}
	}

	indexes := make([]int, 0, len(keep))
	for index := range keep {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	bounded := make([]contextRetentionCandidate, 0, len(indexes))
	byIndex := make(map[int]contextRetentionCandidate, len(candidates))
	for _, candidate := range candidates {
		byIndex[candidate.index] = candidate
	}
	for _, index := range indexes {
		if candidate, ok := byIndex[index]; ok {
			bounded = append(bounded, candidate)
		}
	}
	return bounded
}

func contextRetentionCandidates(messages []llm.Message) ([]contextRetentionCandidate, [][]contextRetentionCandidate) {
	turns := contextSplitTurns(messages)
	candidates := make([]contextRetentionCandidate, 0, len(messages))
	byTurn := make([][]contextRetentionCandidate, 0, len(turns))
	for turnIndex, turn := range turns {
		turnCandidates := make([]contextRetentionCandidate, 0, len(turn))
		for _, unitMessages := range contextMessageUnits(turn) {
			candidate := contextRetentionCandidate{
				messages: unitMessages,
				turn:     turnIndex,
				index:    len(candidates),
				unit: retention.Unit{
					Messages:      projectContextRetentionMessages(unitMessages),
					CurrentTurn:   turnIndex == len(turns)-1,
					Position:      len(candidates),
					OriginalIndex: len(candidates),
				},
			}
			candidates = append(candidates, candidate)
			turnCandidates = append(turnCandidates, candidate)
		}
		byTurn = append(byTurn, turnCandidates)
	}
	return candidates, byTurn
}

func contextSplitTurns(messages []llm.Message) [][]llm.Message {
	var turns [][]llm.Message
	start := 0
	for index := 1; index < len(messages); index++ {
		if messages[index].Role == "user" {
			turns = append(turns, append([]llm.Message(nil), messages[start:index]...))
			start = index
		}
	}
	if start < len(messages) {
		turns = append(turns, append([]llm.Message(nil), messages[start:]...))
	}
	return turns
}

func contextMessageUnits(messages []llm.Message) [][]llm.Message {
	units := make([][]llm.Message, 0, len(messages))
	for index := 0; index < len(messages); {
		if messages[index].Role == "tool" {
			index++
			continue
		}
		end := index + 1
		if messages[index].Role == "assistant" && len(messages[index].ToolCalls) > 0 {
			callIDs := make(map[string]struct{}, len(messages[index].ToolCalls))
			for _, call := range messages[index].ToolCalls {
				if call.ID != "" {
					callIDs[call.ID] = struct{}{}
				}
			}
			for end < len(messages) && messages[end].Role == "tool" {
				if _, ok := callIDs[messages[end].ToolCallID]; !ok {
					break
				}
				end++
			}
		}
		units = append(units, append([]llm.Message(nil), messages[index:end]...))
		index = end
	}
	return units
}

func projectContextRetentionMessages(messages []llm.Message) []retention.Message {
	projected := make([]retention.Message, 0, len(messages))
	for _, message := range messages {
		structural := retention.Message{Role: retention.Role(message.Role)}
		if message.Role == "assistant" && len(message.ToolCalls) > 0 {
			structural.ToolCallIDs = make([]string, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				structural.ToolCallIDs = append(structural.ToolCallIDs, call.ID)
			}
		}
		if message.Role == "tool" {
			structural.ToolCallID = message.ToolCallID
		}
		projected = append(projected, structural)
	}
	return projected
}

func contextCandidateRole(candidate contextRetentionCandidate) retention.Role {
	if len(candidate.unit.Messages) == 0 {
		return retention.RoleUnspecified
	}
	return candidate.unit.Messages[0].Role
}

func contextRetentionTurnComplete(turn []contextRetentionCandidate) bool {
	if len(turn) == 0 {
		return false
	}
	hasResponse := false
	for _, candidate := range turn {
		if !retention.Assess(candidate.unit).IsEligible() {
			return false
		}
		role := contextCandidateRole(candidate)
		if role == retention.RoleAssistant || role == retention.RoleTool {
			hasResponse = true
		}
	}
	return hasResponse
}
