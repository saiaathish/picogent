// Command rendered-cross-platform-evidence packages one validated rendered
// observation per desktop platform. It does not create observations or
// authorize a release.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/runtimeboundary"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("rendered-cross-platform-evidence", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspace := flags.String("workspace", ".", "clean source workspace used for artifact containment checks")
	candidateSHA := flags.String("candidate-sha", "", "exact full commit id shared by all observations")
	darwin := flags.String("darwin", "", "darwin rendered-platform evidence artifact")
	linux := flags.String("linux", "", "linux rendered-platform evidence artifact")
	windows := flags.String("windows", "", "windows rendered-platform evidence artifact")
	outPath := flags.String("out", "", "required absolute aggregate artifact path outside the workspace")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintln(stderr, "rendered-cross-platform-evidence canceled:", err)
		return 1
	}
	if strings.TrimSpace(*outPath) == "" {
		fmt.Fprintln(stderr, "rendered-cross-platform-evidence: -out is required")
		return 2
	}

	absWorkspace, err := filepath.Abs(*workspace)
	if err != nil {
		fmt.Fprintln(stderr, "resolve workspace:", err)
		return 1
	}
	absOut, err := filepath.Abs(*outPath)
	if err != nil {
		fmt.Fprintln(stderr, "resolve aggregate artifact:", err)
		return 1
	}
	evidence, err := runtimeboundary.AggregateRenderedCrossPlatformEvidence(
		absWorkspace,
		strings.TrimSpace(*candidateSHA),
		map[string]string{
			"darwin":  *darwin,
			"linux":   *linux,
			"windows": *windows,
		},
		time.Time{},
	)
	if err != nil {
		fmt.Fprintln(stderr, "rendered-cross-platform-evidence:", err)
		return 1
	}
	if err := runtimeboundary.RetainRenderedCrossPlatformEvidence(absWorkspace, absOut, evidence); err != nil {
		fmt.Fprintln(stderr, "retain aggregate:", err)
		return 1
	}
	loaded, digest, err := runtimeboundary.LoadRenderedCrossPlatformEvidence(absWorkspace, absOut, evidence.CandidateSHA)
	if err != nil {
		fmt.Fprintln(stderr, "validate aggregate:", err)
		return 1
	}
	if loaded.CandidateSHA != evidence.CandidateSHA || loaded.Verdict != evidence.Verdict {
		fmt.Fprintln(stderr, "validate aggregate: retained artifact changed during validation")
		return 1
	}
	if err := runtimeboundary.WriteRenderedCrossPlatformEvidence(stdout, loaded); err != nil {
		fmt.Fprintln(stderr, "write aggregate:", err)
		return 1
	}
	fmt.Fprintf(stderr, "retained rendered cross-platform evidence: %s (%s)\n", absOut, digest)
	return 0
}
