//go:build windows

package agent

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var processMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

// processMemoryCounters matches PROCESS_MEMORY_COUNTERS from psapi.h. SIZE_T
// fields are uintptr so this remains correct on both 32-bit and 64-bit
// Windows runners.
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

func processResidentBytes(pid int) (int64, error) {
	if pid <= 0 {
		return 0, fmt.Errorf("invalid process id %d", pid)
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0, fmt.Errorf("open process %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	counters := processMemoryCounters{cb: uint32(unsafe.Sizeof(processMemoryCounters{}))}
	result, _, callErr := processMemoryInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&counters)),
		uintptr(counters.cb),
	)
	if result == 0 {
		if callErr != nil {
			return 0, fmt.Errorf("read process %d working set: %w", pid, callErr)
		}
		return 0, fmt.Errorf("read process %d working set failed", pid)
	}
	if counters.workingSetSize == 0 {
		return 0, fmt.Errorf("process %d returned an empty working set", pid)
	}
	return int64(counters.workingSetSize), nil
}

func processResidentSource() string {
	return "psapi.dll GetProcessMemoryInfo WorkingSetSize"
}
