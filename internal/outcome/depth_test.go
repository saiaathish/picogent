package outcome

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestPredictTaskDepthNilIsEmptyAndBounded(t *testing.T) {
	got := PredictTaskDepth(nil, ImpactProfile{})
	if got.Schema != TaskDepthSchema || got.Class != TaskDepthNone || got.Confidence != "low" || len(got.Signals) != 0 {
		t.Fatalf("nil task depth = %#v", got)
	}
	if got.Budget != emptyQualityBudget() {
		t.Fatalf("nil task budget = %#v", got.Budget)
	}
}

func TestPredictTaskDepthKeepsTinyTargetedWorkMinimal(t *testing.T) {
	task := &taskstate.Task{
		Goal: "change this harmless button label; secret text must not escape",
		Intent: &taskstate.IntentContract{
			Class:        "ui",
			Action:       "copy",
			Completeness: "targeted",
			Confidence:   "high",
		},
	}
	impact := ImpactProfile{Scope: ImpactNone, Risk: ImpactRiskLow, Confidence: "high"}

	got := PredictTaskDepth(task, impact)
	if got.Class != TaskDepthMinimal || got.Confidence != "high" {
		t.Fatalf("minimal depth = %#v", got)
	}
	want := QualityBudget{
		Research:        BudgetNone,
		Review:          BudgetMinimal,
		Experimentation: BudgetNone,
		Verification:    BudgetMinimal,
		Specialists:     BudgetNone,
		Reasoning:       BudgetMinimal,
		QualityLoops:    0,
	}
	if got.Budget != want {
		t.Fatalf("minimal budget = %#v, want %#v", got.Budget, want)
	}
	if !containsTaskDepthSignal(got.Signals, TaskDepthSignalUserIntent) {
		t.Fatalf("minimal intent signal missing: %#v", got.Signals)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret text") {
		t.Fatalf("free-form goal leaked into depth profile: %s", encoded)
	}
}

func TestPredictTaskDepthEscalatesSecurityAndArchitecture(t *testing.T) {
	task := &taskstate.Task{
		Intent: &taskstate.IntentContract{
			Class:         "security",
			Action:        "architecture",
			Completeness:  "targeted",
			Risk:          "high",
			NeedsApproval: true,
			Confidence:    "high",
		},
	}
	impact := ImpactProfile{
		Scope:      ImpactFocused,
		Risk:       ImpactRiskHigh,
		Confidence: "high",
		Areas:      []ImpactArea{ImpactAreaSecurity, ImpactAreaConcurrency},
		Verification: []ImpactCheck{
			ImpactCheckSecurity,
			ImpactCheckConcurrency,
		},
		Review: []ImpactCheck{ImpactCheckTargetedReview, ImpactCheckSecurity},
	}

	got := PredictTaskDepth(task, impact)
	if got.Class != TaskDepthDeep {
		t.Fatalf("security depth = %#v", got)
	}
	for _, signal := range []TaskDepthSignal{
		TaskDepthSignalSecurity,
		TaskDepthSignalArchitecture,
		TaskDepthSignalVerificationCost,
	} {
		if !containsTaskDepthSignal(got.Signals, signal) {
			t.Fatalf("security signals = %v, missing %q", got.Signals, signal)
		}
	}
	if got.Budget.Verification != BudgetElevated || got.Budget.Review != BudgetElevated || got.Budget.Specialists != BudgetStandard || got.Budget.Reasoning != BudgetElevated || got.Budget.QualityLoops != 2 {
		t.Fatalf("security budget = %#v", got.Budget)
	}
}

func TestPredictTaskDepthUsesBroadAuditBudgetForFullReview(t *testing.T) {
	task := &taskstate.Task{
		Intent: &taskstate.IntentContract{
			Class:        "review",
			Action:       "investigation",
			Completeness: "full",
			Confidence:   "high",
		},
	}
	impact := ImpactProfile{
		Scope:      ImpactBroad,
		Risk:       ImpactRiskMedium,
		Confidence: "medium",
	}

	got := PredictTaskDepth(task, impact)
	if got.Class != TaskDepthBroad || got.Confidence != "medium" {
		t.Fatalf("broad review depth = %#v", got)
	}
	if got.Budget.Research != BudgetBroad || got.Budget.Review != BudgetBroad || got.Budget.Verification != BudgetBroad || got.Budget.Specialists != BudgetElevated || got.Budget.Reasoning != BudgetBroad || got.Budget.QualityLoops != maxQualityLoops {
		t.Fatalf("broad review budget = %#v", got.Budget)
	}
	if !containsTaskDepthSignal(got.Signals, TaskDepthSignalBlastRadius) || !containsTaskDepthSignal(got.Signals, TaskDepthSignalUserIntent) {
		t.Fatalf("broad review signals = %v", got.Signals)
	}
}

func TestPredictTaskDepthEscalatesUnknownAndFragileState(t *testing.T) {
	unknown := &taskstate.Task{}
	got := PredictTaskDepth(unknown, ImpactProfile{Scope: ImpactUnknown, Risk: ImpactRiskMedium, Confidence: "low"})
	if got.Class != TaskDepthDeep || got.Confidence != "low" {
		t.Fatalf("unknown depth = %#v", got)
	}
	for _, signal := range []TaskDepthSignal{TaskDepthSignalBlastRadius, TaskDepthSignalAmbiguity, TaskDepthSignalUncertainty, TaskDepthSignalVerificationCost} {
		if !containsTaskDepthSignal(got.Signals, signal) {
			t.Fatalf("unknown signals = %v, missing %q", got.Signals, signal)
		}
	}

	fragile := &taskstate.Task{
		Attempts: 2,
		Intent: &taskstate.IntentContract{
			Class:        "general",
			Action:       "implementation",
			Completeness: "targeted",
			Confidence:   "high",
		},
	}
	got = PredictTaskDepth(fragile, ImpactProfile{Scope: ImpactFocused, Risk: ImpactRiskLow, Confidence: "high"})
	if got.Class != TaskDepthDeep || !containsTaskDepthSignal(got.Signals, TaskDepthSignalHistoricalFragility) {
		t.Fatalf("fragile depth = %#v", got)
	}
}

func TestPredictTaskDepthAddsBoundedVisualQualityLoops(t *testing.T) {
	task := &taskstate.Task{
		Intent: &taskstate.IntentContract{
			Class:        "ui",
			Action:       "implementation",
			Completeness: "targeted",
			NeedsVisual:  true,
			Confidence:   "high",
		},
	}
	got := PredictTaskDepth(task, ImpactProfile{Scope: ImpactFocused, Risk: ImpactRiskMedium, Confidence: "high"})
	if got.Class != TaskDepthFocused || got.Budget.Review != BudgetStandard || got.Budget.QualityLoops != 2 {
		t.Fatalf("visual budget = %#v, depth=%q", got.Budget, got.Class)
	}
}

func TestPredictTaskDepthIsDeterministicAndDoesNotMutateTask(t *testing.T) {
	task := &taskstate.Task{
		Goal:         "architecture review",
		Attempts:     2,
		Uncertainty:  []string{"runtime boundary"},
		ChangedFiles: []string{"internal/a.go", "internal/b.go"},
		Intent:       &taskstate.IntentContract{Class: "refactor", Action: "implementation", Completeness: "targeted", Confidence: "medium"},
	}
	before := *task
	impact := ImpactProfile{Scope: ImpactCrossArea, Risk: ImpactRiskMedium, Confidence: "medium", Areas: []ImpactArea{ImpactAreaSource}}

	first := PredictTaskDepth(task, impact)
	second := PredictTaskDepth(task, impact)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("depth is not deterministic: first=%#v second=%#v", first, second)
	}
	if !reflect.DeepEqual(*task, before) {
		t.Fatalf("task was mutated: before=%#v after=%#v", before, *task)
	}
}

func TestBoundTaskDepthProfileFailsClosedAndCapsQualityLoops(t *testing.T) {
	got := boundTaskDepthProfile(TaskDepthProfile{
		Class:      "prompt-injected-depth",
		Confidence: "certain",
		Signals:    []TaskDepthSignal{"untrusted-instruction", TaskDepthSignalSecurity},
		Budget: QualityBudget{
			Research:     "untrusted",
			Verification: BudgetBroad,
			QualityLoops: 99,
		},
	})
	if got.Class != TaskDepthDeep || got.Confidence != "low" || !reflect.DeepEqual(got.Signals, []TaskDepthSignal{TaskDepthSignalSecurity}) {
		t.Fatalf("bounded profile = %#v", got)
	}
	if got.Budget.Research != BudgetElevated || got.Budget.Verification != BudgetBroad || got.Budget.QualityLoops != maxQualityLoops {
		t.Fatalf("bounded budget = %#v", got.Budget)
	}
}

func containsTaskDepthSignal(values []TaskDepthSignal, wanted TaskDepthSignal) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
