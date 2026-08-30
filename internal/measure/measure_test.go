package measure

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestDetectUsesOnlyFixedGoBenchmarkPlan(t *testing.T) {
	if got := Detect(""); got.Runner != "" {
		t.Fatalf("empty workspace plan = %#v", got)
	}
	workspace := t.TempDir()
	if got := Detect(workspace); got.Runner != "" {
		t.Fatalf("non-Go workspace plan = %#v", got)
	}
	writeTestFile(t, workspace+"/go.mod", "module example.test\n\ngo 1.25\n")
	got := Detect(workspace)
	if got.Runner != "go" || len(got.Args) == 0 || strings.Contains(got.Display, "{") {
		t.Fatalf("Go measurement plan = %#v", got)
	}
	if strings.Contains(got.Display, "bash") || !strings.Contains(got.Display, "-bench .") {
		t.Fatalf("measurement plan exposed an unexpected command: %#v", got)
	}
}

func TestParseMetricsKeepsOnlyCanonicalBenchmarkLines(t *testing.T) {
	got := parseMetrics("" +
		"PASS\n" +
		"BenchmarkIgnored 100 1 ns\n" +
		"BenchmarkCache-8 12 4.50 ns/op 8 B/op 1 allocs/op\n" +
		"provider secret=must-not-be-durable\n" +
		"BenchmarkHTTP-8 10 2.0 ms/op\n")
	if len(got) != 2 || !strings.Contains(got[0], "BenchmarkCache-8") || !strings.Contains(got[1], "BenchmarkHTTP-8") {
		t.Fatalf("parsed metrics = %#v", got)
	}
}

func TestStatusFromEvidenceRequiresCanonicalMetricCount(t *testing.T) {
	tests := []struct {
		name string
		text string
		want Status
	}{
		{name: "pass", text: "measure PASS (go test) benchmarks=1\nBenchmarkX-8 1 2 ns/op", want: StatusPass},
		{name: "zero", text: "measure PASS (go test) benchmarks=0", want: StatusInconclusive},
		{name: "lookalike", text: "the measurement PASS benchmarks=9", want: StatusInconclusive},
		{name: "passive", text: "measure PASSIVE (go test) benchmarks=9\nBenchmarkX-8 1 2 ns/op", want: StatusInconclusive},
		{name: "missing metric", text: "measure PASS (go test) benchmarks=1", want: StatusInconclusive},
		{name: "fail", text: "measure FAIL (go test) benchmarks=0 reason=benchmark command failed", want: StatusFail},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := StatusFromEvidence(tc.text); got != tc.want {
				t.Fatalf("StatusFromEvidence(%q) = %s, want %s", tc.text, got, tc.want)
			}
		})
	}
}

func TestUnsupportedMeasurementIsInconclusiveAndBounded(t *testing.T) {
	result := Run(context.Background(), t.TempDir())
	if result.Status != StatusInconclusive || result.Benchmarks != 0 {
		t.Fatalf("unsupported measurement = %#v", result)
	}
	formatted := Format(result)
	if !strings.HasPrefix(formatted, "measure INCONCLUSIVE") || len(formatted) > 2048 {
		t.Fatalf("formatted unsupported measurement = %q", formatted)
	}
}

func TestRunExecutesFixedGoBenchmarkAndFiltersOutput(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, workspace+"/go.mod", "module example.test\n\ngo 1.25\n")
	writeTestFile(t, workspace+"/bench_test.go", `package example

import "testing"

func BenchmarkHotPath(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = i * 2
	}
}

func TestNeverRuns(t *testing.T) {
	t.Fatal("test should be excluded by the fixed benchmark command")
}
`)

	result := Run(context.Background(), workspace)
	if result.Status != StatusPass || result.Benchmarks == 0 || len(result.Metrics) == 0 {
		t.Fatalf("fixed Go benchmark = %#v", result)
	}
	formatted := Format(result)
	if !strings.Contains(formatted, "measure PASS") || !strings.Contains(formatted, "BenchmarkHotPath") {
		t.Fatalf("formatted benchmark = %q", formatted)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
