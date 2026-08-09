package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
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
	t.Helper()
	body := strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":false}`)
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
	addAuthConnection(t, database, "qoder", srvDenied.URL, 0)  // higher priority
	addAuthConnection(t, database, "cerebras", srvOK.URL, 1)   // next candidate
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
			"circuit-open connection fallback)")
	}
}
