//go:build !linux

package metrics

// cgroupMemoryLimit only applies to Linux containers.
func cgroupMemoryLimit() uint64 { return 0 }
