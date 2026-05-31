package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// F2.5 server-layer parity tests.
//
// These prove the embeddings handler emits a byte-identical upstream request
// whether embedder_sdk_enabled is ON (build via the provider Embedder) or OFF
// (the inline legacy path). They test the request-BUILD seam directly
// (buildEmbeddingsViaSDK vs an inline mirror) rather than driving the full
// HandleEmbeddings handler through findConnectionForModel — which would depend
// on seeding a :memory: DB and is subject to database/sql connection-pool
// non-determinism (INSERT and SELECT can land on different pooled in-memory
// connections). This mirrors the robust direct-call pattern already used by the
// F1 parity test (runDoUpstream/openAICompatConn), keeping the test
// deterministic and order-independent.

// newEmbeddingsHandler builds a ProxyHandler with embedder_sdk_enabled=embOn.
// The flag is read once at construction (initProviderSDK), so the setting is
// written before NewProxyHandler. No connection seeding is needed because the
// tests call buildEmbeddingsViaSDK directly with an in-code *Connection.
func newEmbeddingsHandler(t *testing.T, embOn bool) *ProxyHandler {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if embOn {
		if err := database.SetSetting("embedder_sdk_enabled", "true"); err != nil {
			t.Fatalf("set flag: %v", err)
		}
	}
	return NewProxyHandler(&config.Config{}, database)
}

// buildEmbeddingsInline replicates the legacy inline path in HandleEmbeddings
// EXACTLY (the flag-OFF branch), so the SDK path can be compared against it.
func buildEmbeddingsInline(ctx context.Context, conn *Connection, body []byte) *http.Request {
	upstreamURL := strings.TrimRight(conn.BaseURL, "/") + "/v1/embeddings"
	upReq, _ := http.NewRequestWithContext(ctx, "POST", upstreamURL, strings.NewReader(string(body)))
	upReq.Header.Set("Content-Type", "application/json")
	authHeader := conn.AuthHeader
	if authHeader == "" {
		authHeader = "Authorization"
	}
	authPrefix := conn.AuthPrefix
	if authPrefix == "" {
		authPrefix = "Bearer "
	}
	if conn.APIKey != "" {
		upReq.Header.Set(authHeader, authPrefix+conn.APIKey)
	}
	return upReq
}

// reqBody drains an *http.Request body to bytes (the requests here are built
// with strings.NewReader, so this is safe and total).
func reqBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	if r.Body == nil {
		return nil
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read req body: %v", err)
	}
	return b
}

// TestF2_5_Embeddings_FlagOnVsOff is the core F2.5 server-layer evidence: the
// upstream request built with the embedder SDK ON is byte-identical to the one
// built by the inline legacy path.
func TestF2_5_Embeddings_FlagOnVsOff(t *testing.T) {
	body := []byte(`{"model":"text-embedding-3-small","input":"hello world"}`)
	conn := &Connection{
		ID: "emb-1", Name: "test-emb", BaseURL: "https://api.example.com",
		APIKey: "sk-test-key", Format: "openai", IsActive: 1, Priority: 10,
	}

	off := newEmbeddingsHandler(t, false)
	if off.embedderSDK {
		t.Fatal("embedderSDK must be false when setting is absent")
	}
	on := newEmbeddingsHandler(t, true)
	if !on.embedderSDK {
		t.Fatal("embedderSDK must be true when setting is on")
	}

	inlineReq := buildEmbeddingsInline(context.Background(), conn, body)
	sdkReq, err := on.buildEmbeddingsViaSDK(context.Background(), conn, body)
	if err != nil {
		t.Fatalf("buildEmbeddingsViaSDK: %v", err)
	}

	// Capture bodies ONCE (strings.NewReader is consumed on first read), then
	// compare. Re-reading r.Body after this would yield empty.
	inlineBody := reqBody(t, inlineReq)
	sdkBody := reqBody(t, sdkReq)

	if inlineReq.Method != sdkReq.Method {
		t.Errorf("method mismatch: off=%q on=%q", inlineReq.Method, sdkReq.Method)
	}
	if inlineReq.URL.String() != sdkReq.URL.String() {
		t.Errorf("URL mismatch: off=%q on=%q", inlineReq.URL.String(), sdkReq.URL.String())
	}
	if string(inlineBody) != string(sdkBody) {
		t.Errorf("body mismatch:\n off=%s\n on=%s", inlineBody, sdkBody)
	}
	for _, h := range []string{"Content-Type", "Authorization"} {
		if inlineReq.Header.Get(h) != sdkReq.Header.Get(h) {
			t.Errorf("%s mismatch: off=%q on=%q", h, inlineReq.Header.Get(h), sdkReq.Header.Get(h))
		}
	}

	// Spot-check absolute expectations (not just equality).
	if sdkReq.URL.String() != "https://api.example.com/v1/embeddings" {
		t.Errorf("unexpected SDK URL: %q", sdkReq.URL.String())
	}
	if got := sdkReq.Header.Get("Authorization"); got != "Bearer sk-test-key" {
		t.Errorf("SDK auth: got %q want %q", got, "Bearer sk-test-key")
	}
	if string(sdkBody) != string(body) {
		t.Errorf("body must pass through unchanged: got %s", sdkBody)
	}
}

// TestF2_5_Embeddings_CustomAuthHeaderParity proves parity under a custom auth
// header with an empty prefix (the faithful "Bearer " quirk): both paths emit
// identical headers.
func TestF2_5_Embeddings_CustomAuthHeaderParity(t *testing.T) {
	body := []byte(`{"model":"emb-x","input":["a","b"]}`)
	conn := &Connection{
		ID: "c-cust", Name: "cust", BaseURL: "https://api.example.com/",
		APIKey: "sk-test-key", Format: "anthropic", AuthHeader: "x-api-key",
		AuthPrefix: "", IsActive: 1, Priority: 10,
	}

	on := newEmbeddingsHandler(t, true)
	inlineReq := buildEmbeddingsInline(context.Background(), conn, body)
	sdkReq, err := on.buildEmbeddingsViaSDK(context.Background(), conn, body)
	if err != nil {
		t.Fatalf("buildEmbeddingsViaSDK: %v", err)
	}

	// Trailing-slash BaseURL must be normalized identically on both paths.
	if inlineReq.URL.String() != sdkReq.URL.String() {
		t.Errorf("URL mismatch: inline=%q sdk=%q", inlineReq.URL.String(), sdkReq.URL.String())
	}
	if got := sdkReq.URL.String(); got != "https://api.example.com/v1/embeddings" {
		t.Errorf("trailing-slash not normalized: %q", got)
	}
	// Faithful empty-prefix quirk: both emit "Bearer <key>" on the custom header.
	for _, r := range []*http.Request{inlineReq, sdkReq} {
		if got := r.Header.Get("x-api-key"); got != "Bearer sk-test-key" {
			t.Errorf("custom auth header: got %q want %q", got, "Bearer sk-test-key")
		}
	}
	if string(reqBody(t, inlineReq)) != string(reqBody(t, sdkReq)) {
		t.Error("body mismatch between inline and SDK paths")
	}
}
