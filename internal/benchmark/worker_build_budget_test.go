package benchmark

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestOutcomeQualityWorkerBuildCapsConcurrency(t *testing.T) {
	for _, parent := range []string{"", "99"} {
		t.Run("parent-"+parent, func(t *testing.T) {
			t.Setenv("GOMAXPROCS", parent)
			t.Setenv("GOFLAGS", "-p=99 -toolexec=untrusted-parent-tool")
			t.Setenv("OPENAI_API_KEY", "synthetic-build-key-must-not-propagate")
			command := outcomeQualityWorkerBuildCommand(context.Background(), "go", "source", "output", "cache")
			wantArgs := []string{"go", "build", "-p=1", "-o", "output", "./cmd/outcome-quality-worker"}
			if !reflect.DeepEqual(command.Args, wantArgs) {
				t.Fatalf("unbounded build arguments: %v", command.Args)
			}
			counts := map[string]int{}
			for _, entry := range command.Env {
				key, value, ok := strings.Cut(entry, "=")
				if !ok {
					t.Fatalf("malformed build environment: %q", entry)
				}
				key = strings.ToUpper(key)
				counts[key]++
				switch key {
				case "GOMAXPROCS":
					if value != "2" {
						t.Fatalf("unbounded compiler concurrency: %q", value)
					}
				case "GOCACHE":
					if value != "cache" {
						t.Fatal("build lost its isolated cache")
					}
				case "GOFLAGS", "OPENAI_API_KEY":
					t.Fatalf("untrusted parent key propagated: %s", key)
				}
			}
			if counts["GOMAXPROCS"] != 1 || counts["GOCACHE"] != 1 || command.Dir != "source" {
				t.Fatalf("build lost its fixed budget or source: counts=%v dir=%q", counts, command.Dir)
			}
			for _, entry := range outcomeQualityWorkerEnvironmentWithCache("runtime-cache") {
				if strings.HasPrefix(strings.ToUpper(entry), "GOMAXPROCS=") {
					t.Fatal("build-only budget changed the evaluated worker runtime")
				}
			}
		})
	}
}
