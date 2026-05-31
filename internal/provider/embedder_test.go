package provider

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// legacyEmbeddingsRequest reproduces, in the test, EXACTLY what the inline
// internal/server/proxy.go HandleEmbeddings handler builds for a given
// connection + body. The Embedder under test must match this byte-for-byte.
// Keeping a hand-written reference here (rather than importing the server)
// makes the parity contract explicit and keeps the provider package free of a
// server dependency.
func legacyEmbeddingsRequest(conn *ConnConfig, body []byte) (url, method, contentType, authHeader, authValue string, outBody []byte) {
	url = strings.TrimRight(conn.BaseURL, "/") + "/v1/embeddings"
	method = http.MethodPost
	contentType = "application/json"
	ah := conn.AuthHeader
	if ah == "" {
		ah = "Authorization"
	}
	ap := conn.AuthPrefix
	if ap == "" {
		ap = "Bearer "
	}
	authHeader = ah
	if conn.APIKey != "" {
		authValue = ap + conn.APIKey
	}
	outBody = body
	return
}

// TestDefaultProviderImplementsEmbedder proves the optional Embedder interface
// is satisfied via a runtime type assertion (the compile-time var _ assertion
// in embedder.go covers the build; this covers the executable contract).
func TestDefaultProviderImplementsEmbedder(t *testing.T) {
	var p Provider = NewDefaultProvider("openai")
	if _, ok := p.(Embedder); !ok {
		t.Fatal("DefaultProvider must satisfy the Embedder interface")
	}
}

// TestEmbedByteParityVsLegacy is the core F2.5 evidence: for a range of
// connection shapes (default auth, custom auth header, empty-prefix quirk,
// trailing-slash BaseURL, missing API key), the UpstreamRequest the Embedder
// produces must match what the legacy inline handler would emit byte-for-byte.
func TestEmbedByteParityVsLegacy(t *testing.T) {
	body := []byte(`{"model":"text-embedding-3-small","input":"hello world"}`)
	cases := []struct {
		name string
		conn *ConnConfig
	}{
		{
			name: "default-auth",
			conn: &ConnConfig{BaseURL: "https://api.openai.com", APIKey: "sk-abc", Format: "openai"},
		},
		{
			name: "trailing-slash-baseurl",
			conn: &ConnConfig{BaseURL: "https://api.openai.com/", APIKey: "sk-abc", Format: "openai"},
		},
		{
			name: "custom-auth-header-empty-prefix-quirk",
			conn: &ConnConfig{BaseURL: "https://api.anthropic.com", APIKey: "sk-xyz", AuthHeader: "x-api-key", AuthPrefix: "", Format: "anthropic"},
		},
		{
			name: "custom-prefix",
			conn: &ConnConfig{BaseURL: "https://host", APIKey: "key", AuthHeader: "X-Token", AuthPrefix: "Token ", Format: "openai"},
		},
		{
			name: "no-api-key-omits-auth",
			conn: &ConnConfig{BaseURL: "https://host", APIKey: "", Format: "openai"},
		},
	}

	d := NewDefaultProvider("openai")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up, err := d.Embed(context.Background(), &Request{Body: body}, tc.conn)
			if err != nil {
				t.Fatalf("Embed error: %v", err)
			}

			wURL, wMethod, wCT, wAuthHeader, wAuthVal, wBody := legacyEmbeddingsRequest(tc.conn, body)

			if up.URL != wURL {
				t.Errorf("URL mismatch: got %q want %q", up.URL, wURL)
			}
			if up.Method != wMethod {
				t.Errorf("Method mismatch: got %q want %q", up.Method, wMethod)
			}
			if got := up.Header.Get("Content-Type"); got != wCT {
				t.Errorf("Content-Type mismatch: got %q want %q", got, wCT)
			}
			if got := up.Header.Get(wAuthHeader); got != wAuthVal {
				t.Errorf("auth header %q mismatch: got %q want %q", wAuthHeader, got, wAuthVal)
			}
			// Body must be the SAME backing bytes (passthrough), not a copy.
			if string(up.Body) != string(wBody) {
				t.Errorf("body mismatch:\n got=%s\n want=%s", up.Body, wBody)
			}
			if &up.Body[0] != &body[0] {
				t.Error("Body must be a passthrough of req.Body (same backing array), not a copy")
			}
		})
	}
}

// TestEmbedEmptyPrefixQuirk pins the faithful quirk in isolation: a custom auth
// header with an empty prefix still emits "Bearer <key>", exactly like the live
// handler. There is intentionally no way to send a truly bare token.
func TestEmbedEmptyPrefixQuirk(t *testing.T) {
	d := NewDefaultProvider("openai")
	conn := &ConnConfig{BaseURL: "https://h", APIKey: "k", AuthHeader: "x-api-key", AuthPrefix: ""}
	up, err := d.Embed(context.Background(), &Request{Body: []byte("{}")}, conn)
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if got, want := up.Header.Get("x-api-key"), "Bearer k"; got != want {
		t.Errorf("empty-prefix quirk: got %q want %q", got, want)
	}
}

// TestEmbedNilGuards verifies the same nil-handling contract as Prepare.
func TestEmbedNilGuards(t *testing.T) {
	d := NewDefaultProvider("openai")
	if _, err := d.Embed(context.Background(), nil, &ConnConfig{}); err == nil {
		t.Error("nil req must error")
	}
	if _, err := d.Embed(context.Background(), &Request{}, nil); err == nil {
		t.Error("nil conn must error")
	}
}
