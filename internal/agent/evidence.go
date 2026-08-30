package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
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
