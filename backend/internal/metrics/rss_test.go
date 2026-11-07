package metrics

import "testing"

func TestReadProcessRSSReportsResidentMemory(t *testing.T) {
	if rss := readProcessRSS(); rss == 0 {
		t.Skip("resident memory is only implemented on Linux and Windows")
	} else if rss < 1<<20 {
		t.Fatalf("resident memory suspiciously small: %d bytes", rss)
	}
}

func TestSnapshotIncludesResidentMemory(t *testing.T) {
	current := Snapshot()
	if current.RSSMB < 0 {
		t.Fatalf("unexpected rss: %+v", current)
	}
}
