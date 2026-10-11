package due

import (
	"testing"
	"time"
)

func TestIsDueBoundary(t *testing.T) {
	deadline := time.Date(2031, time.February, 3, 12, 0, 0, 0, time.UTC)
	minusOneNanosecond := deadline.Add(-time.Nanosecond)
	plusOneNanosecond := deadline.Add(time.Nanosecond)
	equivalentOffset := time.Date(2031, time.February, 3, 7, 0, 0, 0, time.FixedZone("UTC-5", -5*60*60))

	tests := []struct {
		name     string
		now      time.Time
		deadline time.Time
		want     bool
	}{
		{name: "before", now: minusOneNanosecond, deadline: deadline, want: false},
		{name: "exactly due", now: deadline, deadline: deadline, want: true},
		{name: "after", now: plusOneNanosecond, deadline: deadline, want: true},
		{name: "equal instant in another zone", now: equivalentOffset, deadline: deadline, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsDue(test.now, test.deadline); got != test.want {
				t.Fatalf("IsDue(%s, %s) = %t, want %t", test.now, test.deadline, got, test.want)
			}
		})
	}
}
