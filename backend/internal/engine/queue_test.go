package engine

import "testing"

func TestProjectSchedulerRoundRobinAndLimits(t *testing.T) {
	e := &Engine{
		pending:    map[uint64][]uint64{1: {101, 102, 103}, 2: {201}},
		projectOrd: []uint64{1, 2},
		limits:     map[uint64]int{1: 1, 2: 1},
		active:     map[uint64]int{},
		wake:       make(chan struct{}, 1),
	}
	e.pendingN = 4
	first, ok := e.nextEntry()
	if !ok || first.projectID != 1 || first.taskID != 101 {
		t.Fatalf("first entry = %+v ok=%v", first, ok)
	}
	second, ok := e.nextEntry()
	if !ok || second.projectID != 2 || second.taskID != 201 {
		t.Fatalf("second entry = %+v ok=%v", second, ok)
	}
	if _, ok := e.nextEntry(); ok {
		t.Fatal("project 1 must be blocked by its concurrency limit")
	}
	e.releaseSlot(1)
	third, ok := e.nextEntry()
	if !ok || third.projectID != 1 || third.taskID != 102 {
		t.Fatalf("third entry = %+v ok=%v", third, ok)
	}
	if stats := e.QueueStats(); len(stats) != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}
