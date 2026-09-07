package runtimeboundary

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/gitobs"
)

const (
	BehaviorProvenanceExact              = "EXACT_HEAD"
	BehaviorProvenanceDocsOnlyDescendant = "DOCS_ONLY_DESCENDANT"
)

// verifyBehaviorProvenance proves that behaviorSHA is either the candidate
// itself or an ancestor whose entire candidate diff is confined to docs/.
func verifyBehaviorProvenance(workspace, behaviorSHA, candidateSHA string) (string, error) {
	if behaviorSHA == candidateSHA {
		return BehaviorProvenanceExact, nil
	}

	ancestor, err := gitobs.Output(context.Background(), workspace,
		"merge-base", "--is-ancestor", behaviorSHA, candidateSHA)
	if err != nil || ancestor.Truncated {
		return "", errors.New("behavior SHA is not a proven ancestor of candidate SHA")
	}

	history, err := gitobs.Output(context.Background(), workspace,
		"log", "--format=", "--name-only", "-z", behaviorSHA+".."+candidateSHA, "--")
	if err != nil || history.Truncated {
		return "", errors.New("behavior-to-candidate path history is unavailable")
	}
	paths := strings.Split(history.Output, "\x00")
	changed := 0
	for _, path := range paths {
		if path == "" {
			continue
		}
		changed++
		if !strings.HasPrefix(path, "docs/") || path == "docs/" {
			return "", fmt.Errorf("non-docs path changed after behavior SHA: %q", boundText(path))
		}
	}
	if changed == 0 {
		return "", errors.New("distinct behavior and candidate SHAs have no proven docs-only diff")
	}
	return BehaviorProvenanceDocsOnlyDescendant, nil
}
