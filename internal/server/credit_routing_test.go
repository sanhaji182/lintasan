package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/lb"
	"github.com/sanhaji182/lintasan-go/internal/quota"
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

func TestIsCreditExhaustion(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   bool
	}{
		{http.StatusPaymentRequired, "", true},
		{http.StatusBadRequest, `{"error":"insufficient credits"}`, true},
		{http.StatusForbidden, "insufficient balance", true},
		{http.StatusTooManyRequests, "rate limit exceeded", false},
		{http.StatusBadRequest, "invalid model", false},
		{http.StatusUnauthorized, "unauthorized", false},
	}
	for _, tc := range cases {
		if got := isCreditExhaustion(tc.status, []byte(tc.body)); got != tc.want {
			t.Errorf("isCreditExhaustion(%d, %q) = %v, want %v", tc.status, tc.body, got, tc.want)
		}
	}
}

func TestMarkCreditExhaustedOverridesPositiveSnapshot(t *testing.T) {
	p := newTestProxyHandler(t)
	now := time.Now()
	p.creditSnapshots = map[string]creditSnapshot{
		"c1": {Known: true, Remaining: 10, ExpiresAt: now.Add(time.Hour), HasExpiry: true, FetchedAt: now},
	}
	p.markCreditExhausted("c1")
	class, remaining, _ := p.creditClass("c1", time.Now())
	if class != 3 || remaining != 0 {
		t.Fatalf("exhausted class = %d remaining = %v, want class 3 and remaining 0", class, remaining)
	}
}

func TestCreditClassPenalizesOpenCircuit(t *testing.T) {
	p := newTestProxyHandler(t)
	now := time.Now()
	p.creditSnapshots = map[string]creditSnapshot{
		"c1": {Known: true, Remaining: 10, ExpiresAt: now.Add(time.Hour), HasExpiry: true, FetchedAt: now},
	}
	if class, _, _ := p.creditClass("c1", now); class != 0 {
		t.Fatalf("initial class = %d, want 0", class)
	}
	breaker := p.getBreaker("c1")
	for i := 0; i < 3; i++ {
		breaker.Failure()
	}
	if class, _, _ := p.creditClass("c1", now); class != 4 {
		t.Fatalf("open-circuit class = %d, want 4", class)
	}
	breaker.Reset()
	if class, _, _ := p.creditClass("c1", now); class != 0 {
		t.Fatalf("recovered class = %d, want 0", class)
	}
}

func TestCreditClassPenalizesExhaustedQuota(t *testing.T) {
	p := newTestProxyHandler(t)
	now := time.Now()
	p.creditSnapshots = map[string]creditSnapshot{
		"c2": {Known: true, Remaining: 10, ExpiresAt: now.Add(time.Hour), HasExpiry: true, FetchedAt: now},
	}
	p.quota.ReplaceLimits(map[string]*quota.QuotaLimit{
		"c2": {MaxTokensPerDay: 100},
	})
	quota.InitQuotaSchema(p.db.Conn())
	quota.RecordQuota(p.db.Conn(), "c2", 101)
	if class, _, _ := p.creditClass("c2", now); class != 4 {
		t.Fatalf("exhausted-quota class = %d, want 4", class)
	}
}

func TestApplyCreditRankersToPools(t *testing.T) {
	p := newTestProxyHandler(t)
	now := time.Now()
	p.creditSnapshots = map[string]creditSnapshot{
		"a1": {Known: true, Remaining: 5, ExpiresAt: now.Add(20 * 24 * time.Hour), HasExpiry: true, FetchedAt: now},
		"a2": {Known: true, Remaining: 1, ExpiresAt: now.Add(2 * 24 * time.Hour), HasExpiry: true, FetchedAt: now},
	}
	p.multiAccountLBs = map[string]*lb.MultiAccountLB{
		"pool": lb.NewMultiAccountLB("pool", []lb.Account{
			{ID: "a1", APIKey: "key-1", Priority: 1, Active: true},
			{ID: "a2", APIKey: "key-2", Priority: 2, Active: true},
		}),
	}
	atomic.StoreInt32(&p.creditAware, 1)
	p.applyCreditRankers()
	for i := 0; i < 3; i++ {
		account, err := p.multiAccountLBs["pool"].Pick()
		if err != nil {
			t.Fatalf("pool Pick %d: %v", i, err)
		}
		if account.ID != "a2" {
			t.Fatalf("pool pick %d = %s, want soonest-expiring a2", i, account.ID)
		}
	}

	atomic.StoreInt32(&p.creditAware, 0)
	p.applyCreditRankers()
	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		account, err := p.multiAccountLBs["pool"].Pick()
		if err != nil {
			t.Fatalf("disabled pool Pick %d: %v", i, err)
		}
		seen[account.ID]++
	}
	if seen["a1"] != 2 || seen["a2"] != 2 {
		t.Fatalf("disabled pool rotation = %v, want 2 picks each", seen)
	}
}

func TestResolveRouteExpandsSameProviderAccountsWhenCreditAware(t *testing.T) {
	p := newTestProxyHandler(t)
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("cc-%d", i)
		if _, err := p.db.Conn().Exec(
			`INSERT INTO connections (id, name, base_url, api_key, format, is_active, priority) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, id, "https://api.commandcode.ai", "sk-test-"+id, "commandcode", 1, i,
		); err != nil {
			t.Fatalf("insert connection %s: %v", id, err)
		}
		if _, err := p.db.Conn().Exec(
			`INSERT INTO discovered_models (id, connection_id, model_id, is_active) VALUES (?, ?, ?, ?)`,
			"model-"+id, id, "shared-model", 1,
		); err != nil {
			t.Fatalf("insert model for %s: %v", id, err)
		}
	}
	now := time.Now()
	p.creditSnapshots = map[string]creditSnapshot{
		"cc-1": {Known: true, Remaining: 10, ExpiresAt: now.Add(30 * 24 * time.Hour), HasExpiry: true, FetchedAt: now},
		"cc-2": {Known: true, Remaining: 1, ExpiresAt: now.Add(2 * 24 * time.Hour), HasExpiry: true, FetchedAt: now},
		"cc-3": {Known: true, Remaining: 3, ExpiresAt: now.Add(15 * 24 * time.Hour), HasExpiry: true, FetchedAt: now},
		"cc-4": {Known: true, Remaining: 0, ExpiresAt: now.Add(24 * time.Hour), HasExpiry: true, FetchedAt: now},
		// cc-5 has no snapshot and keeps its place ahead of exhausted credit.
	}
	atomic.StoreInt32(&p.creditAware, 1)
	candidates, resolved, combo, err := p.resolveRoute("shared-model")
	if err != nil {
		t.Fatalf("resolveRoute: %v", err)
	}
	if resolved != "shared-model" || combo != "" {
		t.Fatalf("resolved=%q combo=%q", resolved, combo)
	}
	if len(candidates) != 5 {
		t.Fatalf("credit-aware candidates = %d, want all 5 provider accounts", len(candidates))
	}
	got := connectionIDs(p.reorderCandidatesByCredit(candidates))
	want := []string{"cc-2", "cc-3", "cc-1", "cc-5", "cc-4"}
	if !equalIDs(got, want) {
		t.Fatalf("credit-aware route order = %v, want %v", got, want)
	}

	atomic.StoreInt32(&p.creditAware, 0)
	legacy, _, _, err := p.resolveRoute("shared-model")
	if err != nil {
		t.Fatalf("legacy resolveRoute: %v", err)
	}
	if len(legacy) != 1 {
		t.Fatalf("legacy candidates = %d, want 1 (LIMIT 1 behavior)", len(legacy))
	}
}

func TestShouldHedgeDisabledForCreditAwareGroup(t *testing.T) {
	p := newTestProxyHandler(t)
	now := time.Now()
	p.creditSnapshots = map[string]creditSnapshot{
		"h1": {Known: true, Remaining: 10, ExpiresAt: now.Add(time.Hour), HasExpiry: true, FetchedAt: now},
		"h2": {Known: true, Remaining: 5, ExpiresAt: now.Add(2 * time.Hour), HasExpiry: true, FetchedAt: now},
	}
	candidates := []*Connection{creditTestConn("h1", 1), creditTestConn("h2", 2)}

	atomic.StoreInt32(&p.creditAware, 1)
	if p.shouldHedge(false, false, candidates) {
		t.Fatal("hedging must be disabled for a live credit-aware group")
	}

	atomic.StoreInt32(&p.creditAware, 0)
	if !p.shouldHedge(false, false, candidates) {
		t.Fatal("hedging should use its default when credit-aware routing is off")
	}

	atomic.StoreInt32(&p.creditAware, 1)
	p.creditSnapshots["h1"] = creditSnapshot{Known: true, Remaining: 10, ExpiresAt: now.Add(time.Hour), HasExpiry: true, FetchedAt: now.Add(-creditSnapshotMaxAge - time.Minute)}
	p.creditSnapshots["h2"] = creditSnapshot{Known: true, Remaining: 5, ExpiresAt: now.Add(2 * time.Hour), HasExpiry: true, FetchedAt: now.Add(-creditSnapshotMaxAge - time.Minute)}
	if !p.shouldHedge(false, false, candidates) {
		t.Fatal("stale credit snapshots must not disable hedging")
	}
}

func TestMergeCreditSnapshotKeepsDepletionUntilTopUp(t *testing.T) {
	now := time.Now()
	depleted := creditSnapshot{
		Known: true, Remaining: 0, DepletedAt: now.Add(-5 * time.Minute),
		DepletedRemaining: 0.09, FetchedAt: now.Add(-5 * time.Minute),
	}
	unchanged := creditSnapshot{Known: true, Remaining: 0.09, FetchedAt: now}
	if got := mergeCreditSnapshot(depleted, unchanged, now); got.Remaining != 0 || got.DepletedAt.IsZero() {
		t.Fatalf("small unchanged balance should stay depleted, got %+v", got)
	}

	toppedUp := creditSnapshot{Known: true, Remaining: 5, FetchedAt: now}
	if got := mergeCreditSnapshot(depleted, toppedUp, now); got.Remaining != 5 || !got.DepletedAt.IsZero() {
		t.Fatalf("top-up should clear depletion, got %+v", got)
	}

	expiredCooldown := depleted
	expiredCooldown.DepletedAt = now.Add(-creditDepletionCooldown - time.Minute)
	if got := mergeCreditSnapshot(expiredCooldown, unchanged, now); got.Remaining != 0.09 {
		t.Fatalf("cooldown should allow one retry, got %+v", got)
	}
}

func TestMarkCreditExhaustedKeepsOriginalDepletedBalance(t *testing.T) {
	p := newTestProxyHandler(t)
	now := time.Now()
	p.creditSnapshots = map[string]creditSnapshot{
		"c1": {Known: true, Remaining: 0.09, ExpiresAt: now.Add(time.Hour), HasExpiry: true, FetchedAt: now},
	}
	p.markCreditExhausted("c1")
	first := p.creditSnapshots["c1"]
	if first.DepletedRemaining != 0.09 {
		t.Fatalf("depleted balance = %v, want 0.09", first.DepletedRemaining)
	}
	p.creditSnapshots["c1"] = mergeCreditSnapshot(first, creditSnapshot{Known: true, Remaining: 0.09, ExpiresAt: now.Add(time.Hour), HasExpiry: true, FetchedAt: now}, now)
	p.markCreditExhausted("c1")
	second := p.creditSnapshots["c1"]
	if second.DepletedRemaining != 0.09 || second.Remaining != 0 {
		t.Fatalf("repeat depletion must preserve the original balance, got %+v", second)
	}
}
