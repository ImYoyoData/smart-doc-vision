//go:build windows

package metrics

import (
	"syscall"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

type memoryStatusEx struct {
	length               uint32
	memoryLoad           uint32
	totalPhys            uint64
	availPhys            uint64
	totalPageFile        uint64
	availPageFile        uint64
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

func filetimeValue(value syscall.Filetime) uint64 {
	return uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime)
}

// readCPUTimes returns cumulative idle and total CPU time in 100ns units.
func readCPUTimes() (idle, total uint64, ok bool) {
	var idleTime, kernelTime, userTime syscall.Filetime
	result, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)
	if result == 0 {
		return 0, 0, false
	}
	// Kernel time already includes idle time.
	return filetimeValue(idleTime), filetimeValue(kernelTime) + filetimeValue(userTime), true
}

// readHostMemory returns used and total physical memory.
func readHostMemory() (used, total uint64, ok bool) {
	status := memoryStatusEx{length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	result, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if result == 0 || status.totalPhys == 0 {
		return 0, 0, false
	}
	if status.availPhys > status.totalPhys {
		return 0, status.totalPhys, true
	}
	return status.totalPhys - status.availPhys, status.totalPhys, true
}
