//go:build linux

package metrics

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// readCPUTimes returns cumulative idle and total CPU jiffies from /proc/stat.
func readCPUTimes() (idle, total uint64, ok bool) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		for index := 1; index < len(fields); index++ {
			value, err := strconv.ParseUint(fields[index], 10, 64)
			if err != nil {
				return 0, 0, false
			}
			total += value
			if index == 4 || index == 5 {
				idle += value
			}
		}
		return idle, total, true
	}
	return 0, 0, false
}

// readHostMemory returns used and total physical memory from /proc/meminfo.
func readHostMemory() (used, total uint64, ok bool) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	defer file.Close()
	values := map[string]uint64{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		if key != "MemTotal" && key != "MemAvailable" && key != "MemFree" && key != "Buffers" && key != "Cached" {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		values[key] = value * 1024
	}
	total = values["MemTotal"]
	if total == 0 {
		return 0, 0, false
	}
	available := values["MemAvailable"]
	if available == 0 {
		available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	if available > total {
		available = total
	}
	return total - available, total, true
}
