//go:build !linux && !windows

package metrics

// Host-wide CPU and memory readings are only implemented for Linux and
// Windows; other platforms simply omit the reference values.
func readCPUTimes() (idle, total uint64, ok bool) {
	return 0, 0, false
}

func readHostMemory() (used, total uint64, ok bool) {
	return 0, 0, false
}
