package tools

import (
	"context"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/measure"
)

// ProducerResult carries only ephemeral typed output from a package-owned
// producer. It is intentionally separate from Tool so arbitrary tools and
// model text cannot self-declare trusted evidence.
type ProducerResult struct {
	Visual      *VisualEvidence
	Measurement *MeasurementEvidence
}

// MeasurementEvidence is admitted only from the package-owned fixed
// measurement producer. It intentionally carries no arbitrary command output
// or model-supplied labels into the agent's durable evidence path.
type MeasurementEvidence struct {
	Status          measure.Status
	Benchmarks      int
	OutputTruncated bool
}

// VisualEvidence is a live browser screenshot observation. Reference and
// counts are suitable for bounded durable evidence; Parts are transient input
// for the next model request and must never be persisted as task history.
type VisualEvidence struct {
	Reference       string
	ResultError     bool
	ValidImageCount int
	Parts           []llm.Part
}

// evidenceRunner has an unexported method so only this package can implement
// the typed producer seam. External/custom Tool implementations can still run
// normally, but cannot fabricate ProducerResult values through this interface.
type evidenceRunner interface {
	runWithEvidence(ctx context.Context, args string, c Context) (string, error, ProducerResult)
}

// RunWithEvidence runs a tool and returns any producer metadata admitted by a
// package-owned typed implementation. The ordinary Tool contract remains the
// compatibility path for all other tools.
func RunWithEvidence(ctx context.Context, tool Tool, args string, c Context) (string, error, ProducerResult) {
	if runner, ok := tool.(evidenceRunner); ok {
		return runner.runWithEvidence(ctx, args, c)
	}
	out, err := tool.Run(ctx, args, c)
	return out, err, ProducerResult{}
}
