package outcome

import (
	"strings"

	"github.com/saiaathish/picogent/internal/taskstate"
)

const (
	TaskDepthSchema     = "picogent.outcome-depth.v1"
	maxTaskDepthSignals = 8
	maxQualityLoops     = 3
)

// TaskDepthClass is a bounded amount of reasoning and verification depth.
// It is an advisory class: it never authorizes a tool, mutation, permission
// change, completion claim, or release.
type TaskDepthClass string

const (
	TaskDepthNone    TaskDepthClass = "NONE"
	TaskDepthMinimal TaskDepthClass = "MINIMAL"
	TaskDepthFocused TaskDepthClass = "FOCUSED"
	TaskDepthDeep    TaskDepthClass = "DEEP"
	TaskDepthBroad   TaskDepthClass = "BROAD"
)

// TaskDepthSignal is a fixed explanation for the selected depth. Free-form
// task text is deliberately not retained here.
type TaskDepthSignal string

const (
	TaskDepthSignalBlastRadius         TaskDepthSignal = "blast_radius"
	TaskDepthSignalAmbiguity           TaskDepthSignal = "ambiguity"
	TaskDepthSignalSecurity            TaskDepthSignal = "security"
	TaskDepthSignalArchitecture        TaskDepthSignal = "architecture"
	TaskDepthSignalVerificationCost    TaskDepthSignal = "verification_cost"
	TaskDepthSignalUncertainty         TaskDepthSignal = "uncertainty"
	TaskDepthSignalHistoricalFragility TaskDepthSignal = "historical_fragility"
	TaskDepthSignalUserIntent          TaskDepthSignal = "user_intent"
)

// BudgetLevel is intentionally categorical. The levels are bounded guidance,
// not probabilities, token promises, or an unbounded work allowance.
type BudgetLevel string

const (
	BudgetNone     BudgetLevel = "NONE"
	BudgetMinimal  BudgetLevel = "MINIMAL"
	BudgetStandard BudgetLevel = "STANDARD"
	BudgetElevated BudgetLevel = "ELEVATED"
	BudgetBroad    BudgetLevel = "BROAD"
)

// QualityBudget describes finite quality-loop guidance for one task. The
// medium integration lane may map these categories to existing routing and
// verification seams; this contract itself performs no work.
type QualityBudget struct {
	Research        BudgetLevel `json:"research"`
	Review          BudgetLevel `json:"review"`
	Experimentation BudgetLevel `json:"experimentation"`
	Verification    BudgetLevel `json:"verification"`
	Specialists     BudgetLevel `json:"specialists"`
	Reasoning       BudgetLevel `json:"reasoning"`
	QualityLoops    int         `json:"quality_loops"`
}

// TaskDepthProfile is the bounded adaptive-depth contract. It is derived
// from durable task intent plus the existing impact observation only.
type TaskDepthProfile struct {
	Schema     string            `json:"schema"`
	Class      TaskDepthClass    `json:"class"`
	Confidence string            `json:"confidence"`
	Signals    []TaskDepthSignal `json:"signals,omitempty"`
	Budget     QualityBudget     `json:"quality_budget"`
}

var taskDepthSignalOrder = [...]TaskDepthSignal{
	TaskDepthSignalBlastRadius,
	TaskDepthSignalAmbiguity,
	TaskDepthSignalSecurity,
	TaskDepthSignalArchitecture,
	TaskDepthSignalVerificationCost,
	TaskDepthSignalUncertainty,
	TaskDepthSignalHistoricalFragility,
	TaskDepthSignalUserIntent,
}

// PredictTaskDepth derives a deterministic depth profile without inspecting
// the workspace or retaining user-provided text. A missing or uncertain
// boundary is conservative: it increases depth or lowers confidence rather
// than silently reducing verification.
func PredictTaskDepth(task *taskstate.Task, impact ImpactProfile) TaskDepthProfile {
	profile := TaskDepthProfile{
		Schema:     TaskDepthSchema,
		Class:      TaskDepthNone,
		Confidence: "low",
		Budget:     emptyQualityBudget(),
	}
	if task == nil {
		return profile
	}

	impact = boundImpact(impact)
	signals := taskDepthSignals(task, impact)
	profile.Class = classifyTaskDepth(task, impact, signals)
	profile.Confidence = taskDepthConfidence(task, impact)
	profile.Signals = orderedTaskDepthSignals(signals)
	profile.Budget = qualityBudgetFor(profile.Class, task, impact, signals)
	return boundTaskDepthProfile(profile)
}

func taskDepthSignals(task *taskstate.Task, impact ImpactProfile) map[TaskDepthSignal]struct{} {
	signals := make(map[TaskDepthSignal]struct{})
	if impact.Scope == ImpactCrossArea || impact.Scope == ImpactBroad || impact.Scope == ImpactUnknown || impact.ChangedFiles > 3 || impact.ChangedFilesCapped {
		signals[TaskDepthSignalBlastRadius] = struct{}{}
	}

	intent := task.Intent
	if intent == nil || unknownIntentValue(intent.Confidence) || unknownIntentValue(intent.Completeness) {
		signals[TaskDepthSignalAmbiguity] = struct{}{}
	}
	if impact.Scope == ImpactUnknown || impact.Confidence == "low" && impact.Scope != ImpactNone {
		signals[TaskDepthSignalUncertainty] = struct{}{}
	}
	if len(task.Uncertainty) > 0 {
		signals[TaskDepthSignalUncertainty] = struct{}{}
	}
	if impact.Risk == ImpactRiskHigh || intentRisk(intent) == "high" || hasImpactArea(impact, ImpactAreaSecurity) || intentNeedsApproval(intent) || intentClass(intent) == "security" {
		signals[TaskDepthSignalSecurity] = struct{}{}
	}
	if hasImpactArea(impact, ImpactAreaConcurrency) || intentClass(intent) == "refactor" || intentValueIn(intentAction(intent), "architecture", "migrate", "migration", "redesign", "rewrite") {
		signals[TaskDepthSignalArchitecture] = struct{}{}
	}
	if impact.Scope == ImpactCrossArea || impact.Scope == ImpactBroad || impact.Scope == ImpactUnknown || impact.Risk == ImpactRiskHigh || len(impact.Verification) > 1 || len(impact.Review) > 1 || intentNeedsApproval(intent) {
		signals[TaskDepthSignalVerificationCost] = struct{}{}
	}
	if task.Attempts > 1 || task.ConsecutiveVerificationFailures() > 0 {
		signals[TaskDepthSignalHistoricalFragility] = struct{}{}
	}
	if intent != nil && (normalizedIntentCompleteness(intent) == "full" || intentClass(intent) != "general" || intentAction(intent) != "implementation") {
		signals[TaskDepthSignalUserIntent] = struct{}{}
	}
	return signals
}

func classifyTaskDepth(task *taskstate.Task, impact ImpactProfile, signals map[TaskDepthSignal]struct{}) TaskDepthClass {
	if impact.Scope == ImpactBroad || broadIntent(task) {
		return TaskDepthBroad
	}
	if impact.Scope == ImpactUnknown || impact.Scope == ImpactCrossArea || impact.Risk == ImpactRiskHigh || intentRisk(task.Intent) == "high" || normalizedIntentCompleteness(task.Intent) == "full" || hasSignal(signals, TaskDepthSignalSecurity, TaskDepthSignalArchitecture, TaskDepthSignalVerificationCost, TaskDepthSignalUncertainty, TaskDepthSignalHistoricalFragility) {
		return TaskDepthDeep
	}
	if minimalTask(task, impact, signals) {
		return TaskDepthMinimal
	}
	return TaskDepthFocused
}

func minimalTask(task *taskstate.Task, impact ImpactProfile, signals map[TaskDepthSignal]struct{}) bool {
	if task == nil || task.Intent == nil || impact.Risk != ImpactRiskLow || (impact.Scope != ImpactNone && impact.Scope != ImpactFocused) {
		return false
	}
	intent := task.Intent
	if normalizedIntentCompleteness(intent) != "targeted" || intentNeedsResearch(intent) || intentNeedsMeasurement(intent) || intent.NeedsApproval || intent.NeedsTests {
		return false
	}
	if len(task.Uncertainty) > 0 || task.Attempts > 1 || task.ConsecutiveVerificationFailures() > 0 || hasSignal(signals, TaskDepthSignalAmbiguity, TaskDepthSignalBlastRadius, TaskDepthSignalSecurity, TaskDepthSignalArchitecture, TaskDepthSignalUncertainty, TaskDepthSignalHistoricalFragility) {
		return false
	}
	return intentValueIn(intentClass(intent), "general", "documentation", "ui") && intentValueIn(intentAction(intent), "copy", "label", "rename", "text", "format")
}

func broadIntent(task *taskstate.Task) bool {
	if task == nil || task.Intent == nil {
		return false
	}
	intent := task.Intent
	if intentValueIn(intentClass(intent), "release", "launch", "readiness") || intentValueIn(intentAction(intent), "release", "launch", "deploy", "publish", "ship") {
		return true
	}
	return normalizedIntentCompleteness(intent) == "full" && intentValueIn(intentClass(intent), "review", "change")
}

func taskDepthConfidence(task *taskstate.Task, impact ImpactProfile) string {
	if task == nil || task.Intent == nil || impact.Scope == ImpactUnknown || len(task.Uncertainty) > 0 {
		return "low"
	}
	if impact.Confidence == "low" || unknownIntentValue(task.Intent.Confidence) || unknownIntentValue(task.Intent.Completeness) {
		return "medium"
	}
	if impact.Confidence == "medium" {
		return "medium"
	}
	return "high"
}

func qualityBudgetFor(class TaskDepthClass, task *taskstate.Task, impact ImpactProfile, signals map[TaskDepthSignal]struct{}) QualityBudget {
	var budget QualityBudget
	switch class {
	case TaskDepthMinimal:
		budget = QualityBudget{Research: BudgetNone, Review: BudgetMinimal, Experimentation: BudgetNone, Verification: BudgetMinimal, Specialists: BudgetNone, Reasoning: BudgetMinimal, QualityLoops: 0}
	case TaskDepthFocused:
		budget = QualityBudget{Research: BudgetNone, Review: BudgetStandard, Experimentation: BudgetNone, Verification: BudgetStandard, Specialists: BudgetNone, Reasoning: BudgetStandard, QualityLoops: 1}
	case TaskDepthDeep:
		budget = QualityBudget{Research: BudgetStandard, Review: BudgetElevated, Experimentation: BudgetStandard, Verification: BudgetElevated, Specialists: BudgetStandard, Reasoning: BudgetElevated, QualityLoops: 2}
	case TaskDepthBroad:
		budget = QualityBudget{Research: BudgetBroad, Review: BudgetBroad, Experimentation: BudgetElevated, Verification: BudgetBroad, Specialists: BudgetElevated, Reasoning: BudgetBroad, QualityLoops: maxQualityLoops}
	default:
		return emptyQualityBudget()
	}

	intent := task.Intent
	if intentNeedsResearch(intent) {
		budget.Research = maxBudget(budget.Research, BudgetStandard)
	}
	if intentNeedsMeasurement(intent) {
		budget.Experimentation = maxBudget(budget.Experimentation, BudgetStandard)
	}
	if intent != nil && intent.NeedsVisual && !minimalTask(task, impact, signals) {
		budget.Review = maxBudget(budget.Review, BudgetStandard)
		if budget.QualityLoops < 2 {
			budget.QualityLoops = 2
		}
	}
	if intent != nil && intent.NeedsTests {
		budget.Verification = maxBudget(budget.Verification, BudgetStandard)
	}
	if hasSignal(signals, TaskDepthSignalVerificationCost) {
		budget.Verification = maxBudget(budget.Verification, BudgetElevated)
	}
	if intentNeedsApproval(intent) {
		budget.Review = maxBudget(budget.Review, BudgetElevated)
	}
	return boundQualityBudget(budget)
}

func emptyQualityBudget() QualityBudget {
	return QualityBudget{
		Research:        BudgetNone,
		Review:          BudgetNone,
		Experimentation: BudgetNone,
		Verification:    BudgetNone,
		Specialists:     BudgetNone,
		Reasoning:       BudgetNone,
	}
}

func orderedTaskDepthSignals(signals map[TaskDepthSignal]struct{}) []TaskDepthSignal {
	if len(signals) == 0 {
		return nil
	}
	out := make([]TaskDepthSignal, 0, len(signals))
	for _, signal := range taskDepthSignalOrder {
		if _, ok := signals[signal]; ok {
			out = append(out, signal)
		}
		if len(out) >= maxTaskDepthSignals {
			break
		}
	}
	return out
}

func boundTaskDepthProfile(profile TaskDepthProfile) TaskDepthProfile {
	profile.Schema = TaskDepthSchema
	switch profile.Class {
	case TaskDepthNone, TaskDepthMinimal, TaskDepthFocused, TaskDepthDeep, TaskDepthBroad:
	default:
		profile.Class = TaskDepthDeep
	}
	switch profile.Confidence {
	case "low", "medium", "high":
	default:
		profile.Confidence = "low"
	}
	profile.Signals = orderedTaskDepthSignals(signalSet(profile.Signals))
	profile.Budget = boundQualityBudget(profile.Budget)
	if profile.Class == TaskDepthNone {
		profile.Budget = emptyQualityBudget()
	}
	return profile
}

func signalSet(values []TaskDepthSignal) map[TaskDepthSignal]struct{} {
	out := make(map[TaskDepthSignal]struct{}, len(values))
	for _, value := range values {
		for _, known := range taskDepthSignalOrder {
			if value == known {
				out[value] = struct{}{}
				break
			}
		}
	}
	return out
}

func boundQualityBudget(budget QualityBudget) QualityBudget {
	budget.Research = normalizeBudget(budget.Research)
	budget.Review = normalizeBudget(budget.Review)
	budget.Experimentation = normalizeBudget(budget.Experimentation)
	budget.Verification = normalizeBudget(budget.Verification)
	budget.Specialists = normalizeBudget(budget.Specialists)
	budget.Reasoning = normalizeBudget(budget.Reasoning)
	if budget.QualityLoops < 0 {
		budget.QualityLoops = 0
	}
	if budget.QualityLoops > maxQualityLoops {
		budget.QualityLoops = maxQualityLoops
	}
	return budget
}

func normalizeBudget(value BudgetLevel) BudgetLevel {
	switch value {
	case BudgetNone, BudgetMinimal, BudgetStandard, BudgetElevated, BudgetBroad:
		return value
	default:
		return BudgetElevated
	}
}

func maxBudget(left, right BudgetLevel) BudgetLevel {
	if budgetRank(left) >= budgetRank(right) {
		return normalizeBudget(left)
	}
	return normalizeBudget(right)
}

func budgetRank(value BudgetLevel) int {
	switch value {
	case BudgetMinimal:
		return 1
	case BudgetStandard:
		return 2
	case BudgetElevated:
		return 3
	case BudgetBroad:
		return 4
	default:
		return 0
	}
}

func hasSignal(signals map[TaskDepthSignal]struct{}, values ...TaskDepthSignal) bool {
	for _, value := range values {
		if _, ok := signals[value]; ok {
			return true
		}
	}
	return false
}

func hasImpactArea(impact ImpactProfile, area ImpactArea) bool {
	for _, value := range impact.Areas {
		if value == area {
			return true
		}
	}
	return false
}

func intentClass(intent *taskstate.IntentContract) string {
	if intent == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(intent.Class))
}

func intentAction(intent *taskstate.IntentContract) string {
	if intent == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(intent.Action))
}

func normalizedIntentCompleteness(intent *taskstate.IntentContract) string {
	if intent == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(intent.Completeness))
}

func unknownIntentValue(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "" || value == "unknown" || value == "uncertain"
}

func intentValueIn(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func intentNeedsResearch(intent *taskstate.IntentContract) bool {
	return intent != nil && intent.NeedsResearch
}

func intentNeedsMeasurement(intent *taskstate.IntentContract) bool {
	return intent != nil && (intent.NeedsMeasurement || intentClass(intent) == "performance")
}

func intentNeedsApproval(intent *taskstate.IntentContract) bool {
	return intent != nil && intent.NeedsApproval
}

func intentRisk(intent *taskstate.IntentContract) string {
	if intent == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(intent.Risk))
}
