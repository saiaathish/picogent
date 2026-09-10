package benchmark

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/projecthealth"
)

// AdaptiveDepthQualityMeasurementSchema identifies the benchmark-only
// fixed-versus-adaptive execution report. It is not production routing or
// release evidence.
const AdaptiveDepthQualityMeasurementSchema = "picogent.v4.adaptive-depth-quality.v1"

const (
	AdaptiveDepthQualityMeasurementScenarioSet = "v4-adaptive-depth-catalog"
	AdaptiveDepthQualityExecutor               = "scripted-outcome-quality-v2"
	MaxAdaptiveDepthQualityLoops               = 3
	MaxAdaptiveDepthQualityRepetitions         = 2
	MaxAdaptiveDepthQualityObservations        = 64
	adaptiveDepthQualityBaseToolRounds         = 3
)

const adaptiveDepthQualityUnverifiedReason = "deterministic scripted quality evidence does not generalize to live providers or arbitrary repositories"

// AdaptiveDepthQualityReport contains paired executions with identical
// fixture inputs. Only the bounded route turn limit differs between fixed and
// adaptive executions.
type AdaptiveDepthQualityReport struct {
	Schema        string                            `json:"schema"`
	ScenarioSet   string                            `json:"scenario_set"`
	Executor      string                            `json:"executor"`
	Target        OutcomeQualityTarget              `json:"target"`
	Status        OutcomeQualityReportStatus        `json:"status"`
	Policy        OutcomeQualityPolicy              `json:"policy"`
	Observations  []AdaptiveDepthQualityObservation `json:"observations"`
	QualityImpact OutcomeQualityAssessment          `json:"quality_impact"`
	Unverified    []string                          `json:"unverified,omitempty"`
}

// AdaptiveDepthQualityObservation pairs fixed and adaptive route metrics for
// one catalog scenario. The input digest proves that both runs received the
// same bounded fixture without copying its prompt into the report.
type AdaptiveDepthQualityObservation struct {
	ScenarioID              string                         `json:"scenario_id"`
	Category                OutcomeQualityScenarioCategory `json:"category"`
	Kind                    OutcomeQualityScenarioKind     `json:"kind"`
	Repetition              int                            `json:"repetition"`
	InputSHA256             string                         `json:"input_sha256"`
	Profile                 outcome.TaskDepthProfile       `json:"adaptive_profile"`
	RequiredQualityLoops    int                            `json:"required_quality_loops"`
	FixedMaxTurns           int                            `json:"fixed_max_turns"`
	AdaptiveMaxTurns        int                            `json:"adaptive_max_turns"`
	FixedBudgetExhausted    bool                           `json:"fixed_budget_exhausted"`
	AdaptiveBudgetExhausted bool                           `json:"adaptive_budget_exhausted"`
	Fixed                   OutcomeQualityMetrics          `json:"fixed"`
	Adaptive                OutcomeQualityMetrics          `json:"adaptive"`
	FixedUnverified         []string                       `json:"fixed_unverified,omitempty"`
	AdaptiveUnverified      []string                       `json:"adaptive_unverified,omitempty"`
	QualityDelta            OutcomeQualityAssessment       `json:"quality_delta"`
}

// MeasureAdaptiveDepthQualityCatalog runs the same deterministic scripted
// executor for paired fixed/adaptive routes and repeated observations per
// catalog case. It changes only the bounded MaxTurns route budget; no
// production agent configuration is changed.
func MeasureAdaptiveDepthQualityCatalog(ctx context.Context) (AdaptiveDepthQualityReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	report := AdaptiveDepthQualityReport{
		Schema:        AdaptiveDepthQualityMeasurementSchema,
		ScenarioSet:   AdaptiveDepthQualityMeasurementScenarioSet,
		Executor:      AdaptiveDepthQualityExecutor,
		Target:        adaptiveDepthQualityTarget(),
		Status:        OutcomeReportInconclusive,
		Policy:        adaptiveDepthQualityPolicy(),
		QualityImpact: OutcomeAssessmentInconclusive,
		Unverified:    []string{adaptiveDepthQualityUnverifiedReason},
	}
	scenarios := DefaultOutcomeQualityScenarios()
	observationCount := len(scenarios) * report.Policy.Repetitions
	if len(scenarios) > MaxAdaptiveDepthQualityObservations || observationCount > MaxAdaptiveDepthQualityObservations {
		return report, fmt.Errorf("adaptive-depth quality observations=%d exceeds %d", observationCount, MaxAdaptiveDepthQualityObservations)
	}
	report.Observations = make([]AdaptiveDepthQualityObservation, 0, observationCount)
	executor := NewOutcomeQualityAgentExecutor()
	hadUnexpectedExecution := false
	for _, scenario := range scenarios {
		if err := ctx.Err(); err != nil {
			return AdaptiveDepthQualityReport{}, fmt.Errorf("adaptive-depth quality measurement canceled: %w", err)
		}
		input := DefaultOutcomeQualityInput(scenario)
		digest := outcomeQualityInputDigest(input)
		scenario.InputSHA256 = digest
		contract := outcome.Build(adaptiveDepthMeasurementTask(scenario), projecthealth.Report{Schema: projecthealth.Schema})
		requiredLoops := adaptiveDepthRequiredQualityLoops(scenario)
		fixedPolicy := report.Policy
		fixedPolicy.MaxTurns = adaptiveDepthQualityMaxTurns(1)
		adaptiveLoops := maxAdaptiveDepthQualityLoops(contract.Depth)
		adaptivePolicy := report.Policy
		adaptivePolicy.MaxTurns = adaptiveDepthQualityMaxTurns(adaptiveLoops)
		for repetition := 1; repetition <= report.Policy.Repetitions; repetition++ {
			fixedMetrics, fixedExhausted, fixedUnverified, err := runAdaptiveDepthQualityExecution(ctx, executor, adaptiveDepthQualityRequest(scenario, input, digest, fixedPolicy, repetition, requiredLoops))
			if err != nil {
				return AdaptiveDepthQualityReport{}, err
			}
			adaptiveMetrics, adaptiveExhausted, adaptiveUnverified, err := runAdaptiveDepthQualityExecution(ctx, executor, adaptiveDepthQualityRequest(scenario, input, digest, adaptivePolicy, repetition, requiredLoops))
			if err != nil {
				return AdaptiveDepthQualityReport{}, err
			}
			if len(fixedUnverified) > 0 || len(adaptiveUnverified) > 0 {
				hadUnexpectedExecution = true
			}
			report.Observations = append(report.Observations, AdaptiveDepthQualityObservation{
				ScenarioID:              scenario.ID,
				Category:                scenario.Category,
				Kind:                    scenario.Kind,
				Repetition:              repetition,
				InputSHA256:             digest,
				Profile:                 contract.Depth,
				RequiredQualityLoops:    requiredLoops,
				FixedMaxTurns:           fixedPolicy.MaxTurns,
				AdaptiveMaxTurns:        adaptivePolicy.MaxTurns,
				FixedBudgetExhausted:    fixedExhausted,
				AdaptiveBudgetExhausted: adaptiveExhausted,
				Fixed:                   fixedMetrics,
				Adaptive:                adaptiveMetrics,
				FixedUnverified:         fixedUnverified,
				AdaptiveUnverified:      adaptiveUnverified,
				QualityDelta:            adaptiveDepthQualityDelta(fixedMetrics, adaptiveMetrics, fixedUnverified, adaptiveUnverified),
			})
		}
	}
	if !hadUnexpectedExecution {
		report.Status = OutcomeReportComplete
		report.QualityImpact = adaptiveDepthAggregateQualityImpact(report.Observations)
	}
	if err := report.Validate(); err != nil {
		return AdaptiveDepthQualityReport{}, fmt.Errorf("adaptive-depth quality report: %w", err)
	}
	return report, nil
}

func adaptiveDepthQualityPolicy() OutcomeQualityPolicy {
	return OutcomeQualityPolicy{
		Repetitions:   MaxAdaptiveDepthQualityRepetitions,
		TimeoutMillis: 30_000,
		MaxTokens:     16_000,
		MaxModelCalls: 64,
		MaxToolCalls:  256,
		MaxTurns:      adaptiveDepthQualityMaxTurns(MaxAdaptiveDepthQualityLoops),
	}
}

func adaptiveDepthQualityTarget() OutcomeQualityTarget {
	return OutcomeQualityTarget{
		// The direct scripted executor requires a SHA-shaped target token even
		// though this same-process experiment has no source-head comparison.
		SourceHead:  strings.Repeat("0", 40),
		Host:        "local/scripted",
		GoVersion:   "local",
		ToolVersion: OutcomeQualityRunnerToolVersion,
	}
}

func adaptiveDepthQualityRequest(scenario OutcomeQualityScenario, input OutcomeQualityInput, digest string, policy OutcomeQualityPolicy, repetition, qualityLoops int) OutcomeQualityExecutionRequest {
	return OutcomeQualityExecutionRequest{
		Scenario:     scenario,
		Variant:      OutcomeVariantBaseline,
		Repetition:   repetition,
		InputSHA256:  digest,
		Input:        input,
		Target:       adaptiveDepthQualityTarget(),
		Policy:       policy,
		QualityLoops: qualityLoops,
	}
}

func adaptiveDepthQualityMaxTurns(qualityLoops int) int {
	if qualityLoops < 1 {
		qualityLoops = 1
	}
	if qualityLoops > MaxAdaptiveDepthQualityLoops {
		qualityLoops = MaxAdaptiveDepthQualityLoops
	}
	return adaptiveDepthQualityBaseToolRounds + qualityLoops
}

func maxAdaptiveDepthQualityLoops(profile outcome.TaskDepthProfile) int {
	loops := profile.Budget.QualityLoops
	if loops < 1 {
		return 1
	}
	if loops > MaxAdaptiveDepthQualityLoops {
		return MaxAdaptiveDepthQualityLoops
	}
	return loops
}

func runAdaptiveDepthQualityExecution(ctx context.Context, executor *OutcomeQualityAgentExecutor, request OutcomeQualityExecutionRequest) (OutcomeQualityMetrics, bool, []string, error) {
	runCtx := ctx
	cancel := func() {}
	if request.Policy.TimeoutMillis > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(request.Policy.TimeoutMillis)*time.Millisecond)
	}
	execution, executionErr := executor.Execute(runCtx, request)
	runErr := runCtx.Err()
	cancel()
	if err := ctx.Err(); err != nil {
		return OutcomeQualityMetrics{}, false, nil, fmt.Errorf("adaptive-depth quality execution canceled: %w", err)
	}
	if executionErr == nil && runErr != nil {
		executionErr = runErr
	}
	if isAdaptiveDepthQualityBudgetExhaustion(executionErr) {
		metrics := execution.Metrics
		metrics.OutcomeSuccess = OutcomeAssessmentFail
		metrics.Correctness = OutcomeAssessmentFail
		metrics.VerificationQuality = OutcomeVerificationInconclusive
		metrics.Evidence = EvidenceUnverified
		return metrics, true, nil, nil
	}
	reasons := boundedOutcomeQualityReasons(execution.Unverified, 8)
	if executionErr != nil {
		reasons = appendOutcomeQualityReason(reasons, "executor error: "+executionErr.Error(), 8)
	}
	metrics := execution.Metrics
	if !metrics.OutcomeSuccess.valid() || len(reasons) > 0 {
		metrics = inconclusiveOutcomeQualityMetrics(metrics.LatencyMillis)
	} else if err := validateOutcomeQualityMetrics(metrics, request.Policy, 0); err != nil {
		reasons = appendOutcomeQualityReason(reasons, "executor metrics rejected: "+err.Error(), 8)
		metrics = inconclusiveOutcomeQualityMetrics(metrics.LatencyMillis)
	}
	if len(reasons) == 0 {
		reasons = nil
	}
	return metrics, false, reasons, nil
}

func isAdaptiveDepthQualityBudgetExhaustion(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "stopped after") && strings.Contains(value, "tool rounds") && strings.Contains(value, "limit")
}

func adaptiveDepthQualityDelta(fixed, adaptive OutcomeQualityMetrics, fixedUnverified, adaptiveUnverified []string) OutcomeQualityAssessment {
	if len(fixedUnverified) > 0 || len(adaptiveUnverified) > 0 {
		return OutcomeAssessmentInconclusive
	}
	if adaptive.OutcomeSuccess == OutcomeAssessmentPass && fixed.OutcomeSuccess != OutcomeAssessmentPass {
		return OutcomeAssessmentPass
	}
	if fixed.OutcomeSuccess == OutcomeAssessmentPass && adaptive.OutcomeSuccess != OutcomeAssessmentPass {
		return OutcomeAssessmentFail
	}
	return OutcomeAssessmentInconclusive
}

func adaptiveDepthAggregateQualityImpact(observations []AdaptiveDepthQualityObservation) OutcomeQualityAssessment {
	improvement := false
	for _, observation := range observations {
		switch observation.QualityDelta {
		case OutcomeAssessmentFail:
			return OutcomeAssessmentFail
		case OutcomeAssessmentPass:
			improvement = true
		}
	}
	if improvement {
		return OutcomeAssessmentPass
	}
	return OutcomeAssessmentInconclusive
}

// Validate enforces paired inputs, stable catalog ordering, metric bounds, and
// the distinction between deterministic fixture evidence and general quality.
func (r AdaptiveDepthQualityReport) Validate() error {
	if r.Schema != AdaptiveDepthQualityMeasurementSchema {
		return fmt.Errorf("schema=%q, want %q", r.Schema, AdaptiveDepthQualityMeasurementSchema)
	}
	if r.ScenarioSet != AdaptiveDepthQualityMeasurementScenarioSet {
		return fmt.Errorf("scenario_set=%q, want %q", r.ScenarioSet, AdaptiveDepthQualityMeasurementScenarioSet)
	}
	if r.Executor != AdaptiveDepthQualityExecutor {
		return fmt.Errorf("executor=%q, want %q", r.Executor, AdaptiveDepthQualityExecutor)
	}
	if r.Target != adaptiveDepthQualityTarget() {
		return fmt.Errorf("target provenance does not match the scripted executor")
	}
	if !r.Status.valid() {
		return fmt.Errorf("unknown status %q", r.Status)
	}
	if r.QualityImpact != OutcomeAssessmentPass && r.QualityImpact != OutcomeAssessmentFail && r.QualityImpact != OutcomeAssessmentInconclusive && r.QualityImpact != OutcomeAssessmentUnverified {
		return fmt.Errorf("unknown quality_impact %q", r.QualityImpact)
	}
	if len(r.Unverified) == 0 {
		return fmt.Errorf("quality report must record its generalization boundary")
	}
	if r.Status != OutcomeReportComplete && r.QualityImpact == OutcomeAssessmentPass {
		return fmt.Errorf("non-complete quality report cannot claim quality impact pass")
	}
	if err := validateTextList("unverified", r.Unverified, MaxOutcomeQualityUnverified); err != nil {
		return err
	}
	if err := validateOutcomeQualityPolicy(r.Policy); err != nil {
		return err
	}
	if r.Policy.Repetitions != MaxAdaptiveDepthQualityRepetitions {
		return fmt.Errorf("repetitions=%d, want %d for repeated paired observations", r.Policy.Repetitions, MaxAdaptiveDepthQualityRepetitions)
	}
	scenarios := DefaultOutcomeQualityScenarios()
	wantObservations := len(scenarios) * r.Policy.Repetitions
	if len(scenarios) > MaxAdaptiveDepthQualityObservations || wantObservations > MaxAdaptiveDepthQualityObservations || len(r.Observations) != wantObservations {
		return fmt.Errorf("observations=%d, want exactly %d", len(r.Observations), wantObservations)
	}
	hadUnexpectedExecution := false
	for index, observation := range r.Observations {
		scenarioIndex := index / r.Policy.Repetitions
		repetition := index%r.Policy.Repetitions + 1
		scenario := scenarios[scenarioIndex]
		if observation.ScenarioID != scenario.ID || observation.Category != scenario.Category || observation.Kind != scenario.Kind {
			return fmt.Errorf("observation %d does not match the stable catalog", index)
		}
		if observation.Repetition != repetition {
			return fmt.Errorf("observation %d repetition=%d, want %d", index, observation.Repetition, repetition)
		}
		inputDigest := outcomeQualityInputDigest(DefaultOutcomeQualityInput(scenario))
		if observation.InputSHA256 != inputDigest || !validSHA256(observation.InputSHA256) {
			return fmt.Errorf("observation %d input digest is not the stable fixture digest", index)
		}
		if err := validateAdaptiveDepthMeasurementProfile(observation.Profile); err != nil {
			return fmt.Errorf("observation %d profile: %w", index, err)
		}
		requiredLoops := adaptiveDepthRequiredQualityLoops(scenario)
		if observation.RequiredQualityLoops != requiredLoops || observation.FixedMaxTurns != adaptiveDepthQualityMaxTurns(1) || observation.AdaptiveMaxTurns != adaptiveDepthQualityMaxTurns(maxAdaptiveDepthQualityLoops(observation.Profile)) {
			return fmt.Errorf("observation %d route limits are not derived from the bounded profile", index)
		}
		if observation.FixedMaxTurns > r.Policy.MaxTurns || observation.AdaptiveMaxTurns > r.Policy.MaxTurns {
			return fmt.Errorf("observation %d route limit exceeds shared policy", index)
		}
		if err := validateOutcomeQualityMetrics(observation.Fixed, r.Policy, index); err != nil {
			return fmt.Errorf("observation %d fixed metrics: %w", index, err)
		}
		if err := validateOutcomeQualityMetrics(observation.Adaptive, r.Policy, index); err != nil {
			return fmt.Errorf("observation %d adaptive metrics: %w", index, err)
		}
		if err := validateTextList(fmt.Sprintf("observation %d fixed_unverified", index), observation.FixedUnverified, 8); err != nil {
			return err
		}
		if err := validateTextList(fmt.Sprintf("observation %d adaptive_unverified", index), observation.AdaptiveUnverified, 8); err != nil {
			return err
		}
		if len(observation.FixedUnverified) > 0 || len(observation.AdaptiveUnverified) > 0 {
			hadUnexpectedExecution = true
		}
		if want := adaptiveDepthQualityDelta(observation.Fixed, observation.Adaptive, observation.FixedUnverified, observation.AdaptiveUnverified); observation.QualityDelta != want {
			return fmt.Errorf("observation %d quality_delta=%q, want %q", index, observation.QualityDelta, want)
		}
	}
	if r.Status == OutcomeReportComplete && hadUnexpectedExecution {
		return fmt.Errorf("complete quality report contains unexpected execution failures")
	}
	if r.Status == OutcomeReportComplete && r.QualityImpact != adaptiveDepthAggregateQualityImpact(r.Observations) {
		return fmt.Errorf("complete quality impact is not derived from observations")
	}
	return nil
}
