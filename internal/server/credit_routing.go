package server

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	creditRefreshInterval    = 5 * time.Minute
	creditSnapshotMaxAge     = 15 * time.Minute
	creditRefreshConcurrency = 4
)

// creditSnapshot is the routing-time view of a provider-reported balance.
// It is populated by a background refresher so request handling never calls a
// balance endpoint synchronously.
type creditSnapshot struct {
	Known     bool
	Remaining float64
	ExpiresAt time.Time
	HasExpiry bool
	FetchedAt time.Time
}

// creditCapableHost reports whether the balance fetcher for this host returns
// a numeric credit balance. Providers without credit data keep their existing
// routing order.
func creditCapableHost(baseURL string) bool {
	host := extractHost(baseURL)
	switch {
	case strings.Contains(host, "commandcode"),
		strings.Contains(host, "kilo"),
		strings.Contains(host, "deepseek"),
		strings.Contains(host, "openrouter"):
		return true
	}
	return false
}

func (p *ProxyHandler) startCreditRefreshLoop() {
	// Populate immediately so enabling the policy does not wait for a tick.
	p.refreshCreditSnapshots()
	p.creditMu.RLock()
	loaded := len(p.creditSnapshots)
	p.creditMu.RUnlock()
	if loaded > 0 {
		fmt.Printf("credit-aware routing: %d credit snapshot(s) loaded\n", loaded)
	}
	ticker := time.NewTicker(creditRefreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		p.refreshCreditSnapshots()
	}
}

// refreshCreditSnapshots polls balance APIs for active connections whose
// provider exposes credit data. Failures keep the previous snapshot; a
// snapshot older than creditSnapshotMaxAge is ignored by routing.
func (p *ProxyHandler) refreshCreditSnapshots() {
	p.creditRefreshMu.Lock()
	defer p.creditRefreshMu.Unlock()

	rows, err := p.db.Conn().Query(
		`SELECT id, base_url, api_key, format FROM connections WHERE is_active = 1 AND base_url != ''`,
	)
	if err != nil {
		return
	}
	type target struct {
		id, baseURL, apiKey, format string
	}
	var targets []target
	for rows.Next() {
		var t target
		if rows.Scan(&t.id, &t.baseURL, &t.apiKey, &t.format) != nil {
			continue
		}
		if creditCapableHost(t.baseURL) {
			targets = append(targets, t)
		}
	}
	rows.Close()

	sem := make(chan struct{}, creditRefreshConcurrency)
	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func(t target) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			info := fetchProviderBalance(t.baseURL, t.apiKey, t.format)
			if info.Error != "" || !info.CreditKnown {
				return
			}
			snap := creditSnapshot{
				Known:     true,
				Remaining: info.RemainingNum,
				FetchedAt: time.Now(),
			}
			if info.ExpiresAt != "" {
				if expires, err := time.Parse(time.RFC3339, info.ExpiresAt); err == nil {
					snap.ExpiresAt = expires
					snap.HasExpiry = true
				}
			}
			p.creditMu.Lock()
			if p.creditSnapshots == nil {
				p.creditSnapshots = make(map[string]creditSnapshot)
			}
			p.creditSnapshots[t.id] = snap
			p.creditMu.Unlock()
		}(t)
	}
	wg.Wait()
}

func (p *ProxyHandler) creditSnapshotFor(connectionID string) (creditSnapshot, bool) {
	p.creditMu.RLock()
	defer p.creditMu.RUnlock()
	snapshot, ok := p.creditSnapshots[connectionID]
	return snapshot, ok
}

func (p *ProxyHandler) reloadCreditAwareSetting() {
	enabled := strings.EqualFold(strings.TrimSpace(p.getSetting("credit_aware_routing_enabled", "false")), "true")
	if enabled {
		atomic.StoreInt32(&p.creditAware, 1)
	} else {
		atomic.StoreInt32(&p.creditAware, 0)
	}
}

func creditGroupKey(connection *Connection) string {
	host := extractHost(connection.BaseURL)
	if host == "" {
		return "id:" + connection.ID
	}
	return strings.ToLower(connection.Format) + "|" + host
}

// reorderCandidatesByCredit sorts each contiguous provider group by expiring
// credit:
//
//  0. fresh snapshot, credit > 0, expiry known  → soonest expiry first
//  1. fresh snapshot, credit > 0, no expiry     → existing order
//  2. no/stale snapshot                         → existing order
//  3. fresh snapshot, credit <= 0               → last (last-resort attempt)
//
// Only adjacent connections from the same provider/endpoint are reordered, so
// a combo's cross-provider sequence is never changed.
func (p *ProxyHandler) reorderCandidatesByCredit(candidates []*Connection) []*Connection {
	if len(candidates) < 2 || atomic.LoadInt32(&p.creditAware) == 0 {
		return candidates
	}
	now := time.Now()
	out := append([]*Connection(nil), candidates...)
	for start := 0; start < len(out); {
		end := start + 1
		key := creditGroupKey(out[start])
		for end < len(out) && creditGroupKey(out[end]) == key {
			end++
		}
		if end-start > 1 {
			p.sortCreditGroup(out[start:end], now)
		}
		start = end
	}
	return out
}

func (p *ProxyHandler) sortCreditGroup(group []*Connection, now time.Time) {
	type ranked struct {
		connection *Connection
		class      int
		remaining  float64
		expiresAt  time.Time
		priority   int
	}
	items := make([]ranked, len(group))
	for i, connection := range group {
		class, remaining, expiresAt := p.creditClass(connection.ID, now)
		items[i] = ranked{
			connection: connection,
			class:      class,
			remaining:  remaining,
			expiresAt:  expiresAt,
			priority:   connection.Priority,
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.class != b.class {
			return a.class < b.class
		}
		if a.class == 0 {
			if !a.expiresAt.Equal(b.expiresAt) {
				return a.expiresAt.Before(b.expiresAt)
			}
			if a.remaining != b.remaining {
				return a.remaining < b.remaining
			}
			return a.priority < b.priority
		}
		// Classes 1-3 preserve the operator's existing order.
		return false
	})
	for i := range items {
		group[i] = items[i].connection
	}
}

func (p *ProxyHandler) creditClass(connectionID string, now time.Time) (class int, remaining float64, expiresAt time.Time) {
	snapshot, ok := p.creditSnapshotFor(connectionID)
	if !ok || !snapshot.Known || now.Sub(snapshot.FetchedAt) > creditSnapshotMaxAge {
		return 2, 0, time.Time{}
	}
	if snapshot.Remaining <= 0 {
		return 3, snapshot.Remaining, time.Time{}
	}
	if snapshot.HasExpiry {
		return 0, snapshot.Remaining, snapshot.ExpiresAt
	}
	return 1, snapshot.Remaining, time.Time{}
}
