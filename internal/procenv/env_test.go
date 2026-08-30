package procenv

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSanitizedRemovesImplicitExecutionAndSecretInputs(t *testing.T) {
	for _, key := range []string{
		"PICOGENT_TEST_API_TOKEN", "BASH_ENV", "LD_PRELOAD", "DYLD_INSERT_LIBRARIES",
		"GIT_CONFIG_COUNT", "NPM_CONFIG_USERCONFIG", "GOFLAGS", "RUSTC_WRAPPER", "PAGER",
		"RIPGREP_CONFIG_PATH", "GREP_OPTIONS",
	} {
		t.Setenv(key, "sentinel")
	}
	t.Setenv("PICOGENT_TEST_SAFE", "kept")

	env := Sanitized()
	joined := strings.Join(env, "\n")
	for _, key := range []string{
		"PICOGENT_TEST_API_TOKEN", "BASH_ENV", "LD_PRELOAD", "DYLD_INSERT_LIBRARIES",
		"GIT_CONFIG_COUNT", "NPM_CONFIG_USERCONFIG", "GOFLAGS", "RUSTC_WRAPPER", "PAGER",
		"RIPGREP_CONFIG_PATH", "GREP_OPTIONS",
	} {
		if strings.Contains(joined, key+"=") {
			t.Fatalf("unsafe environment variable leaked: %s", key)
		}
	}
	if !strings.Contains(joined, "PICOGENT_TEST_SAFE=kept") {
		t.Fatal("ordinary environment was unexpectedly removed")
	}
	if path, ok := os.LookupEnv("PATH"); ok && !hasKey(env, "PATH") {
		t.Fatalf("PATH was removed despite being present: %q", path)
	}
}

func TestUnsafeKeyIsCaseInsensitive(t *testing.T) {
	for _, key := range []string{"git_config_global", "nPm_CoNfIg_UsErCoNfIg", "dyld_insert_libraries", "api_key", "gh_pager", "manpager"} {
		if !UnsafeKey(key) {
			t.Fatalf("UnsafeKey(%q) = false", key)
		}
	}
	if UnsafeKey("PICOGENT_TEST_SAFE") {
		t.Fatal("ordinary key classified as unsafe")
	}
}

func TestOutputSanitizesEnvironmentAndBoundsOutput(t *testing.T) {
	t.Setenv("PROCENV_TEST_SECRET", "must-not-cross")
	t.Setenv("PROCENV_HELPER", "1")
	// The helper is a fresh test binary. Five seconds leaves room for race
	// instrumentation and cold-start variance while still bounding a hung
	// helper tightly for this contract test.
	result, err := Output(context.Background(), 5*time.Second, os.Args[0], "-test.run=TestProcenvHelperProcess")
	if err != nil {
		t.Fatal(err)
	}
	if result.Truncated {
		t.Fatal("helper output unexpectedly truncated")
	}
	if string(result.Output) != "clean\n" {
		t.Fatalf("helper observed unsanitized environment: %q", result.Output)
	}
}

func TestOutputDrainsNoisyProcessAtBound(t *testing.T) {
	t.Setenv("PROCENV_HELPER", "spam")
	result, err := Output(context.Background(), 5*time.Second, os.Args[0], "-test.run=TestProcenvHelperProcess")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || len(result.Output) != MaxOutputBytes {
		t.Fatalf("bounded result = len %d truncated %v", len(result.Output), result.Truncated)
	}
}

func TestOutputStopsAtDeadline(t *testing.T) {
	t.Setenv("PROCENV_HELPER", "sleep")
	started := time.Now()
	_, err := Output(context.Background(), 50*time.Millisecond, os.Args[0], "-test.run=TestProcenvHelperProcess")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestProcenvHelperProcess(t *testing.T) {
	if os.Getenv("PROCENV_HELPER") == "" {
		return
	}
	switch os.Getenv("PROCENV_HELPER") {
	case "spam":
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("x", MaxOutputBytes+4096))
	case "sleep":
		time.Sleep(5 * time.Second)
	default:
		if os.Getenv("PROCENV_TEST_SECRET") != "" {
			_, _ = fmt.Fprintln(os.Stdout, "leaked")
			return
		}
		_, _ = fmt.Fprintln(os.Stdout, "clean")
	}
	os.Exit(0)
}
