package lb

import (
	"sync"
	"testing"
)

func makeConns(ids []string, priorities []int, weights []int, actives []bool) []Connection {
	conns := make([]Connection, len(ids))
	for i := range ids {
		weight := 1
		if i < len(weights) {
			weight = weights[i]
		}
		active := true
		if i < len(actives) {
			active = actives[i]
		}
		priority := 0
		if i < len(priorities) {
			priority = priorities[i]
		}
		conns[i] = Connection{
			ID:       ids[i],
			Priority: priority,
			Weight:   weight,
			Active:   active,
		}
	}
	return conns
}

func TestPriorityPick(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b", "c"},
		[]int{1, 3, 2},
		nil,
		nil,
	)
	lb := New(Priority, conns)

	conn, err := lb.Pick()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.ID != "b" {
		t.Errorf("expected 'b' (priority 3), got '%s'", conn.ID)
	}
}

func TestPriorityPickAllSame(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b", "c"},
		[]int{0, 0, 0},
		nil,
		nil,
	)
	lb := New(Priority, conns)

	conn, err := lb.Pick()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.ID != "a" {
		t.Errorf("expected 'a' (first when all same priority), got '%s'", conn.ID)
	}
}

func TestRoundRobinCycle(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b", "c"},
		nil, nil, nil,
	)
	lb := New(RoundRobin, conns)

	expected := []string{"a", "b", "c", "a", "b", "c"}
	for i, exp := range expected {
		conn, err := lb.Pick()
		if err != nil {
			t.Fatalf("pick %d: unexpected error: %v", i, err)
		}
		if conn.ID != exp {
			t.Errorf("pick %d: expected '%s', got '%s'", i, exp, conn.ID)
		}
	}
}

func TestRoundRobinSkipsInactive(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b", "c"},
		nil, nil,
		[]bool{true, false, true},
	)
	lb := New(RoundRobin, conns)

	// Should only cycle between a and c
	expected := []string{"a", "c", "a", "c"}
	for i, exp := range expected {
		conn, err := lb.Pick()
		if err != nil {
			t.Fatalf("pick %d: unexpected error: %v", i, err)
		}
		if conn.ID != exp {
			t.Errorf("pick %d: expected '%s', got '%s'", i, exp, conn.ID)
		}
	}
}

func TestLeastLatencyPicksLowest(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b", "c"},
		nil, nil, nil,
	)
	lb := New(LeastLatency, conns)

	// Set different latencies
	lb.RecordLatency("a", 100)
	lb.RecordLatency("b", 50)
	lb.RecordLatency("c", 200)

	conn, err := lb.Pick()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.ID != "b" {
		t.Errorf("expected 'b' (lowest latency 50ms), got '%s'", conn.ID)
	}
}

func TestLeastLatencyEWMA(t *testing.T) {
	conns := makeConns(
		[]string{"a"},
		nil, nil, nil,
	)
	lb := New(LeastLatency, conns)

	lb.RecordLatency("a", 100)
	lb.RecordLatency("a", 50)

	// EWMA: alpha=0.3
	// First: ewma = 100
	// Second: ewma = 0.3*50 + 0.7*100 = 15 + 70 = 85
	lb.mu.RLock()
	ewma := lb.latencies["a"].ewma
	lb.mu.RUnlock()

	if ewma < 84.9 || ewma > 85.1 {
		t.Errorf("expected EWMA ~85 (got %f)", ewma)
	}
}

func TestWeightedDistribution(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b"},
		nil,
		[]int{3, 1},
		nil,
	)
	lb := New(Weighted, conns)

	counts := map[string]int{"a": 0, "b": 0}
	const N = 1000
	for i := 0; i < N; i++ {
		conn, err := lb.Pick()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[conn.ID]++
	}

	ratio := float64(counts["a"]) / float64(counts["b"])
	// Expected: 3:1 -> ratio ~3.0. Allow tolerance.
	if ratio < 2.0 || ratio > 4.5 {
		t.Errorf("expected ~3.0 ratio (3:1), got %f (a=%d, b=%d)", ratio, counts["a"], counts["b"])
	}
}

func TestRandomAllActive(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b", "c"},
		nil, nil, nil,
	)
	lb := New(Random, conns)

	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		conn, err := lb.Pick()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		seen[conn.ID] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected all 3 connections to be picked, got %d: %v", len(seen), seen)
	}
}

func TestNoActiveConnections(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b"},
		nil, nil,
		[]bool{false, false},
	)
	lb := New(Priority, conns)

	_, err := lb.Pick()
	if err == nil {
		t.Fatal("expected error for no active connections")
	}
}

func TestSetStrategy(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b", "c"},
		[]int{1, 3, 2},
		nil, nil,
	)
	lb := New(Priority, conns)

	// Start with Priority
	conn, _ := lb.Pick()
	if conn.ID != "b" {
		t.Errorf("priority: expected 'b', got '%s'", conn.ID)
	}

	// Hot-swap to RoundRobin
	// Note: rrCounter may have been modified by previous picks or other operations.
	// We test that RR cycles through all active connections.
	lb.SetStrategy(RoundRobin)
	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		conn, err := lb.Pick()
		if err != nil {
			t.Fatalf("round-robin pick %d: unexpected error: %v", i, err)
		}
		seen[conn.ID] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected all 3 connections, got %v", seen)
	}

	// Hot-swap to Random
	lb.SetStrategy(Random)
	conn, err := lb.Pick()
	if err != nil {
		t.Fatalf("random after hot-swap: unexpected error: %v", err)
	}
	if conn.ID == "" {
		t.Error("random pick returned empty ID")
	}
}

func TestUpdateConnections(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b"},
		nil, nil, nil,
	)
	lb := New(Priority, conns)

	// Record latency for 'a'
	lb.RecordLatency("a", 100)

	// Update connections (replace b with c)
	newConns := []Connection{
		{ID: "a", Priority: 0, Weight: 1, Active: true},
		{ID: "c", Priority: 5, Weight: 1, Active: true},
	}
	lb.UpdateConnections(newConns)

	// Latency data for 'a' should be preserved
	lb.mu.RLock()
	if l, ok := lb.latencies["a"]; !ok || l.ewma != 100 {
		t.Errorf("expected ewma 100 for 'a', got %v", l)
	}
	if _, ok := lb.latencies["c"]; !ok {
		t.Error("expected latency tracker for 'c' to exist")
	}
	lb.mu.RUnlock()

	// Priority pick should now return 'c'
	lb.SetStrategy(Priority)
	conn, err := lb.Pick()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.ID != "c" {
		t.Errorf("expected 'c' (priority 5), got '%s'", conn.ID)
	}
}

func TestConcurrentRoundRobin(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b", "c"},
		nil, nil, nil,
	)
	lb := New(RoundRobin, conns)

	var wg sync.WaitGroup
	counts := make([]int, 3)
	var mu sync.Mutex

	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				conn, err := lb.Pick()
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				mu.Lock()
				switch conn.ID {
				case "a":
					counts[0]++
				case "b":
					counts[1]++
				case "c":
					counts[2]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Each should be ~100. Allow tolerance.
	for i, id := range []string{"a", "b", "c"} {
		if counts[i] < 85 || counts[i] > 115 {
			t.Errorf("connection %s: expected ~100 picks, got %d", id, counts[i])
		}
	}
}

func TestRecordLatency(t *testing.T) {
	conns := makeConns(
		[]string{"a"},
		nil, nil, nil,
	)
	lb := New(Priority, conns)

	lb.RecordLatency("a", 42)

	lb.mu.RLock()
	tracker := lb.latencies["a"]
	lb.mu.RUnlock()

	if tracker.ewma != 42 {
		t.Errorf("expected ewma 42, got %f", tracker.ewma)
	}
	if tracker.count != 1 {
		t.Errorf("expected count 1, got %d", tracker.count)
	}

	lb.RecordLatency("a", 58)
	// EWMA: 0.3*58 + 0.7*42 = 17.4 + 29.4 = 46.8
	lb.mu.RLock()
	ewma := lb.latencies["a"].ewma
	count := lb.latencies["a"].count
	lb.mu.RUnlock()

	if ewma < 46.7 || ewma > 46.9 {
		t.Errorf("expected ewma ~46.8, got %f", ewma)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestRecordLatencyUnknownConn(t *testing.T) {
	conns := makeConns(
		[]string{"a"},
		nil, nil, nil,
	)
	lb := New(Priority, conns)

	// Should not panic for unknown connection
	lb.RecordLatency("nonexistent", 100)
	// Just checking no panic
}

func TestWeightedZeroWeightDefaults(t *testing.T) {
	conns := []Connection{
		{ID: "a", Weight: 0, Active: true},
		{ID: "b", Weight: 0, Active: true},
	}
	lb := New(Weighted, conns)

	conn, err := lb.Pick()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.ID == "" {
		t.Error("got empty ID")
	}
}

func TestLeastLatencyNoData(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b"},
		nil, nil, nil,
	)
	lb := New(LeastLatency, conns)

	conn, err := lb.Pick()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All have 0 latency, should pick first active
	if conn.ID != "a" {
		t.Errorf("expected 'a' (all zero latency, first active), got '%s'", conn.ID)
	}
}

func TestSetStrategyMultipleTimes(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b"},
		nil, nil, nil,
	)
	lb := New(Priority, conns)

	strategies := []Strategy{Priority, RoundRobin, LeastLatency, Weighted, Random}
	for _, s := range strategies {
		lb.SetStrategy(s)
		conn, err := lb.Pick()
		if err != nil {
			t.Fatalf("strategy %s: unexpected error: %v", s, err)
		}
		if conn.ID == "" {
			t.Errorf("strategy %s: got empty ID", s)
		}
	}
}

func TestConcurrentRecordLatency(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b", "c"},
		nil, nil, nil,
	)
	lb := New(LeastLatency, conns)

	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(id string, ms float64) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				lb.RecordLatency(id, ms)
			}
		}([]string{"a", "b", "c"}[g%3], float64(g*10+10))
	}
	wg.Wait()

	// All trackers should have count >= 50 for at least one of them
	for _, id := range []string{"a", "b", "c"} {
		lb.mu.RLock()
		tracker := lb.latencies[id]
		lb.mu.RUnlock()
		if tracker.count == 0 {
			t.Errorf("tracker for %s has count 0", id)
		}
	}
}

func TestRandomDoesNotAlwaysReturnSame(t *testing.T) {
	conns := makeConns(
		[]string{"a", "b"},
		nil, nil, nil,
	)
	lb := New(Random, conns)

	first, _ := lb.Pick()
	different := false
	for i := 0; i < 20; i++ {
		conn, _ := lb.Pick()
		if conn.ID != first.ID {
			different = true
			break
		}
	}
	if !different {
		t.Error("random strategy always returned the same connection in 20 picks")
	}
}

func TestUpdateConnectionsThenPick(t *testing.T) {
	conns := makeConns(
		[]string{"a"},
		nil, nil, nil,
	)
	lb := New(RoundRobin, conns)

	// Add b and c
	newConns := []Connection{
		{ID: "a", Active: true, Weight: 1},
		{ID: "b", Active: true, Weight: 1},
		{ID: "c", Active: true, Weight: 1},
	}
	lb.UpdateConnections(newConns)

	// Should now cycle through all 3
	seen := map[string]int{}
	for i := 0; i < 6; i++ {
		conn, _ := lb.Pick()
		seen[conn.ID]++
	}
	if len(seen) != 3 {
		t.Errorf("expected all 3 connections, got %v", seen)
	}
}
