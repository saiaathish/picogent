// Command release-artifact builds deterministic production packages and SPDX
// SBOMs for release-readiness evidence. It does not authorize a release.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/saiaathish/picogent/internal/releaseartifact"
	"github.com/saiaathish/picogent/internal/securefile"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("release-artifact", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspace := flags.String("workspace", ".", "clean source workspace")
	outputDir := flags.String("output-dir", "", "absolute directory outside the workspace for artifacts")
	candidateSHA := flags.String("candidate-sha", "", "exact full commit id")
	sourceDate := flags.String("source-date-epoch", "", "unix timestamp used for package metadata")
	goVersion := flags.String("go-version", "", "optional recorded Go version label")
	targetsFlag := flags.String("targets", "linux/amd64,darwin/arm64,windows/amd64", "comma-separated GOOS/GOARCH targets")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintln(stderr, "release-artifact canceled:", err)
		return 1
	}
	if strings.TrimSpace(*outputDir) == "" {
		fmt.Fprintln(stderr, "--output-dir is required")
		return 2
	}
	epoch, err := strconv.ParseInt(strings.TrimSpace(*sourceDate), 10, 64)
	if err != nil || epoch <= 0 {
		fmt.Fprintln(stderr, "invalid --source-date-epoch")
		return 2
	}
	targets, err := parseTargets(*targetsFlag)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	absWorkspace, err := filepath.Abs(*workspace)
	if err != nil {
		fmt.Fprintln(stderr, "resolve workspace:", err)
		return 1
	}
	manifest, err := releaseartifact.Build(releaseartifact.Options{
		Workspace:       absWorkspace,
		OutputDir:       *outputDir,
		CandidateSHA:    strings.TrimSpace(*candidateSHA),
		SourceDateEpoch: epoch,
		GoVersion:       strings.TrimSpace(*goVersion),
		Targets:         targets,
	})
	if err != nil {
		fmt.Fprintln(stderr, "release-artifact:", err)
		return 1
	}
	manifestPath := filepath.Join(*outputDir, "release-artifact-manifest.json")
	var manifestData bytes.Buffer
	if err := releaseartifact.WriteManifest(&manifestData, manifest); err != nil {
		fmt.Fprintln(stderr, "encode manifest:", err)
		return 1
	}
	if err := securefile.WriteAtomic(manifestPath, manifestData.Bytes(), 0o644); err != nil {
		fmt.Fprintln(stderr, "write manifest:", err)
		return 1
	}
	if _, err := stdout.Write(manifestData.Bytes()); err != nil {
		fmt.Fprintln(stderr, "emit manifest:", err)
		return 1
	}
	return 0
}

func parseTargets(raw string) ([]releaseartifact.Target, error) {
	parts := strings.Split(raw, ",")
	out := make([]releaseartifact.Target, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Split(part, "/")
		if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
			return nil, fmt.Errorf("invalid --targets entry %q", part)
		}
		out = append(out, releaseartifact.Target{GOOS: fields[0], GOARCH: fields[1]})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--targets must list at least one GOOS/GOARCH")
	}
	return out, nil
}
