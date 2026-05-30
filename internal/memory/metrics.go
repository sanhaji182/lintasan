package memory

import (
	"os"
	"strconv"
	"sync/atomic"
)

// Search observability + safety knobs.
//
// H3 (benchmark-confirmed): semantic similarity search was an unbounded O(n)
// scan over every stored memory on every cache-miss request — 2.5ms @100 rows,
// 25ms @1k, 122ms @5k, 787ms @20k — and it ran even when the store was empty.
// This package now defends that hot path with: (1) an early-exit when the store
// is empty, (2) a hard scan cap so a runaway store can't dominate latency, and
// (3) counters so we can see real usage and decide whether a proper ANN index
// is worth building.

// defaultMaxScanRows caps how many rows/keys a brute-force similarity scan will
// consider. Tunable via LINTASAN_MEMORY_MAX_SCAN (0 or negative = unbounded,
// preserving old behavior for anyone who explicitly opts out).
const defaultMaxScanRows = 2000

// MaxScanRows returns the configured brute-force scan cap.
func MaxScanRows() int {
	if v := os.Getenv("LINTASAN_MEMORY_MAX_SCAN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultMaxScanRows
}

// Package-level counters (atomic, cheap on the hot path).
var (
	searchCalls   atomic.Int64 // times Search was invoked
	searchHits    atomic.Int64 // times Search returned >=1 result
	searchEmpty   atomic.Int64 // times Search early-exited because the store was empty
	searchScanned atomic.Int64 // cumulative rows/keys actually scanned
	searchCapped  atomic.Int64 // times the scan hit MaxScanRows and stopped early
)

// SearchMetrics is a point-in-time snapshot of search counters.
type SearchMetrics struct {
	Calls      int64 `json:"calls"`
	Hits       int64 `json:"hits"`
	EmptyExits int64 `json:"empty_exits"`
	RowsScanned int64 `json:"rows_scanned"`
	CappedScans int64 `json:"capped_scans"`
	MaxScanRows int   `json:"max_scan_rows"`
}

// Metrics returns a snapshot of the search counters for observability endpoints.
func Metrics() SearchMetrics {
	return SearchMetrics{
		Calls:       searchCalls.Load(),
		Hits:        searchHits.Load(),
		EmptyExits:  searchEmpty.Load(),
		RowsScanned: searchScanned.Load(),
		CappedScans: searchCapped.Load(),
		MaxScanRows: MaxScanRows(),
	}
}

func recordSearchCall()         { searchCalls.Add(1) }
func recordSearchHit()          { searchHits.Add(1) }
func recordSearchEmpty()        { searchEmpty.Add(1) }
func recordSearchScanned(n int) { searchScanned.Add(int64(n)) }
func recordSearchCapped()       { searchCapped.Add(1) }
