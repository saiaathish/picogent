package tools

import (
	"context"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/measure"
	"github.com/saiaathish/picogent/internal/perm"
)

type measureTool struct{}

func (measureTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "measure",
		Description: "Run a fixed, bounded project benchmark for performance work. It currently supports Go modules, requires permission, and returns parsed benchmark metrics without accepting a model-supplied command.",
		Parameters:  schema(map[string]any{}, []string{}),
	}
}

func (measureTool) Permission(_ string, _ Context) perm.Request {
	return perm.Request{Tool: "measure", Summary: "run the fixed project benchmark"}
}

func (measureTool) Run(ctx context.Context, _ string, c Context) (string, error) {
	workspace, err := mustWorkspace(c)
	if err != nil {
		return "", err
	}
	return measure.Format(measure.Run(ctx, workspace)), nil
}
