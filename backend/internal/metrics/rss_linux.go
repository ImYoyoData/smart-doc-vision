//go:build linux

package metrics

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// readProcessRSS returns the resident set size of this process in bytes.
func readProcessRSS() uint64 {
	file, err := os.Open("/proc/self/statm")
	if err != nil {
		return 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return pages * uint64(os.Getpagesize())
}
