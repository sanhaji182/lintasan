package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// capturedReq records what an upstream server actually received, so we can
// compare the request produced by the legacy doUpstream path against the one
// produced by the Provider SDK path byte-for-byte.
type capturedReq struct {
	method string
	path   string
	rawURL string
	header http.Header
	body   []byte
}

// newCapturingUpstream returns an httptest server that records the inbound
// request into *out and replies with a trivial 200 body.
func newCapturingUpstream(t *testing.T, out *capturedReq) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		out.method = r.Method
		out.path = r.URL.Path
		out.rawURL = r.URL.RequestURI()
		out.header = r.Header.Clone()
		out.body = b
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// buildHandler constructs a ProxyHandler whose provider_sdk_enabled flag is set
// to sdkOn. The flag is read at construction time (initProviderSDK), so the
// setting must be written before the handler is built.
func buildHandler(t *testing.T, sdkOn bool) *ProxyHandler {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if sdkOn {
		if err := database.SetSetting("provider_sdk_enabled", "true"); err != nil {
			t.Fatalf("set flag: %v", err)
		}
	}
	return NewProxyHandler(&config.Config{}, database)
}

// buildHandlerWithFlag builds a handler after writing an arbitrary raw string
// value for provider_sdk_enabled, used to exercise flag-parsing edge cases.
func buildHandlerWithFlag(t *testing.T, raw string) *ProxyHandler {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.SetSetting("provider_sdk_enabled", raw); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	return NewProxyHandler(&config.Config{}, database)
}

// runDoUpstream invokes the real doUpstream against the capturing upstream. The
// upstream captured. inboundHeaders are attached to the synthetic inbound
// request (doUpstream reads r.Header / r.Context).
func runDoUpstream(t *testing.T, p *ProxyHandler, conn *Connection, body []byte, inboundHeaders http.Header, out *capturedReq) {
	t.Helper()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	for k, vs := range inboundHeaders {
		for _, v := range vs {
			inbound.Header.Add(k, v)
		}
	}
	resp, err := p.doUpstream(inbound, conn, body)
	if err != nil {
		t.Fatalf("doUpstream error: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
}

// openAICompatConn returns a connection pointing at the capturing upstream,
// configured like a typical OpenAI-compatible provider (the F1 default path).
func openAICompatConn(baseURL, format string) *Connection {
	return &Connection{
		ID:       "c1",
		Name:     "test-" + format,
		BaseURL:  baseURL,
		APIKey:   "sk-test-key",
		Format:   format,
		IsActive: 1,
		Priority: 10,
	}
}

// TestT2_Parity_LegacyVsSDK is the core F1 evidence: for every OpenAI-compatible
// format (openai/anthropic/gemini/deepseek/groq), the request the SDK path emits
// upstream must be byte-identical to the legacy path. This proves zero behavior
// change for the five F1 target providers.
func TestT2_Parity_LegacyVsSDK(t *testing.T) {
	formats := []string{"openai", "anthropic", "gemini", "deepseek", "groq"}
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	inbound := http.Header{}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			// --- legacy path (flag off) ---
			var legacy capturedReq
			lsrv := newCapturingUpstream(t, &legacy)
			lp := buildHandler(t, false)
			runDoUpstream(t, lp, openAICompatConn(lsrv.URL, format), body, inbound, &legacy)

			// --- SDK path (flag on) ---
			var sdk capturedReq
			ssrv := newCapturingUpstream(t, &sdk)
			sp := buildHandler(t, true)
			runDoUpstream(t, sp, openAICompatConn(ssrv.URL, format), body, inbound, &sdk)

			if legacy.method != sdk.method {
				t.Errorf("method mismatch: legacy=%q sdk=%q", legacy.method, sdk.method)
			}
			if legacy.path != sdk.path {
				t.Errorf("path mismatch: legacy=%q sdk=%q", legacy.path, sdk.path)
			}
			if legacy.rawURL != sdk.rawURL {
				t.Errorf("rawURL mismatch: legacy=%q sdk=%q", legacy.rawURL, sdk.rawURL)
			}
			if string(legacy.body) != string(sdk.body) {
				t.Errorf("body mismatch:\n legacy=%s\n sdk=%s", legacy.body, sdk.body)
			}
			// Auth + content-type are the behavior-critical headers.
			if got, want := sdk.header.Get("Authorization"), legacy.header.Get("Authorization"); got != want {
				t.Errorf("Authorization mismatch: legacy=%q sdk=%q", want, got)
			}
			if got, want := sdk.header.Get("Content-Type"), legacy.header.Get("Content-Type"); got != want {
				t.Errorf("Content-Type mismatch: legacy=%q sdk=%q", want, got)
			}
			// Regression guard: empty AuthPrefix must become "Bearer " on BOTH paths.
			if want := "Bearer sk-test-key"; legacy.header.Get("Authorization") != want {
				t.Errorf("legacy auth prefix wrong: got %q want %q", legacy.header.Get("Authorization"), want)
			}
		})
	}
}

// TestT2_Parity_CustomAuthHeader checks parity when a connection overrides the
// auth header / prefix (e.g. x-api-key style). Both paths must honor it identically.
func TestT2_Parity_CustomAuthHeader(t *testing.T) {
	body := []byte(`{"model":"claude-3","messages":[]}`)
	mk := func(baseURL string) *Connection {
		c := openAICompatConn(baseURL, "anthropic")
		c.AuthHeader = "x-api-key"
		c.AuthPrefix = "" // empty prefix => legacy code defaults it to "Bearer "
		c.ChatPath = "/v1/messages"
		return c
	}

	var legacy capturedReq
	lsrv := newCapturingUpstream(t, &legacy)
	lp := buildHandler(t, false)
	runDoUpstream(t, lp, mk(lsrv.URL), body, http.Header{}, &legacy)

	var sdk capturedReq
	ssrv := newCapturingUpstream(t, &sdk)
	sp := buildHandler(t, true)
	runDoUpstream(t, sp, mk(ssrv.URL), body, http.Header{}, &sdk)

	if !reflect.DeepEqual(legacy.header.Values("X-Api-Key"), sdk.header.Values("X-Api-Key")) {
		t.Errorf("x-api-key mismatch: legacy=%v sdk=%v",
			legacy.header.Values("X-Api-Key"), sdk.header.Values("X-Api-Key"))
	}
	if legacy.path != sdk.path {
		t.Errorf("chat_path mismatch: legacy=%q sdk=%q", legacy.path, sdk.path)
	}
	// FAITHFUL QUIRK: the live router (proxy.go:986-987) coerces an empty
	// AuthPrefix to "Bearer " — there is no way to send a truly bare token.
	// The SDK's DefaultProvider.Prepare reproduces this EXACTLY. Asserting the
	// quirk (not an idealized behavior) is the whole point of zero-behavior-change:
	// both paths must emit "Bearer sk-test-key" under a custom auth header too.
	if got, want := sdk.header.Get("X-Api-Key"), "Bearer sk-test-key"; got != want {
		t.Errorf("custom auth header value mismatch on SDK path: got %q want %q (the faithful empty-prefix quirk)", got, want)
	}
	if got := legacy.header.Get("X-Api-Key"); got != "Bearer sk-test-key" {
		t.Errorf("legacy path did not exhibit the documented empty-prefix quirk: %q", got)
	}
}
