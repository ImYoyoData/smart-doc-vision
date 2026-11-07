package metrics

import (
	"context"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"
)

// Memory tuning keeps the process lean while idle and lets it grow when work
// arrives:
//
//   - A soft memory limit (GOMEMLIMIT) makes the GC scale its effort with the
//     live heap, so a burst can use the free RAM but a mostly empty process
//     does not hold a large heap.
//   - An idle reclaimer returns memory to the OS (debug.FreeOSMemory) once the
//     request pipeline and audit queue have been quiet for a while.
const (
	// minMemoryLimit avoids setting a limit so low that a spike would thrash.
	minMemoryLimit int64 = 192 << 20
	// maxMemoryLimit keeps small hosts from reserving too much on big machines.
	maxMemoryLimit int64 = 1 << 30
	// memoryLimitRatio uses 1/5 of physical RAM when it can be read.
	memoryLimitRatio int64 = 5
	// idleReclaimAfter is how long the process must be idle before memory is
	// returned to the OS.
	idleReclaimAfter = 4 * time.Minute
	// idleHeapFloor skips the reclaim when the idle heap is already small.
	idleHeapFloor uint64 = 48 << 20
	// idleCheckInterval is the reclaimer tick.
	idleCheckInterval = time.Minute
)

// idleProbe reports whether application-level work (audit queue, script runs)
// is still running. HTTP in-flight requests are always counted.
var idleProbe atomic.Value

// SetIdleProbe registers an optional extra "is the service busy" signal.
func SetIdleProbe(fn func() bool) {
	if fn == nil {
		return
	}
	idleProbe.Store(fn)
}

func busy() bool {
	if inFlight.Load() > 0 {
		return true
	}
	if fn, ok := idleProbe.Load().(func() bool); ok && fn != nil {
		return fn()
	}
	return false
}

// ConfigureMemory applies a soft memory limit. Inside a container the limit
// follows the cgroup quota (85% of it); otherwise it uses 1/5 of physical RAM
// so a small host is not over-committed. An explicit GOMEMLIMIT in the
// environment always wins. It returns the limit in bytes, or 0 when the
// operator already configured one.
func ConfigureMemory() int64 {
	if strings.TrimSpace(os.Getenv("GOMEMLIMIT")) != "" {
		return 0
	}
	limit := maxMemoryLimit
	if container := int64(cgroupMemoryLimit()); container > 0 {
		limit = container * 85 / 100
	} else if _, total, ok := readHostMemory(); ok && total > 0 {
		limit = int64(total) / memoryLimitRatio
	}
	if limit < minMemoryLimit {
		limit = minMemoryLimit
	}
	if limit > maxMemoryLimit {
		limit = maxMemoryLimit
	}
	debug.SetMemoryLimit(limit)
	return limit
}

// StartIdleReclaimer periodically returns unused heap memory to the OS. It is
// a no-op while requests or audit tasks are running, so peak throughput keeps
// its warm heap.
func StartIdleReclaimer(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(idleCheckInterval)
		defer ticker.Stop()
		var idleSince time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if busy() {
					idleSince = time.Time{}
					continue
				}
				if idleSince.IsZero() {
					idleSince = now
					continue
				}
				if now.Sub(idleSince) < idleReclaimAfter {
					continue
				}
				sampleMu.RLock()
				entry := latest
				sampleMu.RUnlock()
				if entry.heapIdle < idleHeapFloor {
					continue
				}
				// FreeOSMemory runs a full GC and hands free spans back to the OS.
				// Restart the timer so it does not run on every tick.
				debug.FreeOSMemory()
				idleSince = now
				log.Printf("metrics: idle for %s, released heap back to the OS", idleReclaimAfter)
			}
		}
	}()
}
