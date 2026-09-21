package qoder

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// TestProviderTrackIsExperimental is the single most important assertion in this
// package.
//
// TrackExperimental is what keeps Qoder out of production routing. If this ever
// returns TrackOfficial, a reverse-engineered provider becomes eligible for
// default routing — the compliance posture Lintasan documents would be silently
// violated, and it would happen without any other test failing.
func TestProviderTrackIsExperimental(t *testing.T) {
	p := NewProvider("", "global", nil)
	if got := p.Track(); got != provider.TrackExperimental {
		t.Fatalf("Track() = %q, want %q — a RE-derived provider must never be routable by default", got, provider.TrackExperimental)
	}
	if p.Name() != "qoder" {
		t.Errorf("Name() = %q, want qoder", p.Name())
	}
}

// TestProviderPrepareRoundTrip drives Prepare against a stub upstream and checks
// the request that actually leaves: signed, envelope-encoded, with the model
// named consistently in the body and the headers.
func TestProviderPrepareRoundTrip(t *testing.T) {
	installTestTemplate(t)
	var sawEnvelope string
	var sawModel string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		sawEnvelope = string(raw)
		sawModel = r.Header.Get("x-model-key")
		if r.Header.Get("authorization") == "" {
			t.Error("request was not signed")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data:" + sseFrame(`{"choices":[{"delta":{"content":"ok"}}]}`, 200)))
	}))
	defer srv.Close()

	p := NewProvider("", "global", nil)
	p.sessions.endpoints.ChatStreamURL = srv.URL + "/algo/api/v2/service/pro/sse/agent_chat_generation?Encode=1"

	// Seed a session so Prepare does not need to reach the real handshake.
	sess, err := NewSession(Identity{UID: "u", UserType: "personal_standard"}, "m", "t", "ty")
	if err != nil {
		t.Fatal(err)
	}
	p.sessions.mu.Lock()
	p.sessions.cache["pt-test"] = &cachedSession{session: sess, expiresAt: time.Now().Add(time.Hour)}
	p.sessions.mu.Unlock()

	up, err := p.Prepare(context.Background(), &provider.Request{
		Model: "qmodel_38max",
		Body:  []byte(`{"model":"qmodel_38max","stream":true,"messages":[{"role":"user","content":"say PONG"}]}`),
	}, &provider.ConnConfig{ID: "c1", APIKey: "pt-test"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if up.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", up.Method)
	}
	if up.URL == "" {
		t.Error("no upstream URL produced")
	}

	// Prepare describes the call but deliberately does not perform it (execution
	// belongs to the router, so reliability wrapping stays outside the provider).
	// The test therefore has to execute it to observe what would go on the wire.
	httpReq, err := http.NewRequestWithContext(context.Background(), up.Method, up.URL, strings.NewReader(string(up.Body)))
	if err != nil {
		t.Fatalf("build request from prepared upstream: %v", err)
	}
	for k, vs := range up.Header {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("execute prepared request: %v", err)
	}
	resp.Body.Close()

	if sawModel != "qmodel_38max" {
		t.Errorf("x-model-key = %q, want qmodel_38max", sawModel)
	}
	if !isCosyEnvelope(sawEnvelope) {
		t.Errorf("body is not a cosy envelope: %.80q", sawEnvelope)
	}
	back, err := DecodeBase64(sawEnvelope)
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(back, &parsed); err != nil {
		t.Fatalf("envelope does not decode to JSON: %v", err)
	}
	if mc, ok := parsed["model_config"].(map[string]any); ok {
		if mc["key"] != "qmodel_38max" {
			t.Errorf("body model_config.key = %v", mc["key"])
		}
	} else {
		t.Error("model_config missing from the prepared body")
	}
}

// TestProviderPrepareRejectsBadInput keeps malformed inbound requests from
// reaching upstream, where the resulting error would be opaque.
func TestProviderPrepareRejectsBadInput(t *testing.T) {
	p := NewProvider("", "global", nil)
	conn := &provider.ConnConfig{ID: "c1", APIKey: "pt-test"}

	cases := []struct {
		name string
		req  *provider.Request
		conn *provider.ConnConfig
	}{
		{"nil request", nil, conn},
		{"nil connection", &provider.Request{Body: []byte(`{"model":"m","messages":[{"role":"user","content":"x"}]}`)}, nil},
		{"no credential", &provider.Request{Body: []byte(`{"model":"m","messages":[{"role":"user","content":"x"}]}`)}, &provider.ConnConfig{ID: "c1"}},
		{"no model", &provider.Request{Body: []byte(`{"messages":[{"role":"user","content":"x"}]}`)}, conn},
		{"no messages", &provider.Request{Body: []byte(`{"model":"m"}`)}, conn},
		{"invalid json", &provider.Request{Body: []byte(`{not json`)}, conn},
	}
	for _, c := range cases {
		if _, err := p.Prepare(context.Background(), c.req, c.conn); err == nil {
			t.Errorf("%s: expected an error", c.name)
		}
	}
}

// TestProviderTranslateRefusesLoudly pins the deliberate refusal.
//
// Returning the raw upstream bytes here would emit envelope frames to a client
// expecting OpenAI chunks. A loud error at the boundary is worth far more than a
// silent wrong answer, because the wrong answer surfaces as a parsing bug in the
// client, arbitrarily far from this code.
func TestProviderTranslateRefusesLoudly(t *testing.T) {
	p := NewProvider("", "global", nil)
	_, err := p.Translate(context.Background(), []byte(`{"anything":true}`), &provider.Request{})
	if err == nil {
		t.Fatal("Translate must refuse rather than return untranslated envelope frames")
	}
	if !strings.Contains(err.Error(), "enveloped frame stream") {
		t.Errorf("refusal should explain why, got: %v", err)
	}
}

// TestProviderCapabilitiesAreDeclaredNotAsserted keeps the declared capability
// set honest: per the SDK these are display values, so the test checks the set is
// non-empty and plausible rather than treating it as a routing contract.
func TestProviderCapabilitiesAreDeclaredNotAsserted(t *testing.T) {
	caps := NewProvider("", "global", nil).Capabilities()
	if len(caps) == 0 {
		t.Fatal("no capabilities declared")
	}
	// Chat + streaming are the two the proxy actually relies on for this surface.
	if !caps.Has(provider.CapStreaming) {
		t.Error("streaming should be declared: the Qoder chat surface is streaming-only")
	}
}

// TestIsReasoningModel covers the key-based heuristic used when only the model
// name is available.
func TestIsReasoningModel(t *testing.T) {
	reasoning := []string{"qmodel_38max", "qoder-reasoning", "model_thinking"}
	for _, m := range reasoning {
		if !isReasoningModel(m) {
			t.Errorf("%s should be treated as a reasoning model", m)
		}
	}
	plain := []string{"auto", "qfmodel", "gpt-4o"}
	for _, m := range plain {
		if isReasoningModel(m) {
			t.Errorf("%s should not be treated as a reasoning model", m)
		}
	}
}
