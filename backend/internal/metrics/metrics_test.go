package metrics

import (
	"testing"
	"time"
)

func TestHistorySinceAggregatesAndFillsGaps(t *testing.T) {
	sampleMu.Lock()
	original := samples
	sampleMu.Unlock()
	defer func() {
		sampleMu.Lock()
		samples = original
		sampleMu.Unlock()
	}()

	now := time.Now()
	since := now.Add(-30 * time.Second)
	latencyStep := uint64(5 * time.Millisecond)
	sampleMu.Lock()
	samples = []sample{
		{
			at: since.Add(5 * time.Second), heapAlloc: 100 << 20, heapSys: 200 << 20, sys: 300 << 20,
			goroutines: 10, threads: 4, inFlight: 1, requests: 100,
		},
		{
			at: since.Add(15 * time.Second), heapAlloc: 120 << 20, heapSys: 220 << 20, sys: 320 << 20,
			goroutines: 12, threads: 5, inFlight: 3, requests: 110, errors: 2,
			latencyNanos: 10 * latencyStep,
		},
		{
			at: since.Add(25 * time.Second), heapAlloc: 110 << 20, heapSys: 210 << 20, sys: 310 << 20,
			goroutines: 11, threads: 5, inFlight: 2, requests: 120, errors: 2,
			latencyNanos: 20 * latencyStep,
		},
	}
	sampleMu.Unlock()

	points := historySince(since, now, 6)
	if len(points) != 3 {
		t.Fatalf("expected 3 buckets, got %d: %+v", len(points), points)
	}
	if points[1].Requests != 10 || points[1].Errors != 2 {
		t.Fatalf("unexpected second bucket counters: %+v", points[1])
	}
	if points[1].AvgLatencyMS != 5 {
		t.Fatalf("expected avg latency 5ms, got %v", points[1].AvgLatencyMS)
	}
	if points[1].Goroutines != 12 || points[1].HeapAllocMB != 120 {
		t.Fatalf("unexpected second bucket gauges: %+v", points[1])
	}
	if points[2].Requests != 10 || points[2].HeapAllocMB != 110 {
		t.Fatalf("unexpected third bucket: %+v", points[2])
	}
}

func TestHistoryFillsMissingBucketsWithPreviousGauges(t *testing.T) {
	sampleMu.Lock()
	original := samples
	sampleMu.Unlock()
	defer func() {
		sampleMu.Lock()
		samples = original
		sampleMu.Unlock()
	}()

	now := time.Now()
	since := now.Add(-30 * time.Second)
	sampleMu.Lock()
	samples = []sample{
		{at: since.Add(5 * time.Second), heapAlloc: 80 << 20, goroutines: 7, requests: 5},
		{at: since.Add(25 * time.Second), heapAlloc: 90 << 20, goroutines: 9, requests: 8},
	}
	sampleMu.Unlock()

	points := historySince(since, now, 6)
	if len(points) != 3 {
		t.Fatalf("expected gap-filled 3 buckets, got %d: %+v", len(points), points)
	}
	if points[1].Goroutines != 7 || points[1].HeapAllocMB != 80 || points[1].Requests != 0 {
		t.Fatalf("missing bucket should carry gauges with zero counters: %+v", points[1])
	}
}

func TestObserveRequestTracksTotals(t *testing.T) {
	before := totalRequests.Load()
	BeginRequest()
	if inFlight.Load() < 1 {
		t.Fatalf("in-flight should increase")
	}
	ObserveRequest("GET", "/api/test", 500, 12*time.Millisecond)
	if got := totalRequests.Load(); got != before+1 {
		t.Fatalf("request total not incremented: %d -> %d", before, got)
	}
	if errorRequests.Load() == 0 {
		t.Fatalf("error total should include status 500")
	}
}

func TestSnapshotReportsRuntime(t *testing.T) {
	current := Snapshot()
	if current.Goroutines <= 0 || current.NumCPU <= 0 {
		t.Fatalf("unexpected runtime snapshot: %+v", current)
	}
	if current.SampleIntervalSeconds != int(SampleInterval/time.Second) {
		t.Fatalf("unexpected sample interval: %+v", current)
	}
}

func TestSnapshotUsesSampledMemory(t *testing.T) {
	sampleMu.Lock()
	originalSamples, originalLatest := samples, latest
	sampleMu.Unlock()
	defer func() {
		sampleMu.Lock()
		samples, latest = originalSamples, originalLatest
		sampleMu.Unlock()
	}()

	entry := sample{
		at: time.Now(), heapAlloc: 64 << 20, heapInuse: 48 << 20, heapSys: 128 << 20,
		stackInuse: 2 << 20, sys: 256 << 20, gcCount: 7,
		pauseTotal: uint64(3 * time.Millisecond),
	}
	sampleMu.Lock()
	latest = entry
	samples = []sample{entry}
	sampleMu.Unlock()

	current := Snapshot()
	if current.HeapAllocMB != 64 || current.HeapSysMB != 128 || current.SysMB != 256 {
		t.Fatalf("snapshot must use sampled memory instead of re-reading: %+v", current)
	}
	if current.GCCount != 7 || current.GCPauseTotalMS != 3 {
		t.Fatalf("snapshot must use sampled GC stats: %+v", current)
	}
}
