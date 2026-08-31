package outcome

import (
	"encoding/json"
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
)

var benchmarkFailureIntelligenceSink FailureIntelligence

// BenchmarkFailureIntelligence measures the local derived signal only. It
// reports standard ns/op, B/op, and allocs/op with -benchmem plus the encoded
// contract size; it does not claim model, provider, or end-to-end latency.
func BenchmarkFailureIntelligence(b *testing.B) {
	task, err := taskstate.New("failure-benchmark", "repair the failing outcome", nil)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		task.AddVerification("go test ./...", false, "VERIFY FAIL\n--- FAIL: TestSignup (0.00s)")
	}

	b.ReportAllocs()
	b.ResetTimer()
	var got FailureIntelligence
	for i := 0; i < b.N; i++ {
		got = FailureIntelligenceForTask(task)
	}
	b.StopTimer()
	encoded, err := json.Marshal(got)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(len(encoded)), "contract-B/op")
	benchmarkFailureIntelligenceSink = got
}
