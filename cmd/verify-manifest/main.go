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
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/verify"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("target must not be empty")
	}
	*s = append(*s, value)
	return nil
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("verify-manifest", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspaceFlag := flags.String("workspace", ".", "workspace to verify")
	expectedSHA := flags.String("expected-sha", "", "expected full Git commit ID")
	coverProfile := flags.String("coverprofile", "", "absolute path for the targeted Go coverprofile artifact")
	timeoutFlag := flags.String("timeout", "15m", "per-command timeout for broader verification")
	targetedTimeoutFlag := flags.String("targeted-timeout", "45s", "per-command timeout for targeted verification")
	var targets stringList
	flags.Var(&targets, "target", "workspace-relative file or directory for targeted verification (repeatable)")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	workspace, err := filepath.Abs(*workspaceFlag)
	if err != nil {
		fmt.Fprintln(stderr, "resolve workspace:", err)
		return 1
	}
	timeout, err := time.ParseDuration(strings.TrimSpace(*timeoutFlag))
	if err != nil || timeout <= 0 {
		fmt.Fprintln(stderr, "invalid --timeout:", *timeoutFlag)
		return 2
	}
	targetedTimeout, err := time.ParseDuration(strings.TrimSpace(*targetedTimeoutFlag))
	if err != nil || targetedTimeout <= 0 {
		fmt.Fprintln(stderr, "invalid --targeted-timeout:", *targetedTimeoutFlag)
		return 2
	}

	options := verify.Options{
		Targets:         append([]string(nil), targets...),
		Timeout:         timeout,
		TargetedTimeout: targetedTimeout,
	}
	if profile := strings.TrimSpace(*coverProfile); profile != "" {
		absProfile, err := filepath.Abs(profile)
		if err != nil {
			fmt.Fprintln(stderr, "resolve coverprofile:", err)
			return 1
		}
		if err := verify.ValidateReleaseEvidenceDirectory(workspace, filepath.Dir(absProfile)); err != nil {
			fmt.Fprintln(stderr, "coverprofile layout:", err)
			return 1
		}
		if len(options.Targets) == 0 {
			fmt.Fprintln(stderr, "coverprofile requires at least one --target")
			return 2
		}
		options.CoverProfile = absProfile
	}

	provenance := verify.CollectProvenance(ctx, workspace, *expectedSHA)
	pipeline := verify.RunPipeline(ctx, workspace, options)
	manifest := verify.ManifestFromPipeline(pipeline, provenance)
	if err := verify.WriteJSON(stdout, manifest); err != nil {
		fmt.Fprintln(stderr, "write manifest:", err)
		return 1
	}
	return 0
}
