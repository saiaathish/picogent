//go:build rendered_fixture

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/saiaathish/picogent/internal/gui"
)

func main() {
	phase := flag.String("phase", "seed", "fixture phase: seed or reload")
	home := flag.String("home", "", "disposable PICOGENT_HOME; required for reload")
	workspace := flag.String("workspace", "", "disposable workspace; required for reload")
	manifest := flag.String("manifest", "", "manifest path (defaults inside the fixture home)")
	addr := flag.String("addr", "127.0.0.1:0", "loopback listen address")
	flag.Parse()

	_ = os.Setenv("PICOGENT_RENDERED_FIXTURE_PHASE", *phase)
	if *home != "" {
		_ = os.Setenv("PICOGENT_RENDERED_FIXTURE_HOME", *home)
	}
	if *workspace != "" {
		_ = os.Setenv("PICOGENT_RENDERED_FIXTURE_WORKSPACE", *workspace)
	}
	if *manifest != "" {
		_ = os.Setenv("PICOGENT_RENDERED_FIXTURE_MANIFEST", *manifest)
	}
	if *addr != "" {
		_ = os.Setenv("PICOGENT_RENDERED_FIXTURE_ADDR", *addr)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := gui.RunRenderedRecoveryFixture(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
