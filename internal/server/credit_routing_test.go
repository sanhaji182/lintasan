package server

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func creditTestConn(id string, priority int) *Connection {
	return &Connection{ID: id, BaseURL: "https://api.commandcode.ai", Format: "commandcode", Priority: priority}
}

func connectionIDs(connections []*Connection) []string {
	ids := make([]string, len(connections))
	for i, connection := range connections {
		ids[i] = connection.ID
	}
	return ids
}

func equalIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestReorderCandidatesByExpiringCredit(t *testing.T) {
	p := newTestProxyHandler(t)
	atomic.StoreInt32(&p.creditAware, 1)
	now := time.Now()
	p.creditSnapshots = map[string]creditSnapshot{
		"c1": {Known: true, Remaining: 10, ExpiresAt: now.Add(30 * 24 * time.Hour), HasExpiry: true, FetchedAt: now},
		"c2": {Known: true, Remaining: 0.09, ExpiresAt: now.Add(24 * time.Hour), HasExpiry: true, FetchedAt: now},
		"c3": {Known: true, Remaining: 3.49, ExpiresAt: now.Add(15 * 24 * time.Hour), HasExpiry: true, FetchedAt: now},
		"c4": {Known: true, Remaining: 0, ExpiresAt: now.Add(20 * 24 * time.Hour), HasExpiry: true, FetchedAt: now},
		// c5 has no snapshot and keeps its place ahead of exhausted credit.
	}
	input := []*Connection{
		creditTestConn("c1", 1),
		creditTestConn("c2", 2),
		creditTestConn("c3", 3),
		creditTestConn("c4", 4),
		creditTestConn("c5", 5),
	}
	got := connectionIDs(p.reorderCandidatesByCredit(input))
	want := []string{"c2", "c3", "c1", "c5", "c4"}
	if !equalIDs(got, want) {
		t.Fatalf("credit order = %v, want %v", got, want)
	}
}

func TestCreditOrderUsesSmallestBalanceForSameExpiry(t *testing.T) {
	p := newTestProxyHandler(t)
	atomic.StoreInt32(&p.creditAware, 1)
	now := time.Now()
	expiry := now.Add(10 * 24 * time.Hour)
	p.creditSnapshots = map[string]creditSnapshot{
		"big":   {Known: true, Remaining: 9, ExpiresAt: expiry, HasExpiry: true, FetchedAt: now},
		"small": {Known: true, Remaining: 1, ExpiresAt: expiry, HasExpiry: true, FetchedAt: now},
	}
	got := connectionIDs(p.reorderCandidatesByCredit([]*Connection{
		creditTestConn("big", 1),
		creditTestConn("small", 2),
	}))
	if !equalIDs(got, []string{"small", "big"}) {
		t.Fatalf("same-expiry order = %v, want [small big]", got)
	}
}

func TestCreditOrderDisabledAndStalePreserveOrder(t *testing.T) {
	now := time.Now()
	input := []*Connection{
		creditTestConn("a", 1),
		creditTestConn("b", 2),
		creditTestConn("c", 3),
	}
	want := []string{"a", "b", "c"}

	p := newTestProxyHandler(t)
	atomic.StoreInt32(&p.creditAware, 0)
	p.creditSnapshots = map[string]creditSnapshot{
		"b": {Known: true, Remaining: 1, ExpiresAt: now.Add(time.Hour), HasExpiry: true, FetchedAt: now},
	}
	if got := connectionIDs(p.reorderCandidatesByCredit(input)); !equalIDs(got, want) {
		t.Fatalf("disabled order = %v, want %v", got, want)
	}

	atomic.StoreInt32(&p.creditAware, 1)
	p.creditSnapshots["b"] = creditSnapshot{Known: true, Remaining: 1, ExpiresAt: now.Add(time.Hour), HasExpiry: true, FetchedAt: now.Add(-creditSnapshotMaxAge - time.Minute)}
	if got := connectionIDs(p.reorderCandidatesByCredit(input)); !equalIDs(got, want) {
		t.Fatalf("stale order = %v, want %v", got, want)
	}
}

func TestCreditOrderOnlySortsAdjacentProviderRuns(t *testing.T) {
	p := newTestProxyHandler(t)
	atomic.StoreInt32(&p.creditAware, 1)
	now := time.Now()
	p.creditSnapshots = map[string]creditSnapshot{
		"a1": {Known: true, Remaining: 5, ExpiresAt: now.Add(20 * 24 * time.Hour), HasExpiry: true, FetchedAt: now},
		"a2": {Known: true, Remaining: 5, ExpiresAt: now.Add(2 * 24 * time.Hour), HasExpiry: true, FetchedAt: now},
	}
	other := &Connection{ID: "b1", BaseURL: "https://other.example/v1", Format: "openai", Priority: 1}
	input := []*Connection{creditTestConn("a1", 1), other, creditTestConn("a2", 2)}
	got := connectionIDs(p.reorderCandidatesByCredit(input))
	if !equalIDs(got, []string{"a1", "b1", "a2"}) {
		t.Fatalf("non-adjacent groups must keep their sequence, got %v", got)
	}
}

func TestCreditAwareSettingPersistsAndReloads(t *testing.T) {
	s := newRESTTestServer(t)
	rec := httptest.NewRecorder()
	s.handleSmartRoutingAction(rec, reqWithPath("POST", "/api/smart-routing", map[string]any{
		"credit_aware_routing_enabled": true,
	}, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("save setting: got %d body=%s", rec.Code, rec.Body.String())
	}
	if atomic.LoadInt32(&s.proxy.creditAware) != 1 {
		t.Fatal("proxy credit-aware flag was not reloaded")
	}

	getRec := httptest.NewRecorder()
	s.handleSmartRouting(getRec, reqWithPath("GET", "/api/smart-routing", nil, nil))
	data := asMap(decodeBody(t, getRec)["data"])
	if data["credit_aware_routing_enabled"] != true {
		t.Fatalf("GET credit_aware_routing_enabled = %v, want true", data["credit_aware_routing_enabled"])
	}
}
