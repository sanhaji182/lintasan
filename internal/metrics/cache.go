package metrics

import "sync/atomic"

// Response/semantic-cache hit-rate counters (operational question #3:
// "What is the cache hit rate?"). These track the PROXY response cache —
// the exact-hash and semantic caches checked in proxy.go before a request
// falls through to an upstream provider. They are DISTINCT from the H3
// semantic-SEARCH counters in internal/memory (which count memory-context
// similarity scans, not response caching).
//
// Counters live in this package directly: proxy/server already import
// metrics, so there is no import cycle and no bridge collector is needed.
// Atomic and cheap on the hot path.
var (
	cacheHits   atomic.Int64 // exact + semantic response-cache hits
	cacheMisses atomic.Int64 // cache-eligible requests that fell through to upstream
)

// CacheStatsSnapshot is a point-in-time snapshot of the response-cache counters.
type CacheStatsSnapshot struct {
	Hits   int64
	Misses int64
}

// CacheHit records one response-cache hit (exact or semantic).
func CacheHit() { cacheHits.Add(1) }

// CacheMiss records one cache-eligible request that missed and went upstream.
// Call ONCE per request, only when caching was actually attempted — a request
// that bypassed caching entirely (direct mode, all caches disabled) is not a
// miss and must not inflate the denominator.
func CacheMiss() { cacheMisses.Add(1) }

// CacheStats returns a snapshot of the response-cache counters for the
// /metrics collector.
func CacheStats() CacheStatsSnapshot {
	return CacheStatsSnapshot{
		Hits:   cacheHits.Load(),
		Misses: cacheMisses.Load(),
	}
}
