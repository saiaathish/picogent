package tools

import (
	"context"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/projecthealth"
)

type projectHealthTool struct{}

func (projectHealthTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "project_health",
		Description: "Read-only, bounded diagnosis for broad outcome requests such as making a project launch-ready or healthy. It inspects project metadata, manifests, and Git provenance without running build/test/lint commands. Returns ranked evidence and explicit UNKNOWN/UNVERIFIED states; do not use it for tiny edits.",
		Parameters:  schema(map[string]any{}, []string{}),
	}
}

func (projectHealthTool) Permission(_ string, c Context) perm.Request {
	return c.ClassifyPath("project_health", ".", c.Workspace, "inspect project health")
}

func (projectHealthTool) Run(ctx context.Context, _ string, c Context) (string, error) {
	workspace, err := mustWorkspace(c)
	if err != nil {
		return "", err
	}
	report, err := projecthealth.Assess(ctx, workspace)
	if err != nil {
		return "", err
	}
	return projecthealth.Format(report), nil
}
