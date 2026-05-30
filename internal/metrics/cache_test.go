package metrics

import "testing"

// TestCacheCounters verifies the response-cache hit/miss counters increment
// independently and that CacheStats reflects the running totals. These back
// operational question #3 ("What is the cache hit rate?").
func TestCacheCounters(t *testing.T) {
	before := CacheStats()

	CacheHit()
	CacheHit()
	CacheMiss()

	after := CacheStats()

	if got := after.Hits - before.Hits; got != 2 {
		t.Fatalf("CacheHit: expected +2 hits, got +%d", got)
	}
	if got := after.Misses - before.Misses; got != 1 {
		t.Fatalf("CacheMiss: expected +1 miss, got +%d", got)
	}
}
