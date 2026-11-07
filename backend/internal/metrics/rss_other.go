//go:build !linux && !windows

package metrics

// readProcessRSS is only implemented on Linux and Windows.
func readProcessRSS() uint64 { return 0 }
