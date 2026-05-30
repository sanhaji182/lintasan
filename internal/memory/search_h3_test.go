package memory

import (
	"os"
	"path/filepath"
	"testing"
)

// makeEmbedding builds a deterministic unit-ish vector of the right dimension.
func makeEmbedding(seed float64) []float64 {
	e := make([]float64, EmbeddingDim)
	for i := range e {
		e[i] = seed + float64(i%7)*0.01
	}
	return e
}

func resetSearchCounters() {
	searchCalls.Store(0)
	searchHits.Store(0)
	searchEmpty.Store(0)
	searchScanned.Store(0)
	searchCapped.Store(0)
}

// TestSearchEmptyStoreEarlyExit proves H3 fix step (1): searching an empty
// store does not scan and returns quickly with the empty-exit counter bumped.
func TestSearchEmptyStoreEarlyExit(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "mem.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	resetSearchCounters()

	res, err := store.Search(makeEmbedding(0.1), 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("expected 0 results from empty store, got %d", len(res))
	}
	m := Metrics()
	if m.Calls != 1 {
		t.Errorf("expected 1 call, got %d", m.Calls)
	}
	if m.EmptyExits != 1 {
		t.Errorf("expected 1 empty-exit, got %d", m.EmptyExits)
	}
	if m.RowsScanned != 0 {
		t.Errorf("empty store must scan 0 rows, scanned %d", m.RowsScanned)
	}
}

// TestSearchScanCap proves H3 fix steps (2)+(3): the brute-force scan is bounded
// by LINTASAN_MEMORY_MAX_SCAN and the capped counter fires when exceeded.
func TestSearchScanCap(t *testing.T) {
	t.Setenv("LINTASAN_MEMORY_MAX_SCAN", "10")
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "mem.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	// Insert 25 memories — more than the cap of 10.
	for i := 0; i < 25; i++ {
		key := "k" + string(rune('a'+i))
		if err := store.Store(key, makeEmbedding(float64(i)*0.05), "text", nil, nil, 0); err != nil {
			t.Fatalf("Store %d: %v", i, err)
		}
	}

	resetSearchCounters()
	res, err := store.Search(makeEmbedding(0.2), 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("expected results, got none")
	}
	m := Metrics()
	if m.MaxScanRows != 10 {
		t.Errorf("expected cap 10, got %d", m.MaxScanRows)
	}
	if m.RowsScanned > 10 {
		t.Errorf("scan must be capped at 10, scanned %d", m.RowsScanned)
	}
	if m.CappedScans != 1 {
		t.Errorf("expected capped counter to fire once, got %d", m.CappedScans)
	}
	if m.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", m.Hits)
	}
}

// TestSearchUnboundedOptOut proves the escape hatch: LINTASAN_MEMORY_MAX_SCAN<=0
// restores the old unbounded behavior (scans every row, never caps).
func TestSearchUnboundedOptOut(t *testing.T) {
	t.Setenv("LINTASAN_MEMORY_MAX_SCAN", "0")
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "mem.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	for i := 0; i < 30; i++ {
		key := "u" + string(rune('a'+i))
		if err := store.Store(key, makeEmbedding(float64(i)*0.03), "text", nil, nil, 0); err != nil {
			t.Fatalf("Store %d: %v", i, err)
		}
	}

	resetSearchCounters()
	if _, err := store.Search(makeEmbedding(0.3), 5); err != nil {
		t.Fatalf("Search: %v", err)
	}
	m := Metrics()
	if m.RowsScanned != 30 {
		t.Errorf("unbounded mode must scan all 30 rows, scanned %d", m.RowsScanned)
	}
	if m.CappedScans != 0 {
		t.Errorf("unbounded mode must never cap, capped %d", m.CappedScans)
	}
}

func TestMain(m *testing.M) {
	// Ensure a clean env for the cap default in tests that don't set it.
	os.Unsetenv("LINTASAN_MEMORY_MAX_SCAN")
	os.Exit(m.Run())
}
