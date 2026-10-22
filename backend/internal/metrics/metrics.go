package metrics

import (
	"context"
	"fmt"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// SampleInterval is how often runtime gauges and request counters are
	// captured for the admin monitor charts.
	SampleInterval = 10 * time.Second
	// Retention is how much history the in-memory ring keeps.
	Retention = 24 * time.Hour
)

type key struct {
	method string
	route  string
	status int
}

type sample struct {
	at             time.Time
	heapAlloc      uint64
	heapInuse      uint64
	heapSys        uint64
	stackInuse     uint64
	sys            uint64
	pauseTotal     uint64
	goroutines     int
	threads        int
	inFlight       int64
	requests       uint64
	errors         uint64
	latencyNanos   uint64
	gcCount        uint32
	hostCPUPercent float64
	hostMemPercent float64
	hostMemUsedMB  float64
	hostMemTotalMB float64
	hostAvailable  bool
}

// hostStats are host-wide reference values. They are intentionally coarse:
// the monitor page shows them only as context for the process metrics.
type hostStats struct {
	cpuPercent float64
	memPercent float64
	memUsedMB  float64
	memTotalMB float64
	available  bool
}

var (
	hostMu        sync.Mutex
	hostPrevIdle  uint64
	hostPrevTotal uint64
	hostPrevValid bool
)

var (
	startedAt = time.Now()
	mu        sync.Mutex
	requests  = map[key]uint64{}

	totalRequests atomic.Uint64
	errorRequests atomic.Uint64
	inFlight      atomic.Int64
	latencyNanos  atomic.Uint64

	sampleMu   sync.RWMutex
	samples    []sample
	latest     sample
	samplerRun sync.Once
)

// Snapshot intentionally avoids runtime.ReadMemStats: that call stops the
// world and would make every monitor poll expensive for the whole process.
// Values come from the 10s sampler instead.
var snapshotOnce sync.Once

// Current is the live process snapshot shown on the system monitor page.
type Current struct {
	StartedAt             string  `json:"started_at"`
	UptimeSeconds         int64   `json:"uptime_seconds"`
	Goroutines            int     `json:"goroutines"`
	Threads               int     `json:"threads"`
	GOMAXPROCS            int     `json:"gomaxprocs"`
	NumCPU                int     `json:"num_cpu"`
	HeapAllocMB           float64 `json:"heap_alloc_mb"`
	HeapInuseMB           float64 `json:"heap_inuse_mb"`
	HeapSysMB             float64 `json:"heap_sys_mb"`
	StackInuseMB          float64 `json:"stack_inuse_mb"`
	SysMB                 float64 `json:"sys_mb"`
	GCCount               uint32  `json:"gc_count"`
	GCPauseTotalMS        float64 `json:"gc_pause_total_ms"`
	InFlight              int64   `json:"in_flight"`
	RequestsTotal         uint64  `json:"requests_total"`
	ErrorsTotal           uint64  `json:"errors_total"`
	AvgLatencyMS          float64 `json:"avg_latency_ms"`
	HostCPUPercent        float64 `json:"host_cpu_percent"`
	HostMemPercent        float64 `json:"host_mem_percent"`
	HostMemUsedMB         float64 `json:"host_mem_used_mb"`
	HostMemTotalMB        float64 `json:"host_mem_total_mb"`
	HostAvailable         bool    `json:"host_available"`
	SampleIntervalSeconds int     `json:"sample_interval_seconds"`
	RetentionHours        int     `json:"retention_hours"`
}

// Point is one downsampled bucket in the monitor time series.
type Point struct {
	Time           string  `json:"time"`
	HeapAllocMB    float64 `json:"heap_alloc_mb"`
	HeapSysMB      float64 `json:"heap_sys_mb"`
	SysMB          float64 `json:"sys_mb"`
	Goroutines     int     `json:"goroutines"`
	Threads        int     `json:"threads"`
	InFlight       int64   `json:"in_flight"`
	Requests       uint64  `json:"requests"`
	Errors         uint64  `json:"errors"`
	AvgLatencyMS   float64 `json:"avg_latency_ms"`
	GCCount        uint32  `json:"gc_count"`
	HostCPUPercent float64 `json:"host_cpu_percent"`
	HostMemPercent float64 `json:"host_mem_percent"`
}

// BeginRequest marks one HTTP request as in flight.
func BeginRequest() {
	inFlight.Add(1)
}

// ObserveRequest records one completed HTTP request. route should be the
// registered Gin route (not the raw URL) to keep label cardinality bounded.
func ObserveRequest(method, route string, status int, duration time.Duration) {
	if route == "" {
		route = "unmatched"
	}
	k := key{method: method, route: route, status: status}
	mu.Lock()
	requests[k]++
	mu.Unlock()

	totalRequests.Add(1)
	if status >= 400 {
		errorRequests.Add(1)
	}
	if duration > 0 {
		latencyNanos.Add(uint64(duration))
	}
	if n := inFlight.Add(-1); n < 0 {
		inFlight.Store(0)
	}
}

// StartSampler captures an immediate sample and then one every SampleInterval
// until ctx is cancelled.
func StartSampler(ctx context.Context) {
	samplerRun.Do(func() {
		sampleMu.Lock()
		if capacity := int(Retention/SampleInterval) + 16; cap(samples) < capacity {
			samples = make([]sample, 0, capacity)
		}
		sampleMu.Unlock()
		capture()
		go func() {
			ticker := time.NewTicker(SampleInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					capture()
				}
			}
		}()
	})
}

// Snapshot returns live request counters plus the latest sampled memory
// statistics. It never triggers a stop-the-world memory read.
func Snapshot() Current {
	snapshotOnce.Do(func() {
		sampleMu.RLock()
		hasSample := !latest.at.IsZero()
		sampleMu.RUnlock()
		if !hasSample {
			capture()
		}
	})
	sampleMu.RLock()
	sampled := latest
	sampleMu.RUnlock()
	host := hostStats{
		cpuPercent: sampled.hostCPUPercent,
		memPercent: sampled.hostMemPercent,
		memUsedMB:  sampled.hostMemUsedMB,
		memTotalMB: sampled.hostMemTotalMB,
		available:  sampled.hostAvailable,
	}
	total := totalRequests.Load()
	avgLatency := 0.0
	if total > 0 {
		avgLatency = float64(latencyNanos.Load()) / float64(total) / float64(time.Millisecond)
	}
	return Current{
		StartedAt:             startedAt.Format(time.RFC3339),
		UptimeSeconds:         int64(time.Since(startedAt).Seconds()),
		Goroutines:            runtime.NumGoroutine(),
		Threads:               threadCount(),
		GOMAXPROCS:            runtime.GOMAXPROCS(0),
		NumCPU:                runtime.NumCPU(),
		HeapAllocMB:           toMB(sampled.heapAlloc),
		HeapInuseMB:           toMB(sampled.heapInuse),
		HeapSysMB:             toMB(sampled.heapSys),
		StackInuseMB:          toMB(sampled.stackInuse),
		SysMB:                 toMB(sampled.sys),
		GCCount:               sampled.gcCount,
		GCPauseTotalMS:        float64(sampled.pauseTotal) / float64(time.Millisecond),
		InFlight:              inFlight.Load(),
		RequestsTotal:         total,
		ErrorsTotal:           errorRequests.Load(),
		AvgLatencyMS:          round2(avgLatency),
		HostCPUPercent:        host.cpuPercent,
		HostMemPercent:        host.memPercent,
		HostMemUsedMB:         host.memUsedMB,
		HostMemTotalMB:        host.memTotalMB,
		HostAvailable:         host.available,
		SampleIntervalSeconds: int(SampleInterval / time.Second),
		RetentionHours:        int(Retention / time.Hour),
	}
}

// History returns at most maxPoints downsampled buckets starting at since.
func History(since time.Time, maxPoints int) []Point {
	return historySince(since, time.Now(), maxPoints)
}

func historySince(since, now time.Time, maxPoints int) []Point {
	if maxPoints < 2 {
		maxPoints = 2
	}
	// Aggregate while holding the read lock instead of copying up to 24h of
	// samples on every poll; this keeps monitor reads allocation-light.
	sampleMu.RLock()
	defer sampleMu.RUnlock()
	snapshot := samples
	if len(snapshot) == 0 {
		return []Point{}
	}

	// Right after startup there may be much less history than the requested
	// window. Bucket against the available span so the chart still has detail.
	baseSince := since
	if snapshot[0].at.After(baseSince) {
		baseSince = snapshot[0].at
	}
	span := now.Sub(baseSince)
	if span <= 0 {
		span = SampleInterval
	}
	step := span / time.Duration(maxPoints)
	if step < SampleInterval {
		step = SampleInterval
	}

	type bucket struct {
		point        Point
		latencyNanos uint64
	}
	buckets := map[int]*bucket{}
	lastIndex := -1
	for i, current := range snapshot {
		if current.at.Before(since) {
			continue
		}
		index := int(current.at.Sub(baseSince) / step)
		if index >= maxPoints {
			index = maxPoints - 1
		}
		previous := sample{}
		if i > 0 {
			previous = snapshot[i-1]
		}
		item, exists := buckets[index]
		if !exists {
			item = &bucket{point: Point{Time: baseSince.Add(time.Duration(index) * step).Format(time.RFC3339)}}
			buckets[index] = item
		}
		item.point.HeapAllocMB = toMB(current.heapAlloc)
		item.point.HeapSysMB = toMB(current.heapSys)
		item.point.SysMB = toMB(current.sys)
		item.point.Goroutines = current.goroutines
		item.point.Threads = current.threads
		item.point.InFlight = current.inFlight
		item.point.HostCPUPercent = current.hostCPUPercent
		item.point.HostMemPercent = current.hostMemPercent
		if current.requests >= previous.requests {
			item.point.Requests += current.requests - previous.requests
		}
		if current.errors >= previous.errors {
			item.point.Errors += current.errors - previous.errors
		}
		if current.gcCount >= previous.gcCount {
			item.point.GCCount += current.gcCount - previous.gcCount
		}
		if current.latencyNanos >= previous.latencyNanos {
			item.latencyNanos += current.latencyNanos - previous.latencyNanos
		}
		lastIndex = index
	}
	if lastIndex < 0 {
		return []Point{}
	}

	points := make([]Point, 0, lastIndex+1)
	var carry *Point
	for index := 0; index <= lastIndex; index++ {
		item, exists := buckets[index]
		if exists {
			if item.point.Requests > 0 {
				item.point.AvgLatencyMS = round2(float64(item.latencyNanos) / float64(item.point.Requests) / float64(time.Millisecond))
			}
			point := item.point
			points = append(points, point)
			carry = &point
			continue
		}
		if carry == nil {
			continue
		}
		point := Point{
			Time:           baseSince.Add(time.Duration(index) * step).Format(time.RFC3339),
			HeapAllocMB:    carry.HeapAllocMB,
			HeapSysMB:      carry.HeapSysMB,
			SysMB:          carry.SysMB,
			Goroutines:     carry.Goroutines,
			Threads:        carry.Threads,
			InFlight:       carry.InFlight,
			HostCPUPercent: carry.HostCPUPercent,
			HostMemPercent: carry.HostMemPercent,
		}
		points = append(points, point)
	}
	return points
}

func capture() {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	host := sampleHost()
	entry := sample{
		at:             time.Now(),
		heapAlloc:      mem.HeapAlloc,
		heapInuse:      mem.HeapInuse,
		heapSys:        mem.HeapSys,
		stackInuse:     mem.StackInuse,
		sys:            mem.Sys,
		pauseTotal:     mem.PauseTotalNs,
		goroutines:     runtime.NumGoroutine(),
		threads:        threadCount(),
		inFlight:       inFlight.Load(),
		requests:       totalRequests.Load(),
		errors:         errorRequests.Load(),
		latencyNanos:   latencyNanos.Load(),
		gcCount:        mem.NumGC,
		hostCPUPercent: host.cpuPercent,
		hostMemPercent: host.memPercent,
		hostMemUsedMB:  host.memUsedMB,
		hostMemTotalMB: host.memTotalMB,
		hostAvailable:  host.available,
	}
	cutoff := entry.at.Add(-Retention)
	sampleMu.Lock()
	latest = entry
	samples = append(samples, entry)
	trimmed := samples[:0]
	for _, item := range samples {
		if item.at.After(cutoff) {
			trimmed = append(trimmed, item)
		}
	}
	samples = trimmed
	sampleMu.Unlock()
}

// sampleHost computes host-wide CPU and memory usage. CPU needs two readings,
// so it reports 0 on the first call after process start.
func sampleHost() hostStats {
	stats := hostStats{}
	hostMu.Lock()
	defer hostMu.Unlock()
	if idle, total, ok := readCPUTimes(); ok {
		if hostPrevValid && total > hostPrevTotal && idle >= hostPrevIdle {
			totalDelta := total - hostPrevTotal
			idleDelta := idle - hostPrevIdle
			stats.cpuPercent = round2(float64(totalDelta-idleDelta) / float64(totalDelta) * 100)
		}
		hostPrevIdle, hostPrevTotal, hostPrevValid = idle, total, true
	}
	if used, total, ok := readHostMemory(); ok && total > 0 {
		stats.memUsedMB = toMB(used)
		stats.memTotalMB = toMB(total)
		stats.memPercent = round2(float64(used) / float64(total) * 100)
		stats.available = true
	}
	return stats
}

func threadCount() int {
	if profile := pprof.Lookup("threadcreate"); profile != nil {
		return profile.Count()
	}
	return 0
}

func toMB(bytes uint64) float64 {
	return round2(float64(bytes) / (1024 * 1024))
}

func round2(value float64) float64 {
	return float64(int64(value*100+0.5)) / 100
}

// Render writes process and HTTP metrics in Prometheus text exposition format.
func Render(extra ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# HELP smartdoc_build_info Build information.\n")
	fmt.Fprintf(&b, "# TYPE smartdoc_build_info gauge\n")
	mu.Lock()
	snapshot := make([]key, 0, len(requests))
	counts := make(map[key]uint64, len(requests))
	for k, v := range requests {
		snapshot = append(snapshot, k)
		counts[k] = v
	}
	mu.Unlock()
	sort.Slice(snapshot, func(i, j int) bool {
		a, c := snapshot[i], snapshot[j]
		if a.route != c.route {
			return a.route < c.route
		}
		if a.method != c.method {
			return a.method < c.method
		}
		return a.status < c.status
	})
	fmt.Fprintf(&b, "# HELP smartdoc_uptime_seconds Process uptime in seconds.\n")
	fmt.Fprintf(&b, "# TYPE smartdoc_uptime_seconds gauge\n")
	fmt.Fprintf(&b, "smartdoc_uptime_seconds %.0f\n", time.Since(startedAt).Seconds())
	fmt.Fprintf(&b, "# HELP smartdoc_goroutines Current goroutine count.\n")
	fmt.Fprintf(&b, "# TYPE smartdoc_goroutines gauge\n")
	fmt.Fprintf(&b, "smartdoc_goroutines %d\n", runtime.NumGoroutine())
	fmt.Fprintf(&b, "# HELP smartdoc_http_requests_total Completed HTTP requests by route and status.\n")
	fmt.Fprintf(&b, "# TYPE smartdoc_http_requests_total counter\n")
	for _, k := range snapshot {
		fmt.Fprintf(&b, "smartdoc_http_requests_total{method=%q,route=%q,status=\"%d\"} %d\n",
			escape(k.method), escape(k.route), k.status, counts[k])
	}
	for _, line := range extra {
		line = strings.TrimRight(line, "\n")
		if line != "" {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func escape(v string) string {
	v = strings.ReplaceAll(v, `\`, `\`)
	return strings.ReplaceAll(v, `"`, `\"`)
}
