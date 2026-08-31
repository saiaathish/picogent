// Command release-gates validates the bounded CI gate ledger used by release
// evidence. It never authorizes task or goal completion.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/saiaathish/picogent/internal/verify"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("release-gates", flag.ContinueOnError)
	flags.SetOutput(stderr)
	ledgerPath := flags.String("ledger", "", "release gate ledger JSON path")
	expectedSHA := flags.String("expected-sha", "", "expected full Git commit ID")
	event := flags.String("event", "", "expected GitHub event name")
	required := flags.String("required", "", "comma-separated required job names")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintln(stderr, "release gate validation canceled:", err)
		return 1
	}
	if strings.TrimSpace(*ledgerPath) == "" {
		fmt.Fprintln(stderr, "release gate ledger path is required")
		return 2
	}
	file, err := os.Open(*ledgerPath)
	if err != nil {
		fmt.Fprintln(stderr, "open release gate ledger:", err)
		return 1
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, verify.MaxReleaseGateBytes+1))
	if err != nil {
		fmt.Fprintln(stderr, "read release gate ledger:", err)
		return 1
	}
	if len(data) > verify.MaxReleaseGateBytes {
		fmt.Fprintln(stderr, "release gate ledger exceeds size limit")
		return 1
	}
	var ledger verify.ReleaseGateLedger
	if err := json.Unmarshal(data, &ledger); err != nil {
		fmt.Fprintln(stderr, "decode release gate ledger:", err)
		return 1
	}
	jobs := make([]string, 0)
	for _, job := range strings.Split(*required, ",") {
		if job = strings.TrimSpace(job); job != "" {
			jobs = append(jobs, job)
		}
	}
	if err := verify.ValidateReleaseGateLedger(ledger, *expectedSHA, *event, jobs); err != nil {
		fmt.Fprintln(stderr, "release gates:", err)
		return 1
	}
	fmt.Fprintf(stdout, "release gates PASS: %d required job(s) for %s\n", len(jobs), strings.TrimSpace(*event))
	return 0
}
