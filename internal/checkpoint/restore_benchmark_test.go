package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkRestoreSingleFile times only the sealed checkpoint restore. Capture
// and seal setup stay outside the timer so the result isolates restore I/O.
func BenchmarkRestoreSingleFile(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "note.txt")
	before := []byte("before")
	after := []byte("after")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := os.WriteFile(path, before, 0o644); err != nil {
			b.Fatal(err)
		}
		cp, err := Capture(root, []string{"note.txt"})
		if err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, after, 0o644); err != nil {
			b.Fatal(err)
		}
		if err := cp.Seal(); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if _, err := cp.Restore(); err != nil {
			b.Fatal(err)
		}
	}
}
