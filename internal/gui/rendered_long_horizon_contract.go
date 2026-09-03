package gui

import (
	"fmt"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/benchmark"
	"github.com/saiaathish/picogent/internal/taskstate"
)

// RenderedLongHorizonSchema identifies the bounded evidence record for a
// task-owned, rendered GUI long-horizon observation. It is an evidence format,
// not a durable task-state schema or a release decision.
const RenderedLongHorizonSchema = "picogent.v4.rendered-long-horizon.v1"

const (
	maxRenderedLongHorizonTextBytes    = 512
	maxRenderedLongHorizonObservations = benchmark.MaxLongHorizonObservations
	maxRenderedLongHorizonFiles        = 64
)

// RenderedLongHorizonProjection is the small UI projection that must agree
// with the authoritative benchmark observation after the page has settled.
// It deliberately records no transcript or provider text.
type RenderedLongHorizonProjection struct {
	TaskPresent      bool             `json:"task_present"`
	TaskStatus       taskstate.Status `json:"task_status,omitempty"`
	ProgressVisible  bool             `json:"progress_visible"`
	CompletionReady  bool             `json:"completion_ready"`
	CompletionMarker bool             `json:"completion_marker"`
	ChangedFiles     []string         `json:"changed_files,omitempty"`
}

// RenderedLongHorizonObservation combines one existing provider-independent
// long-horizon observation with the rendered state observed for that turn.
// The embedded outcome is authoritative; the rendered fields only prove that
// the GUI did not display a more optimistic state.
type RenderedLongHorizonObservation struct {
	Outcome  benchmark.TurnObservation     `json:"outcome"`
	Rendered RenderedLongHorizonProjection `json:"rendered"`
}

// RenderedLongHorizonReport is a bounded, task-owned rendered evidence
// record. SourceHead must identify the exact tree under observation. A dirty
// or unverified source can still be recorded, but it cannot silently become a
// clean rendered claim.
type RenderedLongHorizonReport struct {
	Schema            string                           `json:"schema"`
	Scenario          string                           `json:"scenario"`
	SourceHead        string                           `json:"source_head"`
	SourceVerified    bool                             `json:"source_sha_verified"`
	SourceTreeDirty   bool                             `json:"source_tree_modified"`
	Host              string                           `json:"host"`
	Runtime           string                           `json:"runtime"`
	BrowserSession    string                           `json:"browser_session"`
	BrowserTab        string                           `json:"browser_tab"`
	ObservedAtUTC     string                           `json:"observed_at_utc"`
	Command           string                           `json:"command"`
	Observations      []RenderedLongHorizonObservation `json:"observations"`
	InvariantFailures []string                         `json:"invariant_failures,omitempty"`
	Unverified        []string                         `json:"unverified,omitempty"`
}

// Validate checks the rendered record and delegates lifecycle ordering and
// fail-closed completion rules to the existing benchmark contract. It never
// upgrades an unverified source or observation into a pass.
func (r RenderedLongHorizonReport) Validate() error {
	if r.Schema != RenderedLongHorizonSchema {
		return fmt.Errorf("schema=%q, want %q", r.Schema, RenderedLongHorizonSchema)
	}
	for name, value := range map[string]string{
		"scenario":        r.Scenario,
		"host":            r.Host,
		"runtime":         r.Runtime,
		"browser_session": r.BrowserSession,
		"browser_tab":     r.BrowserTab,
		"observed_at_utc": r.ObservedAtUTC,
		"command":         r.Command,
	} {
		if err := validateRenderedLongHorizonText(name, value, true); err != nil {
			return err
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, r.ObservedAtUTC); err != nil {
		return fmt.Errorf("observed_at_utc is not RFC3339: %w", err)
	}
	if r.SourceTreeDirty && r.SourceVerified {
		return fmt.Errorf("source_sha_verified cannot be true for a modified source tree")
	}
	if (!r.SourceVerified || r.SourceTreeDirty) && len(r.Unverified) == 0 {
		return fmt.Errorf("unverified source state requires an explicit unverified boundary")
	}
	if len(r.Observations) == 0 || len(r.Observations) > maxRenderedLongHorizonObservations {
		return fmt.Errorf("observations=%d outside 1..%d", len(r.Observations), maxRenderedLongHorizonObservations)
	}
	if err := validateRenderedLongHorizonTextList("invariant_failures", r.InvariantFailures, benchmark.MaxLongHorizonFailures); err != nil {
		return err
	}
	if err := validateRenderedLongHorizonTextList("unverified", r.Unverified, benchmark.MaxLongHorizonUnverified); err != nil {
		return err
	}

	outcomes := make([]benchmark.TurnObservation, 0, len(r.Observations))
	for _, observation := range r.Observations {
		outcomes = append(outcomes, observation.Outcome)
	}
	base := benchmark.Report{
		Schema:            benchmark.LongHorizonSchema,
		Scenario:          r.Scenario,
		SourceHead:        r.SourceHead,
		Host:              r.Host,
		GoVersion:         r.Runtime,
		Command:           r.Command,
		Observations:      outcomes,
		InvariantFailures: r.InvariantFailures,
		Unverified:        r.Unverified,
	}
	if err := base.Validate(); err != nil {
		return fmt.Errorf("outcome contract: %w", err)
	}

	for index, observation := range r.Observations {
		if err := validateRenderedProjection(index, observation); err != nil {
			return err
		}
	}
	return nil
}

func validateRenderedProjection(index int, observation RenderedLongHorizonObservation) error {
	rendered := observation.Rendered
	if rendered.TaskPresent {
		if !validRenderedTaskStatus(rendered.TaskStatus) {
			return fmt.Errorf("observation %d has unknown rendered task status %q", index, rendered.TaskStatus)
		}
		if !rendered.ProgressVisible {
			return fmt.Errorf("observation %d has a task without visible progress", index)
		}
	} else if rendered.TaskStatus != "" || rendered.ProgressVisible {
		return fmt.Errorf("observation %d reports rendered task details without a task", index)
	}
	if rendered.CompletionReady != observation.Outcome.CompletionEligible {
		return fmt.Errorf("observation %d rendered completion=%t disagrees with authoritative eligibility=%t", index, rendered.CompletionReady, observation.Outcome.CompletionEligible)
	}
	if rendered.CompletionMarker != rendered.CompletionReady {
		return fmt.Errorf("observation %d completion marker=%t disagrees with rendered readiness=%t", index, rendered.CompletionMarker, rendered.CompletionReady)
	}
	if len(rendered.ChangedFiles) > maxRenderedLongHorizonFiles {
		return fmt.Errorf("observation %d changed_files=%d exceeds %d", index, len(rendered.ChangedFiles), maxRenderedLongHorizonFiles)
	}
	for fileIndex, path := range rendered.ChangedFiles {
		if err := validateRenderedLongHorizonText(fmt.Sprintf("observation %d changed_files[%d]", index, fileIndex), path, true); err != nil {
			return err
		}
	}
	return nil
}

func validRenderedTaskStatus(status taskstate.Status) bool {
	switch status {
	case taskstate.StatusPlanning, taskstate.StatusWorking, taskstate.StatusVerifying, taskstate.StatusBlocked, taskstate.StatusDone:
		return true
	default:
		return false
	}
}

func validateRenderedLongHorizonText(name, value string, required bool) error {
	if len(value) > maxRenderedLongHorizonTextBytes {
		return fmt.Errorf("%s exceeds %d bytes", name, maxRenderedLongHorizonTextBytes)
	}
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func validateRenderedLongHorizonTextList(name string, values []string, max int) error {
	if len(values) > max {
		return fmt.Errorf("%s=%d exceeds %d", name, len(values), max)
	}
	for index, value := range values {
		if err := validateRenderedLongHorizonText(fmt.Sprintf("%s[%d]", name, index), value, true); err != nil {
			return err
		}
	}
	return nil
}
