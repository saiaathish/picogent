package session

import (
	"sort"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/retention"
)

// retentionCandidate keeps the raw, already-bounded messages beside their
// structural-only ranking projection. The projection is the only data sent to
// the provider-independent retention contract.
type retentionCandidate struct {
	messages []llm.Message
	unit     retention.Unit
	turn     int
	index    int
}

// retainMessagesByValue protects the latest user request and newest complete
// turn, then uses the S-lane ranking contract to fill the remaining bounded
// history with older complete units. Candidate selection is structural only;
// transcript text never becomes retention metadata.
func retainMessagesByValue(messages []llm.Message, base Session) []llm.Message {
	candidates, turns := retentionCandidates(messages)
	if len(candidates) == 0 {
		return nil
	}

	latestUser := -1
	for i := len(candidates) - 1; i >= 0; i-- {
		if candidateRole(candidates[i]) == retention.RoleUser {
			latestUser = i
			break
		}
	}

	latestCompleteTurn := -1
	for i := len(turns) - 1; i >= 0; i-- {
		if retentionTurnComplete(turns[i]) {
			latestCompleteTurn = i
			break
		}
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
	candidates = boundedRetentionCandidates(candidates, required, latestUser)
	if len(candidates) == 0 {
		return nil
	}

	// Rebase the contract coordinates after applying its bounded input window.
	// This keeps every coordinate valid even when a caller supplied a very long
	// legacy transcript.
	for i := range candidates {
		candidates[i].unit.Position = i
		candidates[i].unit.OriginalIndex = i
	}

	byIndex := make(map[int]retentionCandidate, len(candidates))
	for _, candidate := range candidates {
		byIndex[candidate.index] = candidate
	}

	selected := make([]retentionCandidate, 0, MaxSessionMessages)
	selectedIndexes := make(map[int]struct{}, len(required))
	// The user request is an explicit anchor. Add it before the complete-turn
	// anchor so a large turn cannot crowd the latest request out.
	if latestUser >= 0 {
		if candidate, ok := byIndex[latestUser]; ok && appendRetentionCandidate(&selected, selectedIndexes, candidate) {
			selectedIndexes[candidate.index] = struct{}{}
		}
	}
	if latestCompleteTurn >= 0 {
		// Admit the newest units first when the protected turn itself is
		// larger than the message bound. The final sort below restores their
		// original transcript order.
		for i := len(turns[latestCompleteTurn]) - 1; i >= 0; i-- {
			candidate := turns[latestCompleteTurn][i]
			candidate, ok := byIndex[candidate.index]
			if !ok {
				continue
			}
			if _, alreadySelected := selectedIndexes[candidate.index]; alreadySelected {
				continue
			}
			if appendRetentionCandidate(&selected, selectedIndexes, candidate) {
				selectedIndexes[candidate.index] = struct{}{}
			}
		}
	}

	// Only older turns are eligible for the fill pass. A current turn with a
	// pending tool call is deliberately represented by its latest user request
	// rather than by an incomplete assistant/tool fragment.
	rankInput := make([]retention.Unit, 0, len(candidates))
	rankCandidates := make([]retentionCandidate, 0, len(candidates))
	latestTurn := len(turns) - 1
	for _, candidate := range candidates {
		if candidate.turn >= latestTurn {
			continue
		}
		if _, requiredCandidate := required[candidate.index]; requiredCandidate {
			continue
		}
		rankInput = append(rankInput, candidate.unit)
		rankCandidates = append(rankCandidates, candidate)
	}
	ranked, err := retention.Rank(rankInput)
	if err == nil {
		for _, rankedCandidate := range ranked {
			if rankedCandidate.Assessment.Eligibility != retention.EligibilityEligible {
				continue
			}
			candidate := rankCandidates[rankedCandidate.InputIndex]
			if _, alreadySelected := selectedIndexes[candidate.index]; alreadySelected {
				continue
			}
			if appendRetentionCandidate(&selected, selectedIndexes, candidate) {
				selectedIndexes[candidate.index] = struct{}{}
			}
		}
	}

	sort.SliceStable(selected, func(i, j int) bool {
		return selected[i].index < selected[j].index
	})
	flat := make([]llm.Message, 0, MaxSessionMessages)
	for _, candidate := range selected {
		flat = append(flat, candidate.messages...)
	}
	if len(flat) > MaxSessionMessages || !sessionFits(base, flat) {
		if trimmed, ok := trimRetentionCandidates(selected, required, base); ok {
			return trimmed
		}
		// This is the final fail-safe for an anchor set that cannot fit by
		// itself. It preserves the existing recency-only safety behavior.
		return newestUnits(nil, flat, base)
	}
	return flat
}

// retainMessagesByRecency is the pre-M-lane eviction algorithm kept as a
// benchmark control. It intentionally remains disconnected from production so
// the comparison can measure the behavior that value-aware retention replaces.
func retainMessagesByRecency(messages []llm.Message, base Session) []llm.Message {
	turns := splitTurns(messages)
	if len(turns) == 0 {
		return newestUnits(nil, messages, base)
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

// retentionCandidates projects messageUnits without copying any content into
// the contract. Each candidate is an atomic ordinary message or complete
// assistant/tool exchange as determined by the existing structural splitter.
func retentionCandidates(messages []llm.Message) ([]retentionCandidate, [][]retentionCandidate) {
	turns := splitTurns(messages)
	candidates := make([]retentionCandidate, 0, len(messages))
	byTurn := make([][]retentionCandidate, 0, len(turns))
	for turnIndex, turn := range turns {
		turnCandidates := make([]retentionCandidate, 0, len(turn))
		for _, unitMessages := range messageUnits(turn) {
			candidate := retentionCandidate{
				messages: unitMessages,
				turn:     turnIndex,
				index:    len(candidates),
			}
			candidate.unit = retention.Unit{
				Messages:      projectRetentionMessages(unitMessages),
				CurrentTurn:   turnIndex == len(turns)-1,
				Position:      candidate.index,
				OriginalIndex: candidate.index,
			}
			candidates = append(candidates, candidate)
			turnCandidates = append(turnCandidates, candidate)
		}
		byTurn = append(byTurn, turnCandidates)
	}
	return candidates, byTurn
}

func projectRetentionMessages(messages []llm.Message) []retention.Message {
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

func candidateRole(candidate retentionCandidate) retention.Role {
	if len(candidate.unit.Messages) == 0 {
		return retention.RoleUnspecified
	}
	return candidate.unit.Messages[0].Role
}

func retentionTurnComplete(turn []retentionCandidate) bool {
	if len(turn) == 0 {
		return false
	}
	hasResponse := false
	for _, candidate := range turn {
		assessment := retention.Assess(candidate.unit)
		if !assessment.IsEligible() {
			return false
		}
		if candidateRole(candidate) == retention.RoleAssistant || candidateRole(candidate) == retention.RoleTool {
			hasResponse = true
		}
	}
	return hasResponse
}

// boundedRetentionCandidates keeps the newest contract window while making
// room for protected anchors that can sit outside it in a large legacy file.
func boundedRetentionCandidates(candidates []retentionCandidate, required map[int]struct{}, protectedIndex int) []retentionCandidate {
	if len(candidates) <= retention.MaxUnits {
		return candidates
	}

	keep := make(map[int]struct{}, retention.MaxUnits)
	if len(required) > retention.MaxUnits {
		// A single pathological turn can exceed the contract window. Keep the
		// latest request unconditionally, then keep the newest part of the
		// complete-turn anchor until the structural input bound is met.
		if protectedIndex >= 0 {
			keep[protectedIndex] = struct{}{}
		}
		requiredIndexes := make([]int, 0, len(required))
		for index := range required {
			if index != protectedIndex {
				requiredIndexes = append(requiredIndexes, index)
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(requiredIndexes)))
		for _, index := range requiredIndexes {
			if len(keep) == retention.MaxUnits {
				break
			}
			keep[index] = struct{}{}
		}
	} else {
		for index := range required {
			keep[index] = struct{}{}
		}
		for i := len(candidates) - 1; i >= 0 && len(keep) < retention.MaxUnits; i-- {
			keep[candidates[i].index] = struct{}{}
		}
	}

	indexes := make([]int, 0, len(keep))
	for index := range keep {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	out := make([]retentionCandidate, 0, len(indexes))
	for _, index := range indexes {
		out = append(out, candidates[index])
	}
	return out
}

func appendRetentionCandidate(selected *[]retentionCandidate, selectedIndexes map[int]struct{}, candidate retentionCandidate) bool {
	if _, exists := selectedIndexes[candidate.index]; exists {
		return false
	}
	messageCount := len(candidate.messages)
	for _, existing := range *selected {
		messageCount += len(existing.messages)
	}
	if messageCount > MaxSessionMessages {
		return false
	}
	*selected = append(*selected, candidate)
	return true
}

func trimRetentionCandidates(selected []retentionCandidate, required map[int]struct{}, base Session) ([]llm.Message, bool) {
	for i := len(selected) - 1; i >= 0; i-- {
		if _, isRequired := required[selected[i].index]; isRequired {
			continue
		}
		selected = append(selected[:i], selected[i+1:]...)
		flat := make([]llm.Message, 0, MaxSessionMessages)
		messageCount := 0
		for _, candidate := range selected {
			messageCount += len(candidate.messages)
			flat = append(flat, candidate.messages...)
		}
		if messageCount <= MaxSessionMessages && sessionFits(base, flat) {
			sort.SliceStable(selected, func(i, j int) bool {
				return selected[i].index < selected[j].index
			})
			ordered := make([]llm.Message, 0, len(flat))
			for _, candidate := range selected {
				ordered = append(ordered, candidate.messages...)
			}
			return ordered, true
		}
	}
	return nil, false
}
