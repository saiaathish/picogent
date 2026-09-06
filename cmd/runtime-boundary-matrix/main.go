// Command runtime-boundary-matrix emits the bounded v4 runtime-boundary
// evidence matrix. It does not authorize a release.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/runtimeboundary"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("runtime-boundary-matrix", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspace := flags.String("workspace", ".", "clean source workspace")
	candidateSHA := flags.String("candidate-sha", "", "exact full commit id")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintln(stderr, "runtime-boundary-matrix canceled:", err)
		return 1
	}
	abs, err := filepath.Abs(*workspace)
	if err != nil {
		fmt.Fprintln(stderr, "resolve workspace:", err)
		return 1
	}
	report, err := runtimeboundary.Collect(runtimeboundary.Options{
		Workspace:    abs,
		CandidateSHA: strings.TrimSpace(*candidateSHA),
	})
	if err != nil {
		fmt.Fprintln(stderr, "runtime-boundary-matrix:", err)
		return 1
	}
	if err := runtimeboundary.WriteJSON(stdout, report); err != nil {
		fmt.Fprintln(stderr, "write matrix:", err)
		return 1
	}
	return 0
}
