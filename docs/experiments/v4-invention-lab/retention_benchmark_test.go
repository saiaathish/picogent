package inventionlab

import (
	"testing"

	"github.com/saiaathish/picogent/internal/ctxmgr"
	"github.com/saiaathish/picogent/internal/llm"
)

func BenchmarkPrototypeRetention96Turn(b *testing.B) {
	fixture := retentionFixture96()
	for _, test := range []struct {
		name string
		fn   func([]llm.Message, int) []llm.Message
	}{
		{name: "value-aware", fn: ctxmgr.ValueAwareWindow},
		{name: "recency-only", fn: ctxmgr.TruncateTail},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			var retained []llm.Message
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				retained = test.fn(fixture, 10)
			}
			b.StopTimer()
			coverage := 0.0
			if hasTargetPair(retained) {
				coverage = 1
			}
			b.ReportMetric(coverage, "target-coverage/op")
			b.ReportMetric(float64(len(retained)), "messages/op")
		})
	}
}
