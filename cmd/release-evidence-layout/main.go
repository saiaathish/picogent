// Command release-evidence-layout validates the filesystem boundary used by
// the release-evidence workflow. It never authorizes a release.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/saiaathish/picogent/internal/verify"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("release-evidence-layout", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspace := flags.String("workspace", "", "absolute checked-out workspace path")
	evidenceDir := flags.String("evidence-dir", "", "absolute runner-temporary evidence directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintln(stderr, "release evidence layout validation canceled:", err)
		return 1
	}
	if err := verify.ValidateReleaseEvidenceDirectory(*workspace, *evidenceDir); err != nil {
		fmt.Fprintln(stderr, "release evidence layout:", err)
		return 1
	}
	fmt.Fprintln(stdout, "release evidence layout PASS")
	return 0
}
