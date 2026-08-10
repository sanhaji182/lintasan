package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/db"
)

// proxy_pool_auth_failover_test.go — a 401/403 from ONE key in a multi-account
// pool must rotate to the other keys in the SAME pool before the whole
// connection is written off. Before this, a single bad key discarded the entire
// provider (the other keys were never tried) because auth failures are not in
// the retry-status set that drives account rotation.

// addPooledConnection inserts an active connection that belongs to a pool.
// All accounts in a pool share one base_url (one provider, N keys); the key is
// what differs. name doubles as the connection id suffix and the api key so the
// stub upstream can tell which account it was called with.
func addPooledConnection(t *testing.T, database *db.DB, name, baseURL, poolID, apiKey string, priority int) {
	t.Helper()
	_, err := database.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, chat_path, models_path, auth_header, auth_prefix, is_active, priority, pool_id)
		VALUES (?, ?, ?, ?, 'openai', '/chat/completions', '/models', 'Authorization', 'Bearer ', 1, ?, ?)`,
		"conn-"+name, name, baseURL, apiKey, priority, poolID)
	if err != nil {
		t.Fatalf("insert pooled connection %s: %v", name, err)
	}
}

// poolStub answers 403 for keys in badKeys, 200 otherwise, recording every key
// it saw so the test can assert which accounts were tried.
type poolStub struct {
	mu      sync.Mutex
	badKeys map[string]bool
	seen    []string
}

func (s *poolStub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		s.mu.Lock()
		s.seen = append(s.seen, key)
		bad := s.badKeys[key]
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if bad {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":{"message":"quota exhausted"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"c","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}
}

func (s *poolStub) sawKey(k string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.seen {
		if v == k {
			return true
		}
	}
	return false
}

// A pool of 5 keys where 3 are rejected: the request must succeed via one of
// the healthy keys, and the connection must NOT be abandoned.
func TestPoolAuthFailover_RotatesToHealthyKey(t *testing.T) {
	stub := &poolStub{badKeys: map[string]bool{"key-0": true, "key-1": true, "key-2": true}}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	h, database := newAuthProxy(t)
	defer database.Close()

	// 5 accounts, same pool, same upstream, distinct keys. key-0 has the
	// highest priority so it is picked first and fails.
	for i, prio := range []int{0, 1, 2, 3, 4} {
		name := "acct" + string(rune('0'+i))
		addPooledConnection(t, database, name, srv.URL, "pool-x", "key-"+string(rune('0'+i)), prio)
	}
	addAuthModel(t, database, "acct0", "m") // representative connection serves the model
	h.RefreshMultiAccountPools()

	rec := doAuthChat(t, h)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a healthy key in the pool should have served it\nbody: %s", rec.Code, rec.Body.String())
	}
	// A good key (key-3 or key-4) must have been reached.
	if !stub.sawKey("key-3") && !stub.sawKey("key-4") {
		t.Errorf("no healthy key was tried; upstream saw only: %v", stub.seen)
	}
	// The pool-retry header marks that recovery happened within the pool, not
	// via connection failover.
	if rec.Header().Get("X-Lintasan-Pool-Retry") == "" {
		t.Error("expected X-Lintasan-Pool-Retry header on pool recovery")
	}
}

// The rejected key must be taken out of rotation: a second request through the
// same pool must not spend an upstream call on the known-bad key again.
func TestPoolAuthFailover_RejectedKeyRemovedFromRotation(t *testing.T) {
	stub := &poolStub{badKeys: map[string]bool{"key-0": true}}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	h, database := newAuthProxy(t)
	defer database.Close()

	addPooledConnection(t, database, "acct0", srv.URL, "pool-y", "key-0", 0)
	addPooledConnection(t, database, "acct1", srv.URL, "pool-y", "key-1", 1)
	addAuthModel(t, database, "acct0", "m")
	h.RefreshMultiAccountPools()

	// First request: key-0 fails, key-1 serves it.
	if rec := doAuthChat(t, h); rec.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", rec.Code)
	}

	stub.mu.Lock()
	stub.seen = nil // reset the ledger
	stub.mu.Unlock()

	// Second request: key-0 is on auth cooldown, so it must NOT be tried; the
	// pick should go straight to key-1.
	if rec := doAuthChat(t, h); rec.Code != http.StatusOK {
		t.Fatalf("second request status = %d, want 200", rec.Code)
	}
	if stub.sawKey("key-0") {
		t.Errorf("key-0 was rejected earlier and should be on cooldown, but was tried again: %v", stub.seen)
	}
}

// Every key rejected → the pool is exhausted and the request fails over to a
// DIFFERENT connection serving the same model.
func TestPoolAuthFailover_AllRejectedFailsOverConnection(t *testing.T) {
	badStub := &poolStub{badKeys: map[string]bool{"bad-0": true, "bad-1": true}}
	srvBad := httptest.NewServer(badStub.handler())
	defer srvBad.Close()

	var okHits int
	srvOK := httptest.NewServer(okStub(&okHits))
	defer srvOK.Close()

	h, database := newAuthProxy(t)
	defer database.Close()

	// Pool of 2 dead keys (higher priority), plus a separate healthy provider.
	addPooledConnection(t, database, "bad0", srvBad.URL, "pool-z", "bad-0", 0)
	addPooledConnection(t, database, "bad1", srvBad.URL, "pool-z", "bad-1", 1)
	addAuthConnection(t, database, "backup", srvOK.URL, 5) // lower priority, no pool
	addAuthModel(t, database, "bad0", "m")
	addAuthModel(t, database, "backup", "m")
	h.RefreshMultiAccountPools()

	rec := doAuthChat(t, h)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — should have failed over to the backup connection\nbody: %s", rec.Code, rec.Body.String())
	}
	if okHits == 0 {
		t.Error("backup connection was never tried — the request died at the exhausted pool")
	}
}
