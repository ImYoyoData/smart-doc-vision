//go:build windows

package metrics

import (
	"syscall"
	"unsafe"
)

var (
	psapi                       = syscall.NewLazyDLL("psapi.dll")
	procGetProcessMemoryInfo    = psapi.NewProc("GetProcessMemoryInfo")
	procK32GetProcessMemoryInfo = kernel32.NewProc("K32GetProcessMemoryInfo")
)

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

// readProcessRSS returns the process working set (resident memory), which is
// what Task Manager and container memory counters report. It is the number
// users compare against, whereas runtime.MemStats.Sys only covers Go's own
// mapped memory.
func readProcessRSS() uint64 {
	counters := processMemoryCounters{}
	counters.cb = uint32(unsafe.Sizeof(counters))

	call := procGetProcessMemoryInfo
	if err := call.Find(); err != nil {
		call = procK32GetProcessMemoryInfo
		if err := call.Find(); err != nil {
			return 0
		}
	}
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	result, _, _ := call.Call(uintptr(handle), uintptr(unsafe.Pointer(&counters)), uintptr(counters.cb))
	if result == 0 {
		return 0
	}
	return uint64(counters.workingSetSize)
}
