package main

import (
	"context"
	"fmt"
	"os"

	"github.com/saiaathish/picogent/internal/benchmark"
)

func main() {
	if err := benchmark.RunOutcomeQualityWorker(context.Background(), os.Stdin, os.Stdout, benchmark.NewOutcomeQualityAgentExecutor()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "outcome-quality worker:", err)
		os.Exit(1)
	}
}
