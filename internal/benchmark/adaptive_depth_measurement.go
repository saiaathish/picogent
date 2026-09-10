package benchmark

import (
	"context"
	"fmt"

	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/taskstate"
)

// AdaptiveDepthMeasurementSchema identifies the benchmark-only adaptive-depth
// measurement format. It is not a task-state, completion, or release schema.
const AdaptiveDepthMeasurementSchema = "picogent.v4.adaptive-depth-measurement.v1"

const (
	AdaptiveDepthMeasurementScenarioSet     = "v4-adaptive-depth-catalog"
	MaxAdaptiveDepthMeasurementScenarios    = 32
	MaxAdaptiveDepthMeasurementWorkUnits    = 32
	MaxAdaptiveDepthMeasurementQualityLoops = 3
)

const adaptiveDepthMeasurementUnverifiedReason = "outcome quality impact is not isolated because adaptive depth remains advisory"

// AdaptiveDepthMeasurementReport is a bounded experiment report. Budget
// adequacy is a routing proxy; QualityImpact remains inconclusive until the
// same executor can run genuinely isolated fixed and adaptive routes.
type AdaptiveDepthMeasurementReport struct {
	Schema        string                                `json:"schema"`
	ScenarioSet   string                                `json:"scenario_set"`
	Status        OutcomeQualityReportStatus            `json:"status"`
	FixedBudget   outcome.QualityBudget                 `json:"fixed_budget"`
	MaxWorkUnits  int                                   `json:"max_work_units"`
	Observations  []AdaptiveDepthMeasurementObservation `json:"observations"`
	QualityImpact OutcomeQualityAssessment              `json:"quality_impact"`
	Unverified    []string                              `json:"unverified,omitempty"`
}

// AdaptiveDepthMeasurementObservation records only stable catalog metadata,
// the versioned profile, and bounded proxy metrics. It deliberately excludes
// task prompts and repository text.
type AdaptiveDepthMeasurementObservation struct {
	ScenarioID             string                         `json:"scenario_id"`
	Category               OutcomeQualityScenarioCategory `json:"category"`
	Kind                   OutcomeQualityScenarioKind     `json:"kind"`
	Profile                outcome.TaskDepthProfile       `json:"adaptive_profile"`
	RequiredQualityLoops   int                            `json:"required_quality_loops"`
	FixedWorkUnits         int                            `json:"fixed_work_units"`
	AdaptiveWorkUnits      int                            `json:"adaptive_work_units"`
	FixedBudgetAdequacy    OutcomeQualityAssessment       `json:"fixed_budget_adequacy"`
	AdaptiveBudgetAdequacy OutcomeQualityAssessment       `json:"adaptive_budget_adequacy"`
}

// MeasureAdaptiveDepthCatalog runs the provider-independent, deterministic
// routing experiment over the existing bounded outcome-quality catalog. It
// does not invoke a model, tool, planner, specialist, or workspace mutation.
func MeasureAdaptiveDepthCatalog(ctx context.Context) (AdaptiveDepthMeasurementReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	report := AdaptiveDepthMeasurementReport{
		Schema:        AdaptiveDepthMeasurementSchema,
		ScenarioSet:   AdaptiveDepthMeasurementScenarioSet,
		Status:        OutcomeReportInconclusive,
		FixedBudget:   adaptiveDepthFixedBudget(),
		MaxWorkUnits:  MaxAdaptiveDepthMeasurementWorkUnits,
		QualityImpact: OutcomeAssessmentInconclusive,
		Unverified:    []string{adaptiveDepthMeasurementUnverifiedReason},
	}
	scenarios := DefaultOutcomeQualityScenarios()
	if len(scenarios) > MaxAdaptiveDepthMeasurementScenarios {
		return report, fmt.Errorf("adaptive-depth scenarios=%d exceeds %d", len(scenarios), MaxAdaptiveDepthMeasurementScenarios)
	}
	report.Observations = make([]AdaptiveDepthMeasurementObservation, 0, len(scenarios))
	fixedWorkUnits := adaptiveDepthBudgetWorkUnits(report.FixedBudget)
	for _, scenario := range scenarios {
		if err := ctx.Err(); err != nil {
			return AdaptiveDepthMeasurementReport{}, fmt.Errorf("adaptive-depth measurement canceled: %w", err)
		}
		task := adaptiveDepthMeasurementTask(scenario)
		contract := outcome.Build(task, projecthealth.Report{Schema: projecthealth.Schema})
		requiredLoops := adaptiveDepthRequiredQualityLoops(scenario)
		adaptiveWorkUnits := adaptiveDepthBudgetWorkUnits(contract.Depth.Budget)
		if adaptiveWorkUnits > report.MaxWorkUnits {
			return AdaptiveDepthMeasurementReport{}, fmt.Errorf("scenario %q adaptive work=%d exceeds %d", scenario.ID, adaptiveWorkUnits, report.MaxWorkUnits)
		}
		report.Observations = append(report.Observations, AdaptiveDepthMeasurementObservation{
			ScenarioID:             scenario.ID,
			Category:               scenario.Category,
			Kind:                   scenario.Kind,
			Profile:                contract.Depth,
			RequiredQualityLoops:   requiredLoops,
			FixedWorkUnits:         fixedWorkUnits,
			AdaptiveWorkUnits:      adaptiveWorkUnits,
			FixedBudgetAdequacy:    adaptiveDepthBudgetAdequacy(report.FixedBudget, requiredLoops),
			AdaptiveBudgetAdequacy: adaptiveDepthBudgetAdequacy(contract.Depth.Budget, requiredLoops),
		})
	}
	return report, nil
}

func adaptiveDepthFixedBudget() outcome.QualityBudget {
	return outcome.QualityBudget{
		Research:        outcome.BudgetStandard,
		Review:          outcome.BudgetStandard,
		Experimentation: outcome.BudgetStandard,
		Verification:    outcome.BudgetStandard,
		Specialists:     outcome.BudgetStandard,
		Reasoning:       outcome.BudgetStandard,
		QualityLoops:    1,
	}
}

func adaptiveDepthMeasurementTask(scenario OutcomeQualityScenario) *taskstate.Task {
	task := &taskstate.Task{
		Goal: "adaptive-depth-measurement:" + scenario.ID,
		Intent: &taskstate.IntentContract{
			Outcome:      "adaptive-depth-measurement:" + scenario.ID,
			Class:        "general",
			Action:       "implementation",
			Completeness: "targeted",
			Risk:         "low",
			Confidence:   "high",
		},
	}

	switch scenario.Category {
	case OutcomeCategoryBeginner:
		switch scenario.Kind {
		case OutcomeKindVagueFeature:
			task.Intent.Completeness = "unknown"
			task.Intent.Confidence = "low"
		case OutcomeKindBrokenApp:
			task.Intent.Class = "bug"
			task.Intent.Action = "repair"
			task.Attempts = 2
		case OutcomeKindSetupProblem:
			task.Intent.Class = "setup"
			task.ChangedFiles = []string{"config/setup.toml"}
		}
	case OutcomeCategoryStandardDevelopment:
		switch scenario.Kind {
		case OutcomeKindBug:
			task.Intent.Class = "bug"
			task.Intent.Action = "fix"
			task.ChangedFiles = []string{"internal/bug.go"}
		case OutcomeKindFeature:
			task.ChangedFiles = []string{"internal/feature.go"}
		case OutcomeKindRefactor:
			task.Intent.Class = "refactor"
			task.Intent.Action = "refactor"
			task.ChangedFiles = []string{"internal/refactor.go"}
		case OutcomeKindTests:
			task.Intent.Class = "tests"
			task.Intent.NeedsTests = true
			task.ChangedFiles = []string{"internal/feature_test.go"}
		}
	case OutcomeCategoryAdvanced:
		task.ChangedFiles = []string{"internal/core/one.go", "internal/core/two.go", "cmd/picogent/main.go", "docs/architecture.md"}
		switch scenario.Kind {
		case OutcomeKindMigration:
			task.Intent.Class = "migration"
			task.Intent.Action = "migrate"
		case OutcomeKindArchitecture:
			task.Intent.Class = "refactor"
			task.Intent.Action = "architecture"
		case OutcomeKindPerformance:
			task.Intent.Class = "performance"
			task.Intent.NeedsMeasurement = true
		case OutcomeKindSecurity:
			task.Intent.Class = "security"
			task.Intent.Risk = "high"
			task.Intent.NeedsApproval = true
		}
	case OutcomeCategoryProduct:
		switch scenario.Kind {
		case OutcomeKindUIPolish:
			task.Intent.Class = "ui"
			task.Intent.Action = "visual"
			task.Intent.NeedsVisual = true
			task.ChangedFiles = []string{"web/styles.css"}
		case OutcomeKindOnboarding:
			task.Intent.Class = "ui"
			task.Intent.NeedsVisual = true
			task.ChangedFiles = []string{"web/onboarding.css", "web/onboarding.tsx"}
		case OutcomeKindLaunchReadiness:
			task.Intent.Class = "release"
			task.Intent.Action = "launch"
			task.Intent.Completeness = "full"
			task.ChangedFiles = []string{"internal/app.go", "web/app.tsx", "config/release.toml", "docs/release.md"}
		}
	case OutcomeCategoryRobustness:
		task.Attempts = 2
		task.Uncertainty = []string{"measurement boundary"}
		task.ChangedFiles = []string{"internal/runtime/recovery.go"}
		switch scenario.Kind {
		case OutcomeKindConflictingEdits:
			task.ChangedFiles = []string{"internal/runtime/one.go", "internal/runtime/two.go", "internal/gui/state.go", "internal/tui/state.go"}
		case OutcomeKindUndo:
			task.Intent.Action = "undo"
		}
	case OutcomeCategoryLongHorizon:
		task.Intent.Class = "review"
		task.Intent.Action = "investigation"
		task.Intent.Completeness = "full"
		task.ChangedFiles = []string{"internal/one.go", "internal/two.go", "internal/three.go", "web/app.tsx", "docs/plan.md"}
	}
	return task
}

func adaptiveDepthRequiredQualityLoops(scenario OutcomeQualityScenario) int {
	switch scenario.Category {
	case OutcomeCategoryAdvanced, OutcomeCategoryProduct, OutcomeCategoryRobustness:
		return 2
	case OutcomeCategoryLongHorizon:
		return 3
	default:
		return 1
	}
}

func adaptiveDepthBudgetAdequacy(budget outcome.QualityBudget, requiredLoops int) OutcomeQualityAssessment {
	if budget.QualityLoops >= requiredLoops {
		return OutcomeAssessmentPass
	}
	return OutcomeAssessmentFail
}

func adaptiveDepthBudgetWorkUnits(budget outcome.QualityBudget) int {
	return adaptiveDepthBudgetLevelWorkUnits(budget.Research) +
		adaptiveDepthBudgetLevelWorkUnits(budget.Review) +
		adaptiveDepthBudgetLevelWorkUnits(budget.Experimentation) +
		adaptiveDepthBudgetLevelWorkUnits(budget.Verification) +
		adaptiveDepthBudgetLevelWorkUnits(budget.Specialists) +
		adaptiveDepthBudgetLevelWorkUnits(budget.Reasoning) +
		budget.QualityLoops
}

func adaptiveDepthBudgetLevelWorkUnits(level outcome.BudgetLevel) int {
	switch level {
	case outcome.BudgetNone:
		return 0
	case outcome.BudgetMinimal:
		return 1
	case outcome.BudgetStandard:
		return 2
	case outcome.BudgetElevated:
		return 3
	case outcome.BudgetBroad:
		return 4
	default:
		return MaxAdaptiveDepthMeasurementWorkUnits
	}
}

// Validate enforces the stable catalog, bounded profiles, deterministic proxy
// calculations, and the explicit inconclusive quality boundary.
func (r AdaptiveDepthMeasurementReport) Validate() error {
	if r.Schema != AdaptiveDepthMeasurementSchema {
		return fmt.Errorf("schema=%q, want %q", r.Schema, AdaptiveDepthMeasurementSchema)
	}
	if r.ScenarioSet != AdaptiveDepthMeasurementScenarioSet {
		return fmt.Errorf("scenario_set=%q, want %q", r.ScenarioSet, AdaptiveDepthMeasurementScenarioSet)
	}
	if r.Status != OutcomeReportInconclusive {
		return fmt.Errorf("status=%q, want inconclusive until isolated quality execution", r.Status)
	}
	if r.QualityImpact != OutcomeAssessmentInconclusive && r.QualityImpact != OutcomeAssessmentUnverified {
		return fmt.Errorf("quality_impact=%q must remain inconclusive or unverified", r.QualityImpact)
	}
	if len(r.Unverified) == 0 {
		return fmt.Errorf("measurement must record why quality impact is unverified")
	}
	if err := validateTextList("unverified", r.Unverified, MaxOutcomeQualityUnverified); err != nil {
		return err
	}
	if r.FixedBudget != adaptiveDepthFixedBudget() {
		return fmt.Errorf("fixed budget is not the stable standard comparator: %#v", r.FixedBudget)
	}
	if r.MaxWorkUnits <= 0 || r.MaxWorkUnits > MaxAdaptiveDepthMeasurementWorkUnits {
		return fmt.Errorf("max_work_units=%d outside 1..%d", r.MaxWorkUnits, MaxAdaptiveDepthMeasurementWorkUnits)
	}
	scenarios := DefaultOutcomeQualityScenarios()
	if len(scenarios) > MaxAdaptiveDepthMeasurementScenarios || len(r.Observations) != len(scenarios) {
		return fmt.Errorf("observations=%d, want exactly %d", len(r.Observations), len(scenarios))
	}
	fixedWorkUnits := adaptiveDepthBudgetWorkUnits(r.FixedBudget)
	for index, observation := range r.Observations {
		scenario := scenarios[index]
		if observation.ScenarioID != scenario.ID || observation.Category != scenario.Category || observation.Kind != scenario.Kind {
			return fmt.Errorf("observation %d does not match the stable catalog: got=%#v want=%#v", index, observation, scenario)
		}
		if err := validateAdaptiveDepthMeasurementProfile(observation.Profile); err != nil {
			return fmt.Errorf("observation %d profile: %w", index, err)
		}
		requiredLoops := adaptiveDepthRequiredQualityLoops(scenario)
		if observation.RequiredQualityLoops != requiredLoops {
			return fmt.Errorf("observation %d required_quality_loops=%d, want %d", index, observation.RequiredQualityLoops, requiredLoops)
		}
		adaptiveWorkUnits := adaptiveDepthBudgetWorkUnits(observation.Profile.Budget)
		if observation.FixedWorkUnits != fixedWorkUnits || observation.AdaptiveWorkUnits != adaptiveWorkUnits {
			return fmt.Errorf("observation %d work units=%d/%d, want %d/%d", index, observation.FixedWorkUnits, observation.AdaptiveWorkUnits, fixedWorkUnits, adaptiveWorkUnits)
		}
		if observation.AdaptiveWorkUnits > r.MaxWorkUnits {
			return fmt.Errorf("observation %d adaptive work=%d exceeds %d", index, observation.AdaptiveWorkUnits, r.MaxWorkUnits)
		}
		if want := adaptiveDepthBudgetAdequacy(r.FixedBudget, requiredLoops); observation.FixedBudgetAdequacy != want {
			return fmt.Errorf("observation %d fixed budget adequacy=%q, want %q", index, observation.FixedBudgetAdequacy, want)
		}
		if want := adaptiveDepthBudgetAdequacy(observation.Profile.Budget, requiredLoops); observation.AdaptiveBudgetAdequacy != want {
			return fmt.Errorf("observation %d adaptive budget adequacy=%q, want %q", index, observation.AdaptiveBudgetAdequacy, want)
		}
	}
	return nil
}

func validateAdaptiveDepthMeasurementProfile(profile outcome.TaskDepthProfile) error {
	if profile.Schema != outcome.TaskDepthSchema {
		return fmt.Errorf("schema=%q, want %q", profile.Schema, outcome.TaskDepthSchema)
	}
	switch profile.Class {
	case outcome.TaskDepthNone, outcome.TaskDepthMinimal, outcome.TaskDepthFocused, outcome.TaskDepthDeep, outcome.TaskDepthBroad:
	default:
		return fmt.Errorf("unknown class %q", profile.Class)
	}
	switch profile.Confidence {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("unknown confidence %q", profile.Confidence)
	}
	if len(profile.Signals) > 8 {
		return fmt.Errorf("signals=%d exceeds 8", len(profile.Signals))
	}
	seenSignals := make(map[outcome.TaskDepthSignal]struct{}, len(profile.Signals))
	for _, signal := range profile.Signals {
		if _, seen := seenSignals[signal]; seen {
			return fmt.Errorf("signal %q is repeated", signal)
		}
		seenSignals[signal] = struct{}{}
		switch signal {
		case outcome.TaskDepthSignalBlastRadius, outcome.TaskDepthSignalAmbiguity,
			outcome.TaskDepthSignalSecurity, outcome.TaskDepthSignalArchitecture,
			outcome.TaskDepthSignalVerificationCost, outcome.TaskDepthSignalUncertainty,
			outcome.TaskDepthSignalHistoricalFragility, outcome.TaskDepthSignalUserIntent:
		default:
			return fmt.Errorf("unknown signal %q", signal)
		}
	}
	if err := validateAdaptiveDepthMeasurementBudget(profile.Budget); err != nil {
		return err
	}
	if profile.Class == outcome.TaskDepthNone && profile.Budget != (outcome.QualityBudget{
		Research: outcome.BudgetNone, Review: outcome.BudgetNone, Experimentation: outcome.BudgetNone,
		Verification: outcome.BudgetNone, Specialists: outcome.BudgetNone, Reasoning: outcome.BudgetNone,
	}) {
		return fmt.Errorf("NONE profile must have an empty quality budget")
	}
	return nil
}

func validateAdaptiveDepthMeasurementBudget(budget outcome.QualityBudget) error {
	for name, value := range map[string]outcome.BudgetLevel{
		"research": budget.Research, "review": budget.Review, "experimentation": budget.Experimentation,
		"verification": budget.Verification, "specialists": budget.Specialists, "reasoning": budget.Reasoning,
	} {
		switch value {
		case outcome.BudgetNone, outcome.BudgetMinimal, outcome.BudgetStandard, outcome.BudgetElevated, outcome.BudgetBroad:
		default:
			return fmt.Errorf("unknown %s budget %q", name, value)
		}
	}
	if budget.QualityLoops < 0 || budget.QualityLoops > MaxAdaptiveDepthMeasurementQualityLoops {
		return fmt.Errorf("quality_loops=%d outside 0..%d", budget.QualityLoops, MaxAdaptiveDepthMeasurementQualityLoops)
	}
	return nil
}
