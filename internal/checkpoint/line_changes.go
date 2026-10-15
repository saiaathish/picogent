package checkpoint

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// LineChange describes the net text change from the captured state to Seal.
// Known is false for binary files or comparisons above the bounded work budget.
type LineChange struct {
	Path           string
	Added, Removed int
	Known          bool
}

// LineChanges uses only captured and sealed bytes, never repository-wide diffs.
func (c *Checkpoint) LineChanges() []LineChange {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.sealed {
		return nil
	}
	out := make([]LineChange, 0, len(c.entries))
	for _, entry := range c.entries {
		if entry.before.sum != entry.expected {
			out = append(out, entry.lines)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func countLineChange(path string, before, after []byte) LineChange {
	change := LineChange{Path: filepath.ToSlash(path)}
	// Keep UI bookkeeping cheap on small machines. Never turn unknown into zero.
	if len(before)+len(after) > 4<<20 || !utf8.Valid(before) || !utf8.Valid(after) || bytes.IndexByte(before, 0) >= 0 || bytes.IndexByte(after, 0) >= 0 {
		return change
	}
	lines := func(data []byte) []string {
		if len(data) == 0 {
			return nil
		}
		result := strings.SplitAfter(string(data), "\n")
		if result[len(result)-1] == "" {
			result = result[:len(result)-1]
		}
		return result
	}
	a, b := lines(before), lines(after)
	oldCount, newCount := len(a), len(b)
	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		a = a[1:]
		b = b[1:]
	}
	for len(a) > 0 && len(b) > 0 && a[len(a)-1] == b[len(b)-1] {
		a = a[:len(a)-1]
		b = b[:len(b)-1]
	}
	if len(a) > 0 && len(b) > 2_000_000/len(a) {
		return change
	}
	// Two-row longest common subsequence: bounded memory and exact line totals.
	if len(b) > len(a) {
		a, b = b, a
	}
	previous, current := make([]int, len(b)+1), make([]int, len(b)+1)
	for _, line := range a {
		for j, other := range b {
			if line == other {
				current[j+1] = previous[j] + 1
			} else {
				current[j+1] = max(previous[j+1], current[j])
			}
		}
		previous, current = current, previous
		current[0] = 0
	}
	common := previous[len(b)]
	// a and b may have been swapped to minimize scratch memory.
	// The matching prefix/suffix removed above contributes equally to both sides.
	trimmed := oldCount + newCount - len(a) - len(b)
	unchanged := trimmed/2 + common
	change.Added = newCount - unchanged
	change.Removed = oldCount - unchanged
	change.Known = true
	return change
}
