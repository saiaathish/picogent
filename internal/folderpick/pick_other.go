//go:build !darwin && !linux && !windows

package folderpick

import "errors"

func Choose(string) (string, error) {
	return "", errors.New("native folder picker not supported on this platform")
}
