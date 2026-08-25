package tools

import (
	"context"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/repomap"
)

type repoMapTool struct{}

func (repoMapTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "repo_map",
		Description: "Inspect the current workspace on demand and return a compact deterministic map of languages, frameworks, package manager, commands, git state, exact repository provenance, manifests, source/test roots, and project rules. No index or background process.",
		Parameters:  schema(map[string]any{}, []string{}),
	}
}

func (repoMapTool) Permission(_ string, _ Context) perm.Request {
	return perm.Request{Tool: "repo_map", Summary: "inspect workspace structure"}
}

func (repoMapTool) Run(ctx context.Context, _ string, c Context) (string, error) {
	snapshot, err := repomap.Capture(ctx, c.Workspace)
	if err != nil {
		return "", err
	}
	return repomap.FormatSnapshot(snapshot), nil
}
