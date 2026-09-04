package ctxmgr

import (
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
)

// BenchmarkLiveRetentionWindow compares the candidate live compaction window
// with the exact recency-only tail control. target-coverage/op is a structural
// fixture result, not a model-quality claim.
func BenchmarkLiveRetentionWindow(b *testing.B) {
	fixture := liveRetentionFixture()
	for _, tc := range []struct {
		name string
		fn   func([]llm.Message, int) []llm.Message
	}{
		{name: "value-aware", fn: ValueAwareWindow},
		{name: "recency-only", fn: TruncateTail},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			var retained []llm.Message
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				retained = tc.fn(fixture, 10)
			}
			b.StopTimer()
			b.ReportMetric(float64(len(retained)), "messages/op")
			coverage := 0
			if containsContextToolPair(retained, "retention-target") {
				coverage = 1
			}
			b.ReportMetric(float64(coverage), "target-coverage/op")
		})
	}
}
