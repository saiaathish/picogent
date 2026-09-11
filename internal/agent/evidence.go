package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/workspace"
)

// verificationEvidence keeps the tool's human-readable result alongside the
// bounded workspace boundary observed around the check. An unusable boundary
// is retained as explicit state so a PASS cannot silently become durable proof.
type verificationEvidence struct {
	output            string
	err               error
	targets           []string
	observation       *workspace.Observation
	observationUsable bool
	observationReason string
}

// noteVisualEvidence binds only a package-owned browser screenshot producer to
// the durable visual requirement. Screenshot-shaped text, model narration, or
// generic MCP output never reaches this function without first passing the
// catalog-bound identity and image validation in internal/mcpbridge.
func (a *Agent) noteVisualEvidence(producer tools.ProducerResult, err error, ev EventHandler) bool {
	visual := producer.Visual
	if visual == nil {
		return false
	}
	status := "PASS"
	summary := "live browser screenshot returned a validated image"
	switch {
	case err != nil || visual.ResultError:
		status = "FAIL"
		summary = "live browser screenshot producer failed before producing usable visual evidence"
	case visual.ValidImageCount <= 0:
		status = "INCONCLUSIVE"
		summary = "live browser screenshot producer returned no validated image"
	}
	reference := strings.TrimSpace(visual.Reference)
	if reference == "" {
		reference = "browser screenshot producer"
	}
	return a.mutateTask(ev, func(task *taskstate.Task) error {
		task.RecordBrowserEvidence(status, summary, reference)
		return nil
	})
}

// noteMeasurementEvidence binds only the package-owned fixed benchmark
// producer to the durable measurement requirement. A passing-looking tool
// string, an incomplete run, or a producer with no canonical metrics is never
// allowed to become passing evidence.
func (a *Agent) noteMeasurementEvidence(producer tools.ProducerResult, err error, ev EventHandler) bool {
	measurement := producer.Measurement
	if measurement == nil {
		return false
	}
	status := strings.ToUpper(strings.TrimSpace(string(measurement.Status)))
	switch {
	case err != nil:
		status = "FAIL"
	case measurement.OutputTruncated:
		status = "INCONCLUSIVE"
	case status == "PASS" && measurement.Benchmarks <= 0:
		status = "INCONCLUSIVE"
	case status != "PASS" && status != "FAIL" && status != "INCONCLUSIVE":
		status = "INCONCLUSIVE"
	}
	benchmarks := measurement.Benchmarks
	if benchmarks < 0 {
		benchmarks = 0
	}
	summary := fmt.Sprintf("fixed project measurement %s; canonical benchmarks=%d", status, benchmarks)
	return a.mutateTask(ev, func(task *taskstate.Task) error {
		task.RecordMeasurementEvidence(status, summary, "fixed measure tool")
		return nil
	})
}

func cloneLLMParts(parts []llm.Part) []llm.Part {
	if len(parts) == 0 {
		return nil
	}
	out := make([]llm.Part, 0, len(parts))
	for _, part := range parts {
		clone := part
		clone.Data = append([]byte(nil), part.Data...)
		out = append(out, clone)
	}
	return out
}

// executeVerification observes the requested paths before and after the
// verifier runs. Any mutation during the check makes a passing result
// inconclusive, even when the final bytes happen to look stable.
func executeVerification(ctx context.Context, tool tools.Tool, call llm.ToolCall, c tools.Context, runner func(context.Context, llm.ToolCall, tools.Tool, tools.Context) (string, error)) verificationEvidence {
	targets := verificationTargetsFromArgs(call.Arguments)
	before, beforeReason := captureVerificationObservation(ctx, c.Workspace, targets)
	var out string
	var err error
	if runner != nil {
		out, err = runner(ctx, call, tool, c)
	} else {
		out, err = tool.Run(ctx, call.Arguments, c)
	}
	if err != nil {
		out = "error: " + err.Error()
	}
	after, afterReason := captureVerificationObservation(ctx, c.Workspace, targets)
	evidence := verificationEvidence{
		output:      out,
		err:         err,
		targets:     append([]string(nil), targets...),
		observation: cloneWorkspaceObservation(after),
	}
	switch {
	case beforeReason != "":
		evidence.observationReason = beforeReason
	case afterReason != "":
		evidence.observationReason = afterReason
	case before == nil || after == nil:
		evidence.observationReason = "workspace observation is missing"
	default:
		comparison := workspace.Compare(*before, *after)
		if !comparison.Fresh {
			evidence.observationReason = "workspace changed during verification: " + comparison.Reason
		} else {
			evidence.observationUsable = true
		}
	}
	return evidence
}

func captureVerificationObservation(ctx context.Context, root string, targets []string) (*workspace.Observation, string) {
	if len(targets) == 0 {
		return nil, "no tracked verification paths"
	}
	observation, err := workspace.Capture(ctx, root, targets)
	if err != nil {
		return nil, "workspace observation unavailable: " + err.Error()
	}
	if comparison := workspace.Compare(observation, observation); !comparison.Fresh {
		return &observation, comparison.Reason
	}
	return &observation, ""
}

func verificationTargetsFromArgs(args string) []string {
	var in struct {
		Targets []string `json:"targets"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return nil
	}
	return in.Targets
}

func observationPaths(observation *workspace.Observation) []string {
	if observation == nil {
		return nil
	}
	paths := make([]string, 0, len(observation.Files))
	for _, file := range observation.Files {
		paths = append(paths, file.Path)
	}
	return paths
}

func cloneWorkspaceObservation(observation *workspace.Observation) *workspace.Observation {
	if observation == nil {
		return nil
	}
	clone := *observation
	clone.Files = append([]workspace.FileObservation(nil), observation.Files...)
	return &clone
}

func verificationObservationUsable(evidence verificationEvidence) bool {
	return evidence.observationUsable && evidence.observation != nil && len(observationPaths(evidence.observation)) > 0
}

func normalizeVerificationEvidence(evidence verificationEvidence) verificationEvidence {
	if strings.TrimSpace(evidence.output) != "" && verificationStatus(evidence.output) == "PASS" && !verificationObservationUsable(evidence) {
		evidence.output = inconclusiveVerification(evidence.observationReason)
	}
	return evidence
}

func recheckVerificationEvidence(ctx context.Context, root string, evidence verificationEvidence) (bool, string) {
	_, fresh, reason := recheckVerificationEvidenceObservation(ctx, root, evidence)
	return fresh, reason
}

func recheckVerificationEvidenceObservation(ctx context.Context, root string, evidence verificationEvidence) (*workspace.Observation, bool, string) {
	if !verificationObservationUsable(evidence) {
		if reason := strings.TrimSpace(evidence.observationReason); reason != "" {
			return nil, false, reason
		}
		return nil, false, "workspace observation is not usable"
	}
	after, reason := captureVerificationObservation(ctx, root, observationPaths(evidence.observation))
	if reason != "" {
		return after, false, reason
	}
	if after == nil {
		return nil, false, "workspace observation is missing"
	}
	comparison := workspace.Compare(*evidence.observation, *after)
	if !comparison.Fresh {
		if comparison.Reason == "" {
			return after, false, "workspace evidence is not fresh"
		}
		return after, false, comparison.Reason
	}
	return after, true, ""
}

func inconclusiveVerification(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "workspace evidence is not fresh"
	}
	const maxReason = 760
	if len(reason) > maxReason {
		reason = reason[:maxReason] + "…"
	}
	return fmt.Sprintf("verify INCONCLUSIVE — %s", reason)
}
