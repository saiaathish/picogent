//go:build !darwin

package folderpick

// ChooseFiles is only supported on macOS; use the web file picker elsewhere.
func ChooseFiles(_ string) ([]string, error) {
	return nil, ErrCancelled
}
