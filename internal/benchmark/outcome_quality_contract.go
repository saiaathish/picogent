package benchmark

import (
	"fmt"
	"strings"
)

// OutcomeQualitySchema identifies the bounded v3-versus-v4 benchmark report.
// It is an ephemeral measurement format, not a task-state or completion
// authority.
const OutcomeQualitySchema = "picogent.v4.outcome-quality.v1"

const (
	OutcomeQualityScenarioSet = "v3-v4-outcome-quality"

	MaxOutcomeQualityScenarios     = 32
	MaxOutcomeQualityObservations  = 4096
	MaxOutcomeQualityRepetitions   = 32
	MaxOutcomeQualityTextBytes     = 512
	MaxOutcomeQualityFailures      = 64
	MaxOutcomeQualityUnverified    = 64
	MaxOutcomeQualityMetricCount   = 1_000_000
	MaxOutcomeQualityContextGrowth = 128 * 1024 * 1024
	MaxOutcomeQualityLatencyMillis = 24 * 60 * 60 * 1000
)

// OutcomeQualityScenarioCategory is the stable top-level taxonomy from the
// v4 benchmark brief. Repository or provider text cannot add a category.
type OutcomeQualityScenarioCategory string

const (
	OutcomeCategoryBeginner            OutcomeQualityScenarioCategory = "beginner"
	OutcomeCategoryStandardDevelopment OutcomeQualityScenarioCategory = "standard_development"
	OutcomeCategoryAdvanced            OutcomeQualityScenarioCategory = "advanced"
	OutcomeCategoryProduct             OutcomeQualityScenarioCategory = "product"
	OutcomeCategoryRobustness          OutcomeQualityScenarioCategory = "robustness"
	OutcomeCategoryLongHorizon         OutcomeQualityScenarioCategory = "long_horizon"
)

func (c OutcomeQualityScenarioCategory) valid() bool {
	switch c {
	case OutcomeCategoryBeginner, OutcomeCategoryStandardDevelopment, OutcomeCategoryAdvanced,
		OutcomeCategoryProduct, OutcomeCategoryRobustness, OutcomeCategoryLongHorizon:
		return true
	default:
		return false
	}
}

// OutcomeQualityScenarioKind identifies the bounded task shape inside a
// category. The catalog is deliberately small and deterministic; it is not a
// claim that these fixtures represent all real repositories or providers.
type OutcomeQualityScenarioKind string

const (
	OutcomeKindVagueFeature          OutcomeQualityScenarioKind = "vague_feature"
	OutcomeKindBrokenApp             OutcomeQualityScenarioKind = "broken_app"
	OutcomeKindSetupProblem          OutcomeQualityScenarioKind = "setup_problem"
	OutcomeKindBug                   OutcomeQualityScenarioKind = "bug"
	OutcomeKindFeature               OutcomeQualityScenarioKind = "feature"
	OutcomeKindRefactor              OutcomeQualityScenarioKind = "refactor"
	OutcomeKindTests                 OutcomeQualityScenarioKind = "tests"
	OutcomeKindMigration             OutcomeQualityScenarioKind = "migration"
	OutcomeKindArchitecture          OutcomeQualityScenarioKind = "architecture"
	OutcomeKindPerformance           OutcomeQualityScenarioKind = "performance"
	OutcomeKindSecurity              OutcomeQualityScenarioKind = "security"
	OutcomeKindUIPolish              OutcomeQualityScenarioKind = "ui_polish"
	OutcomeKindOnboarding            OutcomeQualityScenarioKind = "onboarding"
	OutcomeKindLaunchReadiness       OutcomeQualityScenarioKind = "launch_readiness"
	OutcomeKindResume                OutcomeQualityScenarioKind = "resume"
	OutcomeKindCancel                OutcomeQualityScenarioKind = "cancel"
	OutcomeKindSteer                 OutcomeQualityScenarioKind = "steer"
	OutcomeKindUndo                  OutcomeQualityScenarioKind = "undo"
	OutcomeKindConflictingEdits      OutcomeQualityScenarioKind = "conflicting_edits"
	OutcomeKindMultiStageImprovement OutcomeQualityScenarioKind = "multi_stage_improvement"
)

func (k OutcomeQualityScenarioKind) valid() bool {
	switch k {
	case OutcomeKindVagueFeature, OutcomeKindBrokenApp, OutcomeKindSetupProblem,
		OutcomeKindBug, OutcomeKindFeature, OutcomeKindRefactor, OutcomeKindTests,
		OutcomeKindMigration, OutcomeKindArchitecture, OutcomeKindPerformance, OutcomeKindSecurity,
		OutcomeKindUIPolish, OutcomeKindOnboarding, OutcomeKindLaunchReadiness,
		OutcomeKindResume, OutcomeKindCancel, OutcomeKindSteer, OutcomeKindUndo,
		OutcomeKindConflictingEdits, OutcomeKindMultiStageImprovement:
		return true
	default:
		return false
	}
}

// OutcomeQualityScenario is a stable fixture definition. InputSHA256 is
// populated in a report after the runner binds the exact prompt/fixture input;
// the catalog intentionally leaves it empty.
type OutcomeQualityScenario struct {
	ID          string                         `json:"id"`
	Category    OutcomeQualityScenarioCategory `json:"category"`
	Kind        OutcomeQualityScenarioKind     `json:"kind"`
	Seed        uint64                         `json:"seed"`
	InputSHA256 string                         `json:"input_sha256"`
}

var outcomeQualityScenarioCatalog = []OutcomeQualityScenario{
	{ID: "beginner-vague-feature", Category: OutcomeCategoryBeginner, Kind: OutcomeKindVagueFeature, Seed: 101},
	{ID: "beginner-broken-app", Category: OutcomeCategoryBeginner, Kind: OutcomeKindBrokenApp, Seed: 102},
	{ID: "beginner-setup-problem", Category: OutcomeCategoryBeginner, Kind: OutcomeKindSetupProblem, Seed: 103},
	{ID: "standard-bug", Category: OutcomeCategoryStandardDevelopment, Kind: OutcomeKindBug, Seed: 201},
	{ID: "standard-feature", Category: OutcomeCategoryStandardDevelopment, Kind: OutcomeKindFeature, Seed: 202},
	{ID: "standard-refactor", Category: OutcomeCategoryStandardDevelopment, Kind: OutcomeKindRefactor, Seed: 203},
	{ID: "standard-tests", Category: OutcomeCategoryStandardDevelopment, Kind: OutcomeKindTests, Seed: 204},
	{ID: "advanced-migration", Category: OutcomeCategoryAdvanced, Kind: OutcomeKindMigration, Seed: 301},
	{ID: "advanced-architecture", Category: OutcomeCategoryAdvanced, Kind: OutcomeKindArchitecture, Seed: 302},
	{ID: "advanced-performance", Category: OutcomeCategoryAdvanced, Kind: OutcomeKindPerformance, Seed: 303},
	{ID: "advanced-security", Category: OutcomeCategoryAdvanced, Kind: OutcomeKindSecurity, Seed: 304},
	{ID: "product-ui-polish", Category: OutcomeCategoryProduct, Kind: OutcomeKindUIPolish, Seed: 401},
	{ID: "product-onboarding", Category: OutcomeCategoryProduct, Kind: OutcomeKindOnboarding, Seed: 402},
	{ID: "product-launch-readiness", Category: OutcomeCategoryProduct, Kind: OutcomeKindLaunchReadiness, Seed: 403},
	{ID: "robustness-resume", Category: OutcomeCategoryRobustness, Kind: OutcomeKindResume, Seed: 501},
	{ID: "robustness-cancel", Category: OutcomeCategoryRobustness, Kind: OutcomeKindCancel, Seed: 502},
	{ID: "robustness-steer", Category: OutcomeCategoryRobustness, Kind: OutcomeKindSteer, Seed: 503},
	{ID: "robustness-undo", Category: OutcomeCategoryRobustness, Kind: OutcomeKindUndo, Seed: 504},
	{ID: "robustness-conflicting-edits", Category: OutcomeCategoryRobustness, Kind: OutcomeKindConflictingEdits, Seed: 505},
	{ID: "long-horizon-multi-stage", Category: OutcomeCategoryLongHorizon, Kind: OutcomeKindMultiStageImprovement, Seed: 601},
}

// DefaultOutcomeQualityScenarios returns a copy so callers cannot mutate the
// process-wide catalog and silently change report ordering.
func DefaultOutcomeQualityScenarios() []OutcomeQualityScenario {
	result := make([]OutcomeQualityScenario, len(outcomeQualityScenarioCatalog))
	copy(result, outcomeQualityScenarioCatalog)
	return result
}

// OutcomeQualityVariant identifies the two source heads being compared.
type OutcomeQualityVariant string

const (
	OutcomeVariantBaseline  OutcomeQualityVariant = "baseline"
	OutcomeVariantCandidate OutcomeQualityVariant = "candidate"
)

func (v OutcomeQualityVariant) valid() bool {
	return v == OutcomeVariantBaseline || v == OutcomeVariantCandidate
}

// OutcomeQualityReportStatus describes report coverage, not task success.
type OutcomeQualityReportStatus string

const (
	OutcomeReportComplete     OutcomeQualityReportStatus = "complete"
	OutcomeReportInconclusive OutcomeQualityReportStatus = "inconclusive"
	OutcomeReportUnverified   OutcomeQualityReportStatus = "unverified"
)

func (s OutcomeQualityReportStatus) valid() bool {
	switch s {
	case OutcomeReportComplete, OutcomeReportInconclusive, OutcomeReportUnverified:
		return true
	default:
		return false
	}
}

// OutcomeQualityAssessment is deliberately four-valued. A missing or
// unavailable measurement cannot be serialized as a successful result.
type OutcomeQualityAssessment string

const (
	OutcomeAssessmentPass         OutcomeQualityAssessment = "pass"
	OutcomeAssessmentFail         OutcomeQualityAssessment = "fail"
	OutcomeAssessmentInconclusive OutcomeQualityAssessment = "inconclusive"
	OutcomeAssessmentUnverified   OutcomeQualityAssessment = "unverified"
)

func (a OutcomeQualityAssessment) valid() bool {
	switch a {
	case OutcomeAssessmentPass, OutcomeAssessmentFail, OutcomeAssessmentInconclusive, OutcomeAssessmentUnverified:
		return true
	default:
		return false
	}
}

// OutcomeQualityVerification records the quality of the verification result.
// VerificationPass is only meaningful with current criterion-bound evidence.
type OutcomeQualityVerification string

const (
	OutcomeVerificationPass         OutcomeQualityVerification = "pass"
	OutcomeVerificationFail         OutcomeQualityVerification = "fail"
	OutcomeVerificationInconclusive OutcomeQualityVerification = "inconclusive"
	OutcomeVerificationSkipped      OutcomeQualityVerification = "skipped"
	OutcomeVerificationUnverified   OutcomeQualityVerification = "unverified"
)

func (v OutcomeQualityVerification) valid() bool {
	switch v {
	case OutcomeVerificationPass, OutcomeVerificationFail, OutcomeVerificationInconclusive,
		OutcomeVerificationSkipped, OutcomeVerificationUnverified:
		return true
	default:
		return false
	}
}

// OutcomeQualityPolicy is shared by both source-head variants. Keeping it at
// report scope prevents a runner from silently giving v4 a different budget.
type OutcomeQualityPolicy struct {
	Repetitions   int   `json:"repetitions"`
	TimeoutMillis int64 `json:"timeout_millis"`
	MaxTokens     int   `json:"max_tokens"`
	MaxModelCalls int   `json:"max_model_calls"`
	MaxToolCalls  int   `json:"max_tool_calls"`
	MaxTurns      int   `json:"max_turns"`
}

// OutcomeQualityTarget carries exact source and tool provenance for one side
// of the comparison. The two targets must otherwise share their environment.
type OutcomeQualityTarget struct {
	SourceHead  string `json:"source_head"`
	Host        string `json:"host"`
	GoVersion   string `json:"go_version"`
	ToolVersion string `json:"tool_version"`
}

// OutcomeQualityMetrics contains the measurements required by the v4 brief.
// Counts are bounded and zero is a valid measured value.
type OutcomeQualityMetrics struct {
	OutcomeSuccess      OutcomeQualityAssessment   `json:"outcome_success"`
	Correctness         OutcomeQualityAssessment   `json:"correctness"`
	UserQuestions       int                        `json:"user_questions"`
	Tokens              int                        `json:"tokens"`
	ModelCalls          int                        `json:"model_calls"`
	ToolCalls           int                        `json:"tool_calls"`
	LatencyMillis       int64                      `json:"latency_millis"`
	ChangedLines        int                        `json:"changed_lines"`
	UnnecessaryChanges  int                        `json:"unnecessary_changes"`
	VerificationQuality OutcomeQualityVerification `json:"verification_quality"`
	RepairCount         int                        `json:"repair_count"`
	ContextGrowthBytes  int64                      `json:"context_growth_bytes"`
	Evidence            EvidenceState              `json:"evidence"`
}

// OutcomeQualityObservation is one scenario/variant/repetition result. The
// source head is repeated here so a copied or merged observation cannot be
// detached from the target it claims to measure.
type OutcomeQualityObservation struct {
	ScenarioID string                `json:"scenario_id"`
	Variant    OutcomeQualityVariant `json:"variant"`
	Repetition int                   `json:"repetition"`
	SourceHead string                `json:"source_head"`
	Metrics    OutcomeQualityMetrics `json:"metrics"`
	Unverified []string              `json:"unverified,omitempty"`
}

// OutcomeQualityReport is an ephemeral, bounded comparison report. It does
// not authorize a release and cannot turn deterministic fixture success into a
// general autonomous-coding quality claim.
type OutcomeQualityReport struct {
	Schema            string                      `json:"schema"`
	ScenarioSet       string                      `json:"scenario_set"`
	Status            OutcomeQualityReportStatus  `json:"status"`
	Baseline          OutcomeQualityTarget        `json:"baseline"`
	Candidate         OutcomeQualityTarget        `json:"candidate"`
	Policy            OutcomeQualityPolicy        `json:"policy"`
	Command           string                      `json:"command"`
	Scenarios         []OutcomeQualityScenario    `json:"scenarios"`
	Observations      []OutcomeQualityObservation `json:"observations"`
	InvariantFailures []string                    `json:"invariant_failures,omitempty"`
	Unverified        []string                    `json:"unverified,omitempty"`
}

// Validate enforces reproducibility, boundedness, deterministic ordering, and
// the fail-closed rule that a passing outcome requires current verification.
func (r OutcomeQualityReport) Validate() error {
	if r.Schema != OutcomeQualitySchema {
		return fmt.Errorf("schema=%q, want %q", r.Schema, OutcomeQualitySchema)
	}
	if err := validateText("scenario_set", r.ScenarioSet, true); err != nil {
		return err
	}
	if r.ScenarioSet != OutcomeQualityScenarioSet {
		return fmt.Errorf("scenario_set=%q, want %q", r.ScenarioSet, OutcomeQualityScenarioSet)
	}
	if !r.Status.valid() {
		return fmt.Errorf("unknown report status %q", r.Status)
	}
	if err := validateText("command", r.Command, true); err != nil {
		return err
	}
	if err := validateOutcomeQualityTarget("baseline", r.Baseline); err != nil {
		return err
	}
	if err := validateOutcomeQualityTarget("candidate", r.Candidate); err != nil {
		return err
	}
	if r.Baseline.SourceHead == r.Candidate.SourceHead {
		return fmt.Errorf("baseline and candidate must use different source heads")
	}
	if r.Baseline.Host != r.Candidate.Host || r.Baseline.GoVersion != r.Candidate.GoVersion || r.Baseline.ToolVersion != r.Candidate.ToolVersion {
		return fmt.Errorf("baseline and candidate must share host, Go version, and tool version")
	}
	if err := validateOutcomeQualityPolicy(r.Policy); err != nil {
		return err
	}

	wantScenarios := outcomeQualityScenarioCatalog
	if len(r.Scenarios) != len(wantScenarios) || len(r.Scenarios) > MaxOutcomeQualityScenarios {
		return fmt.Errorf("scenarios=%d, want exactly %d", len(r.Scenarios), len(wantScenarios))
	}
	for index, scenario := range r.Scenarios {
		want := wantScenarios[index]
		if scenario.ID != want.ID || scenario.Category != want.Category || scenario.Kind != want.Kind || scenario.Seed != want.Seed {
			return fmt.Errorf("scenario %d does not match the stable catalog: got=%#v want=%#v", index, scenario, want)
		}
		if !validSHA256(scenario.InputSHA256) {
			return fmt.Errorf("scenario %d input_sha256 must be a lowercase 64-character SHA-256 digest", index)
		}
	}
	if err := validateTextList("invariant_failures", r.InvariantFailures, MaxOutcomeQualityFailures); err != nil {
		return err
	}
	if err := validateTextList("unverified", r.Unverified, MaxOutcomeQualityUnverified); err != nil {
		return err
	}
	if r.Status == OutcomeReportUnverified && len(r.Unverified) == 0 {
		return fmt.Errorf("unverified report must record an unverified reason")
	}
	if r.Status == OutcomeReportInconclusive && len(r.Unverified) == 0 && len(r.InvariantFailures) == 0 {
		return fmt.Errorf("inconclusive report must record an unverified reason or invariant failure")
	}

	expected := len(r.Scenarios) * 2 * r.Policy.Repetitions
	if expected > MaxOutcomeQualityObservations {
		return fmt.Errorf("expected observations=%d exceeds %d", expected, MaxOutcomeQualityObservations)
	}
	if len(r.Observations) > expected {
		return fmt.Errorf("observations=%d exceeds expected maximum %d", len(r.Observations), expected)
	}
	if r.Status == OutcomeReportComplete && len(r.Observations) != expected {
		return fmt.Errorf("complete report has %d observations, want %d", len(r.Observations), expected)
	}
	if r.Status == OutcomeReportComplete && len(r.Observations) == 0 {
		return fmt.Errorf("complete report must contain observations")
	}

	scenarioIndex := make(map[string]int, len(r.Scenarios))
	for index, scenario := range r.Scenarios {
		scenarioIndex[scenario.ID] = index
	}
	previousRank := -1
	for index, observation := range r.Observations {
		rank, err := outcomeQualityObservationRank(observation, scenarioIndex, r.Policy.Repetitions)
		if err != nil {
			return fmt.Errorf("observation %d: %w", index, err)
		}
		if rank <= previousRank {
			return fmt.Errorf("observation %d is not in deterministic scenario, variant, repetition order", index)
		}
		previousRank = rank

		var target OutcomeQualityTarget
		switch observation.Variant {
		case OutcomeVariantBaseline:
			target = r.Baseline
		case OutcomeVariantCandidate:
			target = r.Candidate
		}
		if observation.SourceHead != target.SourceHead {
			return fmt.Errorf("observation %d source_head=%q does not match %s target", index, observation.SourceHead, observation.Variant)
		}
		if err := validateOutcomeQualityMetrics(observation.Metrics, r.Policy, index); err != nil {
			return err
		}
		if err := validateTextList(fmt.Sprintf("observation %d unverified", index), observation.Unverified, 8); err != nil {
			return err
		}
		if len(r.InvariantFailures) > 0 && observation.Metrics.OutcomeSuccess == OutcomeAssessmentPass {
			return fmt.Errorf("observation %d claims success while invariant failures are recorded", index)
		}
	}
	return nil
}

func validateOutcomeQualityTarget(name string, target OutcomeQualityTarget) error {
	if !validSHA(target.SourceHead) {
		return fmt.Errorf("%s source_head must be a full 40-character commit SHA", name)
	}
	for field, value := range map[string]string{
		name + ".host":         target.Host,
		name + ".go_version":   target.GoVersion,
		name + ".tool_version": target.ToolVersion,
	} {
		if err := validateText(field, value, true); err != nil {
			return err
		}
	}
	return nil
}

func validateOutcomeQualityPolicy(policy OutcomeQualityPolicy) error {
	if policy.Repetitions < 2 || policy.Repetitions > MaxOutcomeQualityRepetitions {
		return fmt.Errorf("repetitions=%d outside 2..%d", policy.Repetitions, MaxOutcomeQualityRepetitions)
	}
	if policy.TimeoutMillis <= 0 || policy.TimeoutMillis > MaxOutcomeQualityLatencyMillis {
		return fmt.Errorf("timeout_millis=%d outside 1..%d", policy.TimeoutMillis, MaxOutcomeQualityLatencyMillis)
	}
	for name, value := range map[string]int{
		"max_tokens":      policy.MaxTokens,
		"max_model_calls": policy.MaxModelCalls,
		"max_tool_calls":  policy.MaxToolCalls,
		"max_turns":       policy.MaxTurns,
	} {
		if value <= 0 || value > MaxOutcomeQualityMetricCount {
			return fmt.Errorf("%s=%d outside 1..%d", name, value, MaxOutcomeQualityMetricCount)
		}
	}
	return nil
}

func validateOutcomeQualityMetrics(metrics OutcomeQualityMetrics, policy OutcomeQualityPolicy, index int) error {
	if !metrics.OutcomeSuccess.valid() {
		return fmt.Errorf("observation %d has unknown outcome_success %q", index, metrics.OutcomeSuccess)
	}
	if !metrics.Correctness.valid() {
		return fmt.Errorf("observation %d has unknown correctness %q", index, metrics.Correctness)
	}
	if !metrics.VerificationQuality.valid() {
		return fmt.Errorf("observation %d has unknown verification_quality %q", index, metrics.VerificationQuality)
	}
	if !metrics.Evidence.valid() {
		return fmt.Errorf("observation %d has unknown evidence %q", index, metrics.Evidence)
	}
	for name, value := range map[string]int{
		"user_questions":      metrics.UserQuestions,
		"tokens":              metrics.Tokens,
		"model_calls":         metrics.ModelCalls,
		"tool_calls":          metrics.ToolCalls,
		"changed_lines":       metrics.ChangedLines,
		"unnecessary_changes": metrics.UnnecessaryChanges,
		"repair_count":        metrics.RepairCount,
	} {
		if value < 0 || value > MaxOutcomeQualityMetricCount {
			return fmt.Errorf("observation %d %s=%d outside 0..%d", index, name, value, MaxOutcomeQualityMetricCount)
		}
	}
	if metrics.Tokens > policy.MaxTokens || metrics.ModelCalls > policy.MaxModelCalls || metrics.ToolCalls > policy.MaxToolCalls {
		return fmt.Errorf("observation %d exceeds the shared benchmark budget", index)
	}
	if metrics.LatencyMillis < 0 || metrics.LatencyMillis > MaxOutcomeQualityLatencyMillis {
		return fmt.Errorf("observation %d latency_millis=%d outside 0..%d", index, metrics.LatencyMillis, MaxOutcomeQualityLatencyMillis)
	}
	if metrics.ContextGrowthBytes < 0 || metrics.ContextGrowthBytes > MaxOutcomeQualityContextGrowth {
		return fmt.Errorf("observation %d context_growth_bytes=%d outside 0..%d", index, metrics.ContextGrowthBytes, MaxOutcomeQualityContextGrowth)
	}
	if metrics.VerificationQuality == OutcomeVerificationPass && metrics.Evidence != EvidenceCurrent {
		return fmt.Errorf("observation %d passing verification requires current evidence", index)
	}
	if metrics.Correctness == OutcomeAssessmentPass && (metrics.VerificationQuality != OutcomeVerificationPass || metrics.Evidence != EvidenceCurrent) {
		return fmt.Errorf("observation %d passing correctness requires current passing verification evidence", index)
	}
	if metrics.OutcomeSuccess == OutcomeAssessmentPass && (metrics.Correctness != OutcomeAssessmentPass || metrics.VerificationQuality != OutcomeVerificationPass || metrics.Evidence != EvidenceCurrent) {
		return fmt.Errorf("observation %d passing outcome requires current passing verification evidence", index)
	}
	return nil
}

func outcomeQualityObservationRank(observation OutcomeQualityObservation, scenarioIndex map[string]int, repetitions int) (int, error) {
	index, ok := scenarioIndex[observation.ScenarioID]
	if !ok {
		return 0, fmt.Errorf("unknown scenario_id %q", observation.ScenarioID)
	}
	if !observation.Variant.valid() {
		return 0, fmt.Errorf("unknown variant %q", observation.Variant)
	}
	if observation.Repetition < 1 || observation.Repetition > repetitions {
		return 0, fmt.Errorf("repetition=%d outside 1..%d", observation.Repetition, repetitions)
	}
	variantIndex := 0
	if observation.Variant == OutcomeVariantCandidate {
		variantIndex = 1
	}
	return (index*2+variantIndex)*repetitions + observation.Repetition - 1, nil
}

func validSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
