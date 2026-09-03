// Package retention defines the provider-independent value contract used by
// a future history-retention implementation.
//
// The package deliberately accepts a structural projection of a message,
// rather than an llm.Message. Callers must decide which bounded, authoritative
// markers to provide. Transcript text, repository text, commands, tool
// arguments, and model output are not part of this API and cannot become
// retention metadata by accident.
package retention

import (
	"errors"
	"sort"
	"unicode/utf8"
)

const (
	// VocabularyVersion identifies the stable wire vocabulary. Changing an
	// allowlisted value or the ranking contract requires a new version.
	VocabularyVersion = "picogent.retention-value.v1"
	// SchemaVersion is a descriptive alias for callers that use schema
	// terminology.
	SchemaVersion = VocabularyVersion

	// MaxUnits bounds one ranking request. The S lane is a pure contract; the
	// M lane will map its existing byte/message limits onto this bound.
	MaxUnits = 256
	// MaxMessagesPerUnit bounds the structural projection of one unit.
	MaxMessagesPerUnit = 16
	// MaxToolCallsPerMessage bounds assistant tool-call identifiers.
	MaxToolCallsPerMessage = 8
	// MaxToolIDBytes bounds an identifier used only while checking pairing.
	MaxToolIDBytes = 128
	// MaxPosition bounds caller-provided ordering coordinates.
	MaxPosition = 1 << 20
	// MaxScore is the saturating upper bound for the deterministic score.
	MaxScore = 100
)

var (
	// ErrTooManyUnits reports a ranking request outside the contract bound.
	ErrTooManyUnits = errors.New("retention input exceeds unit limit")
	// ErrInvalidLimit reports a selection limit outside the contract bound.
	ErrInvalidLimit = errors.New("retention selection limit is invalid")
)

// Role is the small structural role vocabulary used by the contract.
type Role string

const (
	RoleUnspecified Role = ""
	RoleUser        Role = "user"
	RoleAssistant   Role = "assistant"
	RoleTool        Role = "tool"
	RoleSystem      Role = "system"
)

// Valid reports whether r is a recognized structural role. System is
// recognized so it can be rejected explicitly rather than treated as unknown.
func (r Role) Valid() bool {
	switch r {
	case RoleUser, RoleAssistant, RoleTool, RoleSystem:
		return true
	default:
		return false
	}
}

// OutcomeCode is an allowlisted outcome marker. It has no free-form value.
type OutcomeCode string

const (
	OutcomeUnmarked  OutcomeCode = "unmarked"
	OutcomeCompleted OutcomeCode = "completed"
	OutcomeChanged   OutcomeCode = "changed"
	OutcomeBlocked   OutcomeCode = "blocked"
	OutcomeFailed    OutcomeCode = "failed"
)

// Valid reports whether c is in the versioned outcome vocabulary. The empty
// value is accepted as the Go zero value and normalized to OutcomeUnmarked.
func (c OutcomeCode) Valid() bool {
	switch c {
	case "", OutcomeUnmarked, OutcomeCompleted, OutcomeChanged, OutcomeBlocked, OutcomeFailed:
		return true
	default:
		return false
	}
}

// VerificationCode is an allowlisted verification marker.
type VerificationCode string

const (
	VerificationUnmarked     VerificationCode = "unmarked"
	VerificationPassed       VerificationCode = "passed"
	VerificationFailed       VerificationCode = "failed"
	VerificationInconclusive VerificationCode = "inconclusive"
)

// Valid reports whether c is in the versioned verification vocabulary.
func (c VerificationCode) Valid() bool {
	switch c {
	case "", VerificationUnmarked, VerificationPassed, VerificationFailed, VerificationInconclusive:
		return true
	default:
		return false
	}
}

// RecoveryCode is an allowlisted recovery marker.
type RecoveryCode string

const (
	RecoveryUnmarked RecoveryCode = "unmarked"
	RecoveryResumed  RecoveryCode = "resumed"
	RecoveryRestored RecoveryCode = "restored"
	RecoveryRepaired RecoveryCode = "repaired"
)

// Valid reports whether c is in the versioned recovery vocabulary.
func (c RecoveryCode) Valid() bool {
	switch c {
	case "", RecoveryUnmarked, RecoveryResumed, RecoveryRestored, RecoveryRepaired:
		return true
	default:
		return false
	}
}

// ErrorCode is an allowlisted error marker. Error text itself is never part
// of a Unit or Assessment.
type ErrorCode string

const (
	ErrorUnmarked   ErrorCode = "unmarked"
	ErrorObserved   ErrorCode = "observed"
	ErrorResolved   ErrorCode = "resolved"
	ErrorUnresolved ErrorCode = "unresolved"
)

// Valid reports whether c is in the versioned error vocabulary.
func (c ErrorCode) Valid() bool {
	switch c {
	case "", ErrorUnmarked, ErrorObserved, ErrorResolved, ErrorUnresolved:
		return true
	default:
		return false
	}
}

// Markers contains only typed, allowlisted signals. A zero-valued field means
// unmarked and is normalized in an eligible Assessment.
type Markers struct {
	Outcome      OutcomeCode      `json:"outcome"`
	Verification VerificationCode `json:"verification"`
	Recovery     RecoveryCode     `json:"recovery"`
	Error        ErrorCode        `json:"error"`
}

// Valid reports whether every marker is recognized. It does not normalize the
// zero value; callers that retain a value should use the normalized result
// produced by Assess.
func (m Markers) Valid() bool {
	return m.Outcome.Valid() && m.Verification.Valid() && m.Recovery.Valid() && m.Error.Valid()
}

func normalizeMarkers(m Markers) (Markers, bool) {
	if !m.Valid() {
		return Markers{}, false
	}
	if m.Outcome == "" {
		m.Outcome = OutcomeUnmarked
	}
	if m.Verification == "" {
		m.Verification = VerificationUnmarked
	}
	if m.Recovery == "" {
		m.Recovery = RecoveryUnmarked
	}
	if m.Error == "" {
		m.Error = ErrorUnmarked
	}
	return m, true
}

// Message is the bounded structural projection required to check a unit. It
// intentionally has no content, name, command, argument, or model-output
// field. ToolCallIDs is meaningful only for assistant messages; ToolCallID is
// meaningful only for tool messages.
type Message struct {
	Role        Role     `json:"role"`
	ToolCallIDs []string `json:"tool_call_ids,omitempty"`
	ToolCallID  string   `json:"tool_call_id,omitempty"`
}

// Unit is one candidate history unit. Position and OriginalIndex are caller
// supplied coordinates, not transcript text. OriginalIndex is the final
// deterministic tie-breaker and is also used to restore original order after
// selection.
type Unit struct {
	Messages      []Message `json:"messages"`
	CurrentTurn   bool      `json:"current_turn"`
	Position      int       `json:"position"`
	OriginalIndex int       `json:"original_index"`
	Markers       Markers   `json:"markers"`
}

// ToolPairStatus reports whether a unit contains a complete assistant/tool
// exchange. A unit without tool calls is complete but has NotPresent status.
type ToolPairStatus string

const (
	ToolPairNotPresent ToolPairStatus = "not-present"
	ToolPairComplete   ToolPairStatus = "complete"
)

// Eligibility is deliberately tri-state. Unknown or malformed data is never
// promoted to eligible, while structurally orphaned or incomplete tool units
// remain explicitly ineligible.
type Eligibility string

const (
	EligibilityUnverified Eligibility = "UNVERIFIED"
	EligibilityIneligible Eligibility = "INELIGIBLE"
	EligibilityEligible   Eligibility = "ELIGIBLE"
)

// ReasonCode is a closed explanation vocabulary. It is safe to expose in
// logs or durable metadata because it contains no caller-provided text.
type ReasonCode string

const (
	ReasonEligible           ReasonCode = "eligible"
	ReasonEmptyUnit          ReasonCode = "empty-unit"
	ReasonInputTooLarge      ReasonCode = "input-too-large"
	ReasonInvalidPosition    ReasonCode = "invalid-position"
	ReasonUnknownRole        ReasonCode = "unknown-role"
	ReasonSystemMessage      ReasonCode = "system-message"
	ReasonUnknownMarker      ReasonCode = "unknown-marker"
	ReasonMalformedInput     ReasonCode = "malformed-input"
	ReasonOrphanToolResult   ReasonCode = "orphan-tool-result"
	ReasonIncompleteToolPair ReasonCode = "incomplete-tool-pair"
)

// Assessment is the complete output of Assess. It contains only bounded
// enums, booleans, and coordinates; it never echoes a Message or identifier.
type Assessment struct {
	Version       string         `json:"version"`
	Eligibility   Eligibility    `json:"eligibility"`
	Reason        ReasonCode     `json:"reason"`
	Role          Role           `json:"role,omitempty"`
	ToolPair      ToolPairStatus `json:"tool_pair"`
	CurrentTurn   bool           `json:"current_turn"`
	Position      int            `json:"position"`
	OriginalIndex int            `json:"original_index"`
	Score         uint8          `json:"score"`
	Markers       Markers        `json:"markers"`
}

// IsEligible is a convenience predicate for callers that must fail closed.
func (a Assessment) IsEligible() bool {
	return a.Eligibility == EligibilityEligible
}

// Assess validates one unit and computes its deterministic value. Unknown or
// malformed input returns UNVERIFIED. Orphaned and incomplete tool-call pairs
// return INELIGIBLE, so neither class can be selected by Select.
func Assess(unit Unit) Assessment {
	a := Assessment{
		Version:     VocabularyVersion,
		Eligibility: EligibilityUnverified,
		Reason:      ReasonMalformedInput,
		ToolPair:    ToolPairNotPresent,
		Markers: Markers{
			Outcome:      OutcomeUnmarked,
			Verification: VerificationUnmarked,
			Recovery:     RecoveryUnmarked,
			Error:        ErrorUnmarked,
		},
	}
	if len(unit.Messages) == 0 {
		a.Reason = ReasonEmptyUnit
		return a
	}
	if len(unit.Messages) > MaxMessagesPerUnit {
		a.Reason = ReasonInputTooLarge
		return a
	}
	if unit.Position < 0 || unit.Position > MaxPosition || unit.OriginalIndex < 0 || unit.OriginalIndex > MaxPosition {
		a.Reason = ReasonInvalidPosition
		return a
	}
	markers, ok := normalizeMarkers(unit.Markers)
	if !ok {
		a.Reason = ReasonUnknownMarker
		return a
	}

	role, pair, reason := validateMessages(unit.Messages)
	if reason != "" {
		a.Reason = reason
		if reason == ReasonSystemMessage || reason == ReasonOrphanToolResult || reason == ReasonIncompleteToolPair {
			a.Eligibility = EligibilityIneligible
		}
		return a
	}

	a.Eligibility = EligibilityEligible
	a.Reason = ReasonEligible
	a.Role = role
	a.ToolPair = pair
	a.CurrentTurn = unit.CurrentTurn
	a.Position = unit.Position
	a.OriginalIndex = unit.OriginalIndex
	a.Markers = markers
	a.Score = score(role, pair, markers)
	return a
}

func validateMessages(messages []Message) (Role, ToolPairStatus, ReasonCode) {
	if reason := validateMessageShapes(messages); reason != "" {
		return "", ToolPairNotPresent, reason
	}

	var primary Role
	pending := make(map[string]struct{})
	pair := ToolPairNotPresent
	for _, message := range messages {
		switch message.Role {
		case RoleSystem:
			return RoleSystem, ToolPairNotPresent, ReasonSystemMessage
		case RoleUser, RoleAssistant:
			if len(pending) != 0 {
				return primary, pair, ReasonIncompleteToolPair
			}
			if primary == "" {
				primary = message.Role
			}
			if message.Role != RoleAssistant {
				continue
			}
			for _, id := range message.ToolCallIDs {
				pending[id] = struct{}{}
			}
			if len(message.ToolCallIDs) != 0 {
				pair = ToolPairComplete
			}
		case RoleTool:
			if primary == "" || len(pending) == 0 || !validToolID(message.ToolCallID) {
				return primary, pair, ReasonOrphanToolResult
			}
			if _, exists := pending[message.ToolCallID]; !exists {
				return primary, pair, ReasonOrphanToolResult
			}
			delete(pending, message.ToolCallID)
		}
	}
	if len(pending) != 0 {
		return primary, pair, ReasonIncompleteToolPair
	}
	if primary == "" {
		return "", ToolPairNotPresent, ReasonMalformedInput
	}
	return primary, pair, ""
}

// validateMessageShapes checks all role-specific fields before pairing
// semantics run. This keeps malformed structural input UNVERIFIED even when
// a later semantic rule would otherwise call it an orphan or incomplete pair.
func validateMessageShapes(messages []Message) ReasonCode {
	seenCallIDs := make(map[string]struct{})
	for _, message := range messages {
		if !message.Role.Valid() {
			return ReasonUnknownRole
		}
		if len(message.ToolCallIDs) > MaxToolCallsPerMessage {
			return ReasonInputTooLarge
		}

		switch message.Role {
		case RoleAssistant:
			if message.ToolCallID != "" {
				return ReasonMalformedInput
			}
			for _, id := range message.ToolCallIDs {
				if !validToolID(id) {
					return ReasonMalformedInput
				}
				if _, exists := seenCallIDs[id]; exists {
					return ReasonMalformedInput
				}
				seenCallIDs[id] = struct{}{}
			}
		case RoleUser, RoleSystem:
			if len(message.ToolCallIDs) != 0 || message.ToolCallID != "" {
				return ReasonMalformedInput
			}
		case RoleTool:
			if len(message.ToolCallIDs) != 0 || !validToolID(message.ToolCallID) {
				return ReasonMalformedInput
			}
		}
	}
	return ""
}

func validToolID(id string) bool {
	return id != "" && len(id) <= MaxToolIDBytes && utf8.ValidString(id)
}

func score(role Role, pair ToolPairStatus, markers Markers) uint8 {
	value := 0
	switch role {
	case RoleUser:
		value += 24
	case RoleAssistant:
		value += 16
	case RoleTool:
		value += 8
	}
	if pair == ToolPairComplete {
		value += 12
	}
	switch markers.Outcome {
	case OutcomeCompleted, OutcomeChanged:
		value += 20
	case OutcomeBlocked, OutcomeFailed:
		value += 14
	}
	switch markers.Verification {
	case VerificationPassed, VerificationFailed:
		value += 18
	case VerificationInconclusive:
		value += 12
	}
	switch markers.Recovery {
	case RecoveryResumed, RecoveryRestored, RecoveryRepaired:
		value += 14
	}
	switch markers.Error {
	case ErrorObserved:
		value += 16
	case ErrorResolved:
		value += 12
	case ErrorUnresolved:
		value += 20
	}
	if value > MaxScore {
		value = MaxScore
	}
	return uint8(value)
}

// RankedUnit pairs a bounded assessment with its position in the input slice.
// InputIndex is not a transcript value; it is a final deterministic tie-break.
type RankedUnit struct {
	InputIndex    int        `json:"input_index"`
	OriginalIndex int        `json:"original_index"`
	Assessment    Assessment `json:"assessment"`
}

// Rank returns all candidates in deterministic preference order:
// eligibility, score, current-turn priority, newer position, original index,
// and finally input index. It does not mutate units or select/evict history.
func Rank(units []Unit) ([]RankedUnit, error) {
	if len(units) > MaxUnits {
		return nil, ErrTooManyUnits
	}
	ranked := make([]RankedUnit, len(units))
	for i, unit := range units {
		assessment := Assess(unit)
		ranked[i] = RankedUnit{
			InputIndex:    i,
			OriginalIndex: assessment.OriginalIndex,
			Assessment:    assessment,
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		left, right := ranked[i].Assessment, ranked[j].Assessment
		if eligibilityRank(left.Eligibility) != eligibilityRank(right.Eligibility) {
			return eligibilityRank(left.Eligibility) > eligibilityRank(right.Eligibility)
		}
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if left.CurrentTurn != right.CurrentTurn {
			return left.CurrentTurn
		}
		if left.Position != right.Position {
			return left.Position > right.Position
		}
		if left.OriginalIndex != right.OriginalIndex {
			return left.OriginalIndex < right.OriginalIndex
		}
		return ranked[i].InputIndex < ranked[j].InputIndex
	})
	return ranked, nil
}

func eligibilityRank(value Eligibility) int {
	switch value {
	case EligibilityEligible:
		return 2
	case EligibilityIneligible:
		return 1
	default:
		return 0
	}
}

// Select returns at most limit eligible candidates from Rank, restoring the
// selected candidates to original transcript order. It is a pure contract
// helper and is not wired into session eviction in the S lane.
func Select(units []Unit, limit int) ([]RankedUnit, error) {
	if limit < 0 || limit > MaxUnits {
		return nil, ErrInvalidLimit
	}
	if limit == 0 {
		return []RankedUnit{}, nil
	}
	ranked, err := Rank(units)
	if err != nil {
		return nil, err
	}
	selected := make([]RankedUnit, 0, limit)
	for _, candidate := range ranked {
		if !candidate.Assessment.IsEligible() {
			continue
		}
		selected = append(selected, candidate)
		if len(selected) == limit {
			break
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].OriginalIndex != selected[j].OriginalIndex {
			return selected[i].OriginalIndex < selected[j].OriginalIndex
		}
		return selected[i].InputIndex < selected[j].InputIndex
	})
	return selected, nil
}
