package metrics

import (
	"testing"
	"time"
)

func TestConfigureMemoryKeepsExplicitLimit(t *testing.T) {
	t.Setenv("GOMEMLIMIT", "300MiB")
	if got := ConfigureMemory(); got != 0 {
		t.Fatalf("GOMEMLIMIT must win, got configured limit %d", got)
	}
}

func TestConfigureMemoryAppliesBoundedLimit(t *testing.T) {
	t.Setenv("GOMEMLIMIT", "")
	limit := ConfigureMemory()
	if limit < minMemoryLimit || limit > maxMemoryLimit {
		t.Fatalf("limit %d outside [%d, %d]", limit, minMemoryLimit, maxMemoryLimit)
	}
}

func TestBusyTracksInFlightRequests(t *testing.T) {
	before := inFlight.Load()
	BeginRequest()
	if !busy() {
		t.Fatal("in-flight request must count as busy")
	}
	ObserveRequest("GET", "/test", 200, time.Millisecond)
	if inFlight.Load() != before {
		t.Fatalf("in-flight not restored: %d -> %d", before, inFlight.Load())
	}
}

func TestBusyUsesIdleProbe(t *testing.T) {
	SetIdleProbe(func() bool { return true })
	defer SetIdleProbe(func() bool { return false })
	if !busy() {
		t.Fatal("probe reporting work must count as busy")
	}
}
