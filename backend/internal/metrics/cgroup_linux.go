//go:build linux

package metrics

import (
	"os"
	"strconv"
	"strings"
)

// cgroupMemoryLimit returns the container memory limit in bytes, or 0 when the
// process is not limited (cgroup v2 reports "max").
func cgroupMemoryLimit() uint64 {
	// cgroup v2
	if value, ok := readUintFile("/sys/fs/cgroup/memory.max"); ok {
		return value
	}
	// cgroup v1
	if value, ok := readUintFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); ok {
		// v1 reports a huge sentinel value when unlimited.
		if value > 1<<60 {
			return 0
		}
		return value
	}
	return 0
}

func readUintFile(path string) (uint64, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "max" {
		return 0, false
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	return value, true
}
