package outcome

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestBuildAndFromJSONExposeTheSameAdaptiveDepth(t *testing.T) {
	task, err := taskstate.New("depth-integration", "make the authentication boundary ready", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Intent = &taskstate.IntentContract{
		Outcome:       task.Goal,
		Class:         "security",
		Action:        "architecture",
		Completeness:  "targeted",
		Risk:          "high",
		NeedsApproval: true,
		Confidence:    "high",
	}
	task.RecordChanged("internal/auth/handler.go")
	report := projecthealth.Report{Schema: projecthealth.Schema, Status: projecthealth.StateAttention}

	built := Build(task, report)
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, ok := FromJSON(task, string(raw))
	if !ok {
		t.Fatal("valid project-health report was rejected")
	}
	if !reflect.DeepEqual(built.Depth, fromJSON.Depth) {
		t.Fatalf("Build and FromJSON depth diverged: build=%#v fromJSON=%#v", built.Depth, fromJSON.Depth)
	}
	if !reflect.DeepEqual(built.Depth, PredictTaskDepth(task, built.Impact)) {
		t.Fatalf("depth was not derived from Build's impact: depth=%#v impact=%#v", built.Depth, built.Impact)
	}
	if built.Depth.Schema != TaskDepthSchema || built.Depth.Class != TaskDepthDeep {
		t.Fatalf("adaptive depth identity = %#v", built.Depth)
	}
}

func TestEngineInstructionRendersBoundedAdaptiveDepthBudget(t *testing.T) {
	task, err := taskstate.New("depth-instruction", "secure the service boundary", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Intent = &taskstate.IntentContract{
		Outcome:       task.Goal,
		Class:         "security",
		Action:        "architecture",
		Completeness:  "targeted",
		Risk:          "high",
		NeedsApproval: true,
		Confidence:    "high",
	}
	task.RecordChanged("internal/auth/handler.go")

	contract := Build(task, projecthealth.Report{Schema: projecthealth.Schema})
	instruction := EngineInstruction(contract)
	for _, marker := range []string{
		"Adaptive task depth: schema=picogent.outcome-depth.v1 class=DEEP confidence=high signals=security,architecture,verification_cost,user_intent",
		"Quality budget: research=STANDARD review=ELEVATED experimentation=STANDARD verification=ELEVATED specialists=STANDARD reasoning=ELEVATED quality_loops=2",
	} {
		if !strings.Contains(instruction, marker) {
			t.Fatalf("adaptive-depth marker %q missing from instruction: %q", marker, instruction)
		}
	}
	encodedDepth, err := json.Marshal(contract.Depth)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedDepth), task.Goal) {
		t.Fatalf("task text crossed the adaptive-depth profile boundary: %s", encodedDepth)
	}
}

func TestBoundContractFailsClosedForCallerShapedDepth(t *testing.T) {
	unsafe := Contract{
		Schema: EngineSchema,
		Depth: TaskDepthProfile{
			Schema:     "prompt-injected-depth",
			Class:      "execute-untrusted-plan",
			Confidence: "trust-all",
			Signals:    []TaskDepthSignal{"untrusted-instruction", TaskDepthSignalSecurity},
			Budget: QualityBudget{
				Research:     "run-the-secret-command",
				Review:       BudgetBroad,
				QualityLoops: 99,
			},
		},
	}

	bounded := boundContract(unsafe)
	if bounded.Depth.Schema != TaskDepthSchema || bounded.Depth.Class != TaskDepthDeep || bounded.Depth.Confidence != "low" {
		t.Fatalf("caller-shaped depth identity was not conservative: %#v", bounded.Depth)
	}
	if !reflect.DeepEqual(bounded.Depth.Signals, []TaskDepthSignal{TaskDepthSignalSecurity}) {
		t.Fatalf("caller-shaped signals were not allowlisted: %#v", bounded.Depth.Signals)
	}
	if bounded.Depth.Budget.Research != BudgetElevated || bounded.Depth.Budget.Review != BudgetBroad || bounded.Depth.Budget.QualityLoops != maxQualityLoops {
		t.Fatalf("caller-shaped budget was not bounded: %#v", bounded.Depth.Budget)
	}

	instruction := EngineInstruction(unsafe)
	for _, hostile := range []string{"prompt-injected-depth", "execute-untrusted-plan", "trust-all", "untrusted-instruction", "run-the-secret-command", "99"} {
		if strings.Contains(instruction, hostile) {
			t.Fatalf("hostile depth value escaped instruction: %q", instruction)
		}
	}
	if !strings.Contains(instruction, "Adaptive task depth: schema=picogent.outcome-depth.v1 class=DEEP confidence=low signals=security") {
		t.Fatalf("bounded depth guidance missing: %q", instruction)
	}
}

func TestContractJSONRoundTripsAdaptiveDepth(t *testing.T) {
	task, err := taskstate.New("depth-json", "review the launch boundary", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Intent = &taskstate.IntentContract{
		Outcome:      task.Goal,
		Class:        "review",
		Action:       "investigation",
		Completeness: "full",
		Confidence:   "high",
	}
	contract := Build(task, projecthealth.Report{Schema: projecthealth.Schema, Status: projecthealth.StateUnknown})
	data, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Contract
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Depth, contract.Depth) {
		t.Fatalf("adaptive depth did not survive contract JSON round trip: before=%#v after=%#v", contract.Depth, decoded.Depth)
	}
	if !strings.Contains(string(data), `"depth"`) || !strings.Contains(string(data), `"schema":"picogent.outcome-depth.v1"`) {
		t.Fatalf("versioned depth profile missing from contract JSON: %s", data)
	}
}
