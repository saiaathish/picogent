//go:build windows

package session

import (
	"errors"
	"time"

	"golang.org/x/sys/windows"
)

func replaceFile(src, dst string) error {
	// Windows readers may briefly keep the destination open without delete
	// sharing. Retry only those transient rename failures; all other errors
	// remain fail-fast and the source stays available for cleanup.
	var lastErr error
	for attempt := 0; attempt < 100; attempt++ {
		err := windows.MoveFileEx(
			windows.StringToUTF16Ptr(src),
			windows.StringToUTF16Ptr(dst),
			windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
		)
		if err == nil {
			return nil
		}
		lastErr = err
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return err
		}
		time.Sleep(time.Millisecond)
	}
	return lastErr
}
