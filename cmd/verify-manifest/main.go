// Command verify-manifest emits developer-facing verification evidence.
// It is intentionally separate from the Picogent user-facing binary and does
// not authorize task or goal completion.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/saiaathish/picogent/internal/verify"
)

func main() {
	workspaceFlag := flag.String("workspace", ".", "workspace to verify")
	expectedSHA := flag.String("expected-sha", "", "expected full Git commit ID")
	flag.Parse()

	workspace, err := filepath.Abs(*workspaceFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve workspace:", err)
		os.Exit(1)
	}
	provenance := verify.CollectProvenance(context.Background(), workspace, *expectedSHA)
	pipeline := verify.RunPipeline(context.Background(), workspace, verify.Options{})
	manifest := verify.ManifestFromPipeline(pipeline, provenance)
	if err := verify.WriteJSON(os.Stdout, manifest); err != nil {
		fmt.Fprintln(os.Stderr, "write manifest:", err)
		os.Exit(1)
	}
}
