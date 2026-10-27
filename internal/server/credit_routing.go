package server

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/circuit"
	"github.com/sanhaji182/lintasan-go/internal/lb"
)

const (
	creditRefreshInterval    = 5 * time.Minute
	creditSnapshotMaxAge     = 15 * time.Minute
	creditRefreshConcurrency = 4
	creditDepletionCooldown  = 30 * time.Minute
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

	// DepletedAt marks an upstream "insufficient credit" rejection. The mark
	// survives refreshes while the reported balance has not increased, because
	// the balance API can still show a small positive amount that cannot cover
	// a request. A top-up (or the cooldown) clears it.
	DepletedAt        time.Time
	DepletedRemaining float64
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
			if os.Getenv("LINTASAN_DEBUG_CREDIT") == "1" {
				fmt.Printf("credit fetch %s: error=%q known=%v remaining=%.6f expires=%s\n", t.id, info.Error, info.CreditKnown, info.RemainingNum, info.ExpiresAt)
			}
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
			if previous, ok := p.creditSnapshots[t.id]; ok {
				snap = mergeCreditSnapshot(previous, snap, snap.FetchedAt)
			}
			p.creditSnapshots[t.id] = snap
			p.creditMu.Unlock()
		}(t)
	}
	wg.Wait()
}

// mergeCreditSnapshot keeps an upstream credit rejection sticky until the
// provider reports a higher balance (top-up) or the cooldown expires.
func mergeCreditSnapshot(previous, incoming creditSnapshot, now time.Time) creditSnapshot {
	if previous.DepletedAt.IsZero() || now.Sub(previous.DepletedAt) >= creditDepletionCooldown {
		return incoming
	}
	if incoming.Remaining > previous.DepletedRemaining+0.01 {
		return incoming
	}
	incoming.Remaining = 0
	incoming.DepletedAt = previous.DepletedAt
	incoming.DepletedRemaining = previous.DepletedRemaining
	return incoming
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
	p.applyCreditRankers()
}

// applyCreditRankers installs or clears the pool-level ranker. Safe to call
// with no pools; NewProxyHandler calls it before initMultiAccountPools.
func (p *ProxyHandler) applyCreditRankers() {
	p.mabMu.RLock()
	defer p.mabMu.RUnlock()
	p.applyCreditRankersLocked()
}

// applyCreditRankersLocked must be called with mabMu held.
func (p *ProxyHandler) applyCreditRankersLocked() {
	enabled := atomic.LoadInt32(&p.creditAware) == 1
	for _, mab := range p.multiAccountLBs {
		if enabled {
			mab.SetCreditRanker(p.creditRankForAccount)
		} else {
			mab.SetCreditRanker(nil)
		}
	}
}

func (p *ProxyHandler) creditRankForAccount(accountID string) lb.CreditRank {
	class, remaining, expiresAt := p.creditClass(accountID, time.Now())
	return lb.CreditRank{Class: class, ExpiresAt: expiresAt, Remaining: remaining}
}

// noteCreditExhaustion marks an account as depleted immediately and schedules
// a debounced refresh, so a stale 5-minute snapshot cannot keep selecting it.
func (p *ProxyHandler) noteCreditExhaustion(connectionID string) {
	p.markCreditExhausted(connectionID)
	p.requestCreditRefresh()
}

func (p *ProxyHandler) markCreditExhausted(connectionID string) {
	if connectionID == "" {
		return
	}
	p.creditMu.Lock()
	if p.creditSnapshots == nil {
		p.creditSnapshots = make(map[string]creditSnapshot)
	}
	snapshot, ok := p.creditSnapshots[connectionID]
	if !ok {
		snapshot = creditSnapshot{Known: true}
	}
	now := time.Now()
	if snapshot.DepletedAt.IsZero() {
		snapshot.DepletedRemaining = snapshot.Remaining
		snapshot.DepletedAt = now
	}
	snapshot.Known = true
	snapshot.Remaining = 0
	snapshot.FetchedAt = now
	p.creditSnapshots[connectionID] = snapshot
	p.creditMu.Unlock()
}

func (p *ProxyHandler) requestCreditRefresh() {
	if !atomic.CompareAndSwapInt32(&p.creditRefreshRequested, 0, 1) {
		return
	}
	go func() {
		time.Sleep(2 * time.Second)
		p.refreshCreditSnapshots()
		atomic.StoreInt32(&p.creditRefreshRequested, 0)
	}()
}

func creditAccountID(conn *Connection, poolAccountID string) string {
	if poolAccountID != "" {
		return poolAccountID
	}
	if conn != nil {
		return conn.ID
	}
	return ""
}

// isCreditExhaustion recognises a depleted-credit upstream response. 402 is
// unambiguous; other statuses need a credit-specific phrase so a generic
// rate-limit or auth error is never mistaken for an empty balance.
func isRequestSpecific4xx(statusCode int) bool {
	switch statusCode {
	case http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusNotAcceptable,
		http.StatusConflict,
		http.StatusGone,
		http.StatusRequestEntityTooLarge,
		http.StatusRequestURITooLong,
		http.StatusUnsupportedMediaType,
		http.StatusUnprocessableEntity:
		return true
	}
	return false
}

func isCreditExhaustion(statusCode int, body []byte) bool {
	if statusCode == http.StatusPaymentRequired {
		return true
	}
	text := strings.ToLower(string(body))
	for _, phrase := range []string{
		"insufficient credit",
		"insufficient balance",
		"out of credits",
		"no credits remaining",
		"exceeded your credit",
		"credit balance",
		"payment required",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func creditGroupKey(connection *Connection) string {
	host := extractHost(connection.BaseURL)
	if host == "" {
		return "id:" + connection.ID
	}
	return strings.ToLower(connection.Format) + "|" + host
}

func sameCreditGroup(connections []*Connection) bool {
	if len(connections) < 2 {
		return true
	}
	key := creditGroupKey(connections[0])
	for _, connection := range connections[1:] {
		if creditGroupKey(connection) != key {
			return false
		}
	}
	return true
}

func (p *ProxyHandler) creditAwareEnabled() bool {
	return atomic.LoadInt32(&p.creditAware) == 1
}

// creditAwareGroupHasData reports whether this candidate group is ordered by
// live credit data. Hedging is unsafe for such a group: it can race two
// accounts and surface a depleted account's 4xx instead of failing over.
func (p *ProxyHandler) creditAwareGroupHasData(candidates []*Connection) bool {
	if len(candidates) < 2 || !sameCreditGroup(candidates) {
		return false
	}
	now := time.Now()
	for _, connection := range candidates {
		snapshot, ok := p.creditSnapshotFor(connection.ID)
		if ok && snapshot.Known && now.Sub(snapshot.FetchedAt) <= creditSnapshotMaxAge {
			return true
		}
	}
	return false
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
	debug := os.Getenv("LINTASAN_DEBUG_CREDIT") == "1"
	var before []string
	if debug {
		before = creditDebugIDs(out)
	}
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
	if debug {
		fmt.Printf("credit order: %v -> %v\n", before, creditDebugIDs(out))
	}
	return out
}

func creditDebugIDs(connections []*Connection) []string {
	ids := make([]string, len(connections))
	for i, connection := range connections {
		ids[i] = connection.ID
	}
	return ids
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
	// An open circuit or an exhausted token quota is worse than an empty
	// credit balance: the request is guaranteed to fail or be skipped.
	if breaker := p.getBreaker(connectionID); breaker.State() == circuit.StateOpen {
		return 4, 0, time.Time{}
	}
	if p.quotaRemainingFraction(connectionID) <= 0 {
		return 4, 0, time.Time{}
	}

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
