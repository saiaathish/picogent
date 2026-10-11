package due

import "time"

// IsDue reports whether now is later than the deadline.
func IsDue(now, deadline time.Time) bool {
	return now.After(deadline)
}
