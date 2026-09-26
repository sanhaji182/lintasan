package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// proxy_auth_failover_test.go — the "stuck here and stops" fix. When an
// upstream answers 401/403 (auth/quota failure), the proxy must try the NEXT
// candidate rather than passing the failure to the client as a dead end.

// --- test scaffolding -------------------------------------------------------

// newAuthProxy builds a ProxyHandler backed by an in-memory DB (the real
// schema comes from db.Open's migrations).
func newAuthProxy(t *testing.T) (*ProxyHandler, *db.DB) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewProxyHandler(&config.Config{}, database), database
}

// addAuthConnection inserts an active connection pointing at a stub upstream.
// Higher priority wins in findConnectionForModel's ORDER BY priority DESC.
func addAuthConnection(t *testing.T, database *db.DB, name, baseURL string, priority int) {
	t.Helper()
	_, err := database.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, chat_path, models_path, auth_header, auth_prefix, is_active, priority)
		VALUES (?, ?, ?, ?, 'openai', '/chat/completions', '/models', 'Authorization', 'Bearer ', 1, ?)`,
		"conn-"+name, name, baseURL, "sk-test", priority)
	if err != nil {
		t.Fatalf("insert connection %s: %v", name, err)
	}
}

// addAuthModel makes a connection serve a model.
func addAuthModel(t *testing.T, database *db.DB, connName, model string) {
	t.Helper()
	_, err := database.Conn().Exec(`
		INSERT INTO discovered_models (connection_id, model_id, is_active)
		VALUES (?, ?, 1)`, "conn-"+connName, model)
	if err != nil {
		t.Fatalf("insert model: %v", err)
	}
}

// doAuthChat drives a non-streaming chat request through the proxy.
func doAuthChat(t *testing.T, p *ProxyHandler) *httptest.ResponseRecorder {
	return doAuthChatModel(t, p, "m", false)
}

func doAuthChatModel(t *testing.T, p *ProxyHandler, model string, stream bool) *httptest.ResponseRecorder {
	t.Helper()
	body := strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, model, stream))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	p.HandleChatCompletions(rec, req)
	return rec
}

// authStub is an upstream that answers every chat request with a fixed status.
type authStub struct {
	status int
	hits   int
}

func (a *authStub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.hits++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(a.status)
		w.Write([]byte(`{"error":{"message":"key rejected"}}`))
	}
}

// okStub answers with a minimal successful chat completion.
func okStub(hits *int) http.HandlerFunc {
	return okStubLogged(hits, nil)
}

// okStubLogged records each request path so tests can tell a real chat retry
// apart from a non-chat probe (e.g. a /models health check hitting the stub).
func okStubLogged(hits *int, paths *[]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*hits++
		if paths != nil {
			*paths = append(*paths, r.Method+" "+r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl-x",
			"model": "m",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]string{"role": "assistant", "content": "hi"}, "finish_reason": "stop"},
			},
			"usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}
}

func TestPinnedComboAuthFailureDoesNotWiden(t *testing.T) {
	denied := &authStub{status: 403}
	srvDenied := httptest.NewServer(denied.handler())
	defer srvDenied.Close()
	var outsiderHits int
	var outsiderPaths []string
	srvOutsider := httptest.NewServer(okStubLogged(&outsiderHits, &outsiderPaths))
	defer srvOutsider.Close()

	h, database := newAuthProxy(t)
	addAuthConnection(t, database, "exact", srvDenied.URL, 100)
	addAuthConnection(t, database, "outsider", srvOutsider.URL, 0)
	addAuthModel(t, database, "exact", "m")
	addAuthModel(t, database, "outsider", "m")
	if err := h.cmb.LoadFromSettings(`[{"name":"pinned-combo","strategy":"priority","entries":[{"model":"m","connection_id":"conn-exact","provider_id":"pool:wrong"}]}]`); err != nil {
		t.Fatal(err)
	}

	rec := doAuthChatModel(t, h, "pinned-combo", false)
	if rec.Code == http.StatusOK {
		t.Fatalf("exact pin widened after auth failure: %s", rec.Body.String())
	}
	for _, path := range outsiderPaths {
		if strings.Contains(path, "/chat/completions") {
			t.Fatalf("outsider received exact-pin chat request: %v", outsiderPaths)
		}
	}
}

func TestProviderComboAuthFailureStaysWithinPool(t *testing.T) {
	var deniedHits, poolHits, outsiderHits int
	var providerPaths []string
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerPaths = append(providerPaths, r.Header.Get("Authorization")+" "+r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") == "Bearer key-denied" {
			deniedHits++
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"message":"key rejected"}}`))
			return
		}
		okStub(&poolHits)(w, r)
	}))
	defer providerServer.Close()
	var outsiderPaths []string
	srvOutsider := httptest.NewServer(okStubLogged(&outsiderHits, &outsiderPaths))
	defer srvOutsider.Close()

	h, database := newAuthProxy(t)
	for _, row := range []struct {
		id, key  string
		priority int
	}{
		{"pool-a", "key-denied", 100}, {"pool-b", "key-ok", 50},
	} {
		_, err := database.Conn().Exec(`INSERT INTO connections (id,name,base_url,api_key,format,chat_path,models_path,auth_header,auth_prefix,is_active,priority) VALUES (?,?,?,?,'openai','/chat/completions','/models','Authorization','Bearer ',1,?)`, "conn-"+row.id, row.id, providerServer.URL, row.key, row.priority)
		if err != nil {
			t.Fatal(err)
		}
	}
	addAuthConnection(t, database, "outsider", srvOutsider.URL, 0)
	for _, name := range []string{"pool-a", "pool-b", "outsider"} {
		addAuthModel(t, database, name, "m")
	}
	providerID := provider.RoutingPoolIdentity("openai", providerServer.URL, "/chat/completions", "")
	if err := h.cmb.LoadFromSettings(fmt.Sprintf(`[{"name":"provider-combo","strategy":"priority","entries":[{"model":"m","provider_id":%q}]}]`, providerID)); err != nil {
		t.Fatal(err)
	}
	resolved, _, ok := h.resolveCombo("provider-combo")
	if !ok || len(resolved) != 2 {
		t.Fatalf("provider combo resolved %d candidates, want 2: %#v", len(resolved), resolved)
	}
	if resolved[0].ID != "conn-pool-a" || resolved[1].ID != "conn-pool-b" || resolved[0].APIKey != "key-denied" || resolved[1].APIKey != "key-ok" {
		t.Fatalf("unexpected provider candidates: %#v", resolved)
	}

	rec := doAuthChatModel(t, h, "provider-combo", false)
	if rec.Code != http.StatusOK || deniedHits == 0 || poolHits == 0 {
		t.Fatalf("same-provider fallback failed: status=%d denied=%d poolHits=%d paths=%v body=%s", rec.Code, deniedHits, poolHits, providerPaths, rec.Body.String())
	}
	for _, path := range outsiderPaths {
		if strings.Contains(path, "/chat/completions") {
			t.Fatalf("outsider received provider-combo chat request: %v", outsiderPaths)
		}
	}
}

func TestProviderComboStreamFailureStaysWithinPool(t *testing.T) {
	var refusedHits, poolHits, outsiderHits int
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if r.Header.Get("Authorization") == "Bearer key-refused" {
			refusedHits++
			_, _ = w.Write([]byte(qoderRoleThenRefusal))
			return
		}
		poolHits++
		_, _ = w.Write([]byte(qoderRoleThenContent))
	}))
	defer providerServer.Close()
	var outsiderPaths []string
	srvOutsider := httptest.NewServer(okStubLogged(&outsiderHits, &outsiderPaths))
	defer srvOutsider.Close()

	h, database := newAuthProxy(t)
	for _, row := range []struct {
		id, key  string
		priority int
	}{{"pool-a", "key-refused", 100}, {"pool-b", "key-ok", 50}} {
		_, err := database.Conn().Exec(`INSERT INTO connections (id,name,base_url,api_key,format,chat_path,models_path,auth_header,auth_prefix,is_active,priority) VALUES (?,?,?,?,'qoder','/chat/completions','/models','Authorization','Bearer ',1,?)`, "conn-"+row.id, row.id, providerServer.URL, row.key, row.priority)
		if err != nil {
			t.Fatal(err)
		}
	}
	addAuthConnection(t, database, "outsider", srvOutsider.URL, 0)
	for _, name := range []string{"pool-a", "pool-b", "outsider"} {
		addAuthModel(t, database, name, "m")
	}
	providerID := provider.RoutingPoolIdentity("qoder", providerServer.URL, "/chat/completions", "")
	if err := h.cmb.LoadFromSettings(fmt.Sprintf(`[{"name":"provider-stream","strategy":"priority","entries":[{"model":"m","provider_id":%q}]}]`, providerID)); err != nil {
		t.Fatal(err)
	}

	rec := doAuthChatModel(t, h, "provider-stream", true)
	if rec.Code != http.StatusOK || refusedHits == 0 || poolHits == 0 || !strings.Contains(rec.Body.String(), "PONG") {
		t.Fatalf("same-provider stream fallback failed: status=%d refused=%d pool=%d body=%s", rec.Code, refusedHits, poolHits, rec.Body.String())
	}
	for _, path := range outsiderPaths {
		if strings.Contains(path, "/chat/completions") {
			t.Fatalf("outsider received stream combo request: %v", outsiderPaths)
		}
	}
}

func configureConnectionFallback(t *testing.T, h *ProxyHandler, database *db.DB, from, to string) {
	t.Helper()
	raw, err := json.Marshal(map[string][]string{from: {to}})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetSetting("fallback_connection_chains", string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := h.fb.LoadChains(); err != nil {
		t.Fatal(err)
	}
}

func openConnectionBreaker(h *ProxyHandler, connectionID string) {
	breaker := h.getBreaker(connectionID)
	breaker.Failure()
	breaker.Failure()
	breaker.Failure()
}

func TestPinnedComboCircuitOpenDoesNotWidenToConnectionFallback(t *testing.T) {
	var pinnedHits, outsiderHits int
	var pinnedPaths []string
	pinned := httptest.NewServer(okStubLogged(&pinnedHits, &pinnedPaths))
	defer pinned.Close()
	var outsiderPaths []string
	outsider := httptest.NewServer(okStubLogged(&outsiderHits, &outsiderPaths))
	defer outsider.Close()

	h, database := newAuthProxy(t)
	addAuthConnection(t, database, "exact", pinned.URL, 100)
	addAuthConnection(t, database, "outsider", outsider.URL, 0)
	addAuthModel(t, database, "exact", "m")
	addAuthModel(t, database, "outsider", "m")
	if err := h.cmb.LoadFromSettings(`[{"name":"pinned-circuit","strategy":"priority","entries":[{"model":"m","connection_id":"conn-exact"}]}]`); err != nil {
		t.Fatal(err)
	}
	configureConnectionFallback(t, h, database, "conn-exact", "conn-outsider")
	openConnectionBreaker(h, "conn-exact")

	rec := doAuthChatModel(t, h, "pinned-circuit", false)
	if rec.Code == http.StatusOK {
		t.Fatalf("exact pin widened through circuit fallback: %s", rec.Body.String())
	}
	for _, path := range pinnedPaths {
		if strings.Contains(path, "/chat/completions") {
			t.Fatalf("open exact connection received a chat request: %v", pinnedPaths)
		}
	}
	for _, path := range outsiderPaths {
		if strings.Contains(path, "/chat/completions") {
			t.Fatalf("outsider received exact-pin circuit fallback request: %v", outsiderPaths)
		}
	}
}

func TestProviderComboCircuitOpenDoesNotWidenToConnectionFallback(t *testing.T) {
	var poolHits, outsiderHits int
	var poolPaths []string
	pool := httptest.NewServer(okStubLogged(&poolHits, &poolPaths))
	defer pool.Close()
	var outsiderPaths []string
	outsider := httptest.NewServer(okStubLogged(&outsiderHits, &outsiderPaths))
	defer outsider.Close()

	h, database := newAuthProxy(t)
	addAuthConnection(t, database, "pool", pool.URL, 100)
	addAuthConnection(t, database, "outsider", outsider.URL, 0)
	addAuthModel(t, database, "pool", "m")
	addAuthModel(t, database, "outsider", "m")
	providerID := provider.RoutingPoolIdentity("openai", pool.URL, "/chat/completions", "")
	if err := h.cmb.LoadFromSettings(fmt.Sprintf(`[{"name":"provider-circuit","strategy":"priority","entries":[{"model":"m","provider_id":%q}]}]`, providerID)); err != nil {
		t.Fatal(err)
	}
	configureConnectionFallback(t, h, database, "conn-pool", "conn-outsider")
	openConnectionBreaker(h, "conn-pool")

	rec := doAuthChatModel(t, h, "provider-circuit", false)
	if rec.Code == http.StatusOK {
		t.Fatalf("provider scope widened through circuit fallback: %s", rec.Body.String())
	}
	for _, path := range poolPaths {
		if strings.Contains(path, "/chat/completions") {
			t.Fatalf("open provider connection received a chat request: %v", poolPaths)
		}
	}
	for _, path := range outsiderPaths {
		if strings.Contains(path, "/chat/completions") {
			t.Fatalf("outsider received provider circuit fallback request: %v", outsiderPaths)
		}
	}
}

// The core behaviour: a 403 from the first provider must not reach the client
// when a second candidate exists — the second provider's success must.
func TestAuthFailureFailsOverToNextCandidate(t *testing.T) {
	denied := &authStub{status: 403}
	srvDenied := httptest.NewServer(denied.handler())
	defer srvDenied.Close()

	var okHits int
	srvOK := httptest.NewServer(okStub(&okHits))
	defer srvOK.Close()

	h, database := newAuthProxy(t)
	defer database.Close()
	addAuthConnection(t, database, "qoder", srvDenied.URL, 100) // higher priority
	addAuthConnection(t, database, "cerebras", srvOK.URL, 0)    // next candidate
	addAuthModel(t, database, "qoder", "m")
	addAuthModel(t, database, "cerebras", "m")

	rec := doAuthChat(t, h)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — the 403 should have failed over, not surfaced\nbody: %s", rec.Code, rec.Body.String())
	}
	if denied.hits == 0 {
		t.Error("denied provider was never tried")
	}
	if okHits == 0 {
		t.Error("failover target was never tried — the request died at the 403")
	}
}

// When EVERY candidate fails with auth errors, the client must still get a
// real status code (not a hanging request or a hollow 502), so the failure is
// visible as "the key was rejected", not silence.
func TestAuthFailureAllExhaustedReturnsRealStatus(t *testing.T) {
	denied := &authStub{status: 403}
	srvDenied := httptest.NewServer(denied.handler())
	defer srvDenied.Close()

	h, database := newAuthProxy(t)
	defer database.Close()
	addAuthConnection(t, database, "qoder", srvDenied.URL, 0)
	addAuthModel(t, database, "qoder", "m")

	rec := doAuthChat(t, h)

	// Single provider, nowhere to fail over to: the proxy reports exhaustion
	// with the provider's real auth status, never a bare "all routes failed"
	// blob that hides the cause.
	if rec.Code == http.StatusOK {
		t.Fatal("expected an error status, got 200")
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Errorf("body should describe the failure, got: %s", rec.Body.String())
	}
}

// A 200 must NOT be treated as failover-worthy — the new branch must only fire
// on 401/403, never on success.
func TestSuccessDoesNotTriggerFailover(t *testing.T) {
	var okHits int
	var paths []string
	srvOK := httptest.NewServer(okStubLogged(&okHits, &paths))
	defer srvOK.Close()

	h, database := newAuthProxy(t)
	defer database.Close()
	addAuthConnection(t, database, "cerebras", srvOK.URL, 0)
	addAuthModel(t, database, "cerebras", "m")

	rec := doAuthChat(t, h)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// Count only real chat calls — a stub also answers any non-chat probe the
	// proxy makes (e.g. a /models health check), and those are not retries.
	chatCalls := 0
	for _, pth := range paths {
		if strings.Contains(pth, "/chat/completions") {
			chatCalls++
		}
	}
	if chatCalls != 1 {
		t.Errorf("chat completions called %d times, want exactly 1 — a 200 must not retry. paths=%v", chatCalls, paths)
	}
}

// The failover works by appending a candidate INSIDE the request loop. That is
// only possible because the loop is an index loop ("for i := 0; i <
// len(candidates); i++"), not "for i, conn := range candidates" — range
// evaluates len once, so an appended candidate would never be reached. Guard
// that invariant by reading the loop header out of the source: if someone
// reverts to the range form, this test fails before the behaviour silently
// does.
func TestCandidateLoopReevaluatesLength(t *testing.T) {
	src, err := os.ReadFile("proxy.go")
	if err != nil {
		t.Fatalf("read proxy.go: %v", err)
	}
	if !strings.Contains(string(src), "for i := 0; i < len(candidates); i++") {
		t.Error("candidate loop must be an index loop over len(candidates); " +
			"a range loop breaks mid-loop candidate appends (auth failover, " +
			"non-combo circuit-open connection fallback)")
	}
}
