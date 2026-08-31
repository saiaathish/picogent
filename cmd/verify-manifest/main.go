// Command verify-manifest emits developer-facing verification evidence.
// It is intentionally separate from the Picogent user-facing binary and does
// not authorize task or goal completion.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/saiaathish/picogent/internal/verify"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("verify-manifest", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspaceFlag := flags.String("workspace", ".", "workspace to verify")
	expectedSHA := flags.String("expected-sha", "", "expected full Git commit ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	workspace, err := filepath.Abs(*workspaceFlag)
	if err != nil {
		fmt.Fprintln(stderr, "resolve workspace:", err)
		return 1
	}
	provenance := verify.CollectProvenance(ctx, workspace, *expectedSHA)
	pipeline := verify.RunPipeline(ctx, workspace, verify.Options{})
	manifest := verify.ManifestFromPipeline(pipeline, provenance)
	if err := verify.WriteJSON(stdout, manifest); err != nil {
		fmt.Fprintln(stderr, "write manifest:", err)
		return 1
	}
	return 0
}
