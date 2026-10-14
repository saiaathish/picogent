package gui

import (
	"fmt"
	"os"
	"testing"
)

// Some handler tests persist config as part of a scripted turn. Give every GUI
// test a disposable default profile, including tests without their own Setenv.
// Individual tests can still override PICOGENT_HOME with a narrower fixture.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "picogent-gui-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Setenv("PICOGENT_HOME", home); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
