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
)

// isCosyEnvelope reports whether s consists only of characters in the cosy
// envelope alphabet (plus the custom pad). It is the precise form of "this body
// was envelope-encoded" — checking for individual punctuation characters is
// unreliable because the alphabet legitimately contains several of them.
func isCosyEnvelope(s string) bool {
	for i := 0; i < len(s); i++ {
		c := int(s[i])
		if c >= 128 {
			return false
		}
		// custom2std maps encoded -> standard, so a negative entry means the byte
		// is NOT part of the envelope alphabet. (std2custom answers the reverse
		// question and would accept any standard-base64 character.)
		if custom2std[c] < 0 && c != customPad {
			return false
		}
	}
	return true
}

// TestStartChatStreamEncodesBody is the regression test for the defect that made
// every chat attempt fail silently.
//
// The chat endpoint is an Encode=1 endpoint: the body must be wrapped in the
// cosy base64 envelope and the signature must cover the ENCODED bytes. Sending
// plain JSON produces no useful client-side error — upstream's decoder rejects
// the first space character and reports an internal server error, which looks
// like an upstream outage rather than a malformed request. Worse, the response
// still arrives as HTTP 200, so nothing above it can tell the difference.
//
// This test asserts on what actually goes on the wire.
func TestStartChatStreamEncodesBody(t *testing.T) {
	var gotBody string
	var gotSig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		gotSig = r.Header.Get("authorization")

		// The body must be a valid cosy envelope that decodes back to JSON.
		decoded, err := DecodeBase64(gotBody)
		if err != nil {
			t.Errorf("upstream received a body that is not a cosy envelope: %v", err)
		} else if !json.Valid(decoded) {
			t.Errorf("envelope did not decode to JSON: %q", decoded)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data:" + sseFrame(`{"choices":[{"delta":{"content":"ok"}}]}`, 200)))
	}))
	defer srv.Close()

	m := NewSessionManager("", "global", nil)
	m.endpoints.ChatStreamURL = srv.URL + "/algo/api/v2/service/pro/sse/agent_chat_generation?Encode=1"

	// Seed a session directly so the test does not depend on the live handshake.
	sess, err := NewSession(Identity{Name: "t", AccountID: "acct", UID: "acct"}, "m", "t", "ty")
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.cache["cred"] = &cachedSession{session: sess, expiresAt: time.Now().Add(time.Hour)}
	m.mu.Unlock()

	// The content deliberately contains a space: real prompts do, and a space is
	// exactly the character upstream's decoder chokes on when the body is not
	// envelope-encoded ("Illegal base64 character 20").
	plain := []byte(`{"model":"auto","stream":true,"messages":[{"role":"user","content":"say PONG"}]}`)
	if !strings.Contains(string(plain), " ") {
		t.Fatal("test fixture must contain a space to reproduce the original defect")
	}

	resp, err := m.StartChatStream(context.Background(), "cred", "auto", plain, "")
	if err != nil {
		t.Fatalf("StartChatStream: %v", err)
	}
	defer resp.Body.Close()

	// Every byte must be in the cosy alphabet; raw JSON would contain spaces,
	// braces and quotes, none of which are.
	if !isCosyEnvelope(gotBody) {
		t.Errorf("body is not a cosy envelope (raw JSON reached upstream): %.80q", gotBody)
	}
	// And it must round-trip to exactly what the caller asked to send.
	back, err := DecodeBase64(gotBody)
	if err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if string(back) != string(plain) {
		t.Errorf("body did not round-trip.\n got: %s\nwant: %s", back, plain)
	}
	if !strings.HasPrefix(gotSig, "Bearer COSY.") {
		t.Errorf("authorization header is not a COSY bearer: %q", gotSig)
	}
}

// TestBuildAPIHeadersSignsTheEncodedBody documents that the signature covers the
// envelope, not the plaintext. Signing the wrong one produces a signature
// mismatch that upstream reports as an authentication failure — easy to
// misdiagnose as a bad credential.
func TestBuildAPIHeadersSignsTheEncodedBody(t *testing.T) {
	m := NewSessionManager("", "global", nil)
	sess, err := NewSession(Identity{UID: "u"}, "m", "t", "ty")
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := EncodeBase64([]byte(`{"a": 1}`))
	if err != nil {
		t.Fatal(err)
	}

	h, err := m.BuildAPIHeaders(sess, []byte(encoded), "/api/v2/x", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}

	bearer := h["authorization"]
	if !strings.HasPrefix(bearer, "Bearer COSY.") {
		t.Fatalf("unexpected bearer: %q", bearer)
	}
	parts := strings.SplitN(strings.TrimPrefix(bearer, "Bearer COSY."), ".", 2)
	if len(parts) != 2 {
		t.Fatalf("bearer is not payload.signature: %q", bearer)
	}
	payload, sig := parts[0], parts[1]

	// Recomputing with the SAME encoded body must reproduce the signature, and
	// recomputing with the plaintext must not.
	want := SignRequest(payload, sess.CosyKey, h["cosy-date"], []byte(encoded), "/api/v2/x")
	if sig != want {
		t.Error("signature does not cover the encoded body")
	}
	plainSig := SignRequest(payload, sess.CosyKey, h["cosy-date"], []byte(`{"a": 1}`), "/api/v2/x")
	if sig == plainSig {
		t.Error("signature is indistinguishable from one over the plaintext; the encoding is not being covered")
	}
}

// TestStartChatStreamSendsModelHeaders keeps the body/header pairing intact.
// Upstream cross-checks the model named in the body against these headers and
// rejects a mismatch, so a refactor that sets one and not the other breaks every
// request.
func TestStartChatStreamSendsModelHeaders(t *testing.T) {
	installTestTemplate(t)
	var bodyModel, hdrModel string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		decoded, _ := DecodeBase64(string(raw))
		var parsed map[string]any
		_ = json.Unmarshal(decoded, &parsed)
		if mc, ok := parsed["model_config"].(map[string]any); ok {
			bodyModel, _ = mc["key"].(string)
		}
		hdrModel = r.Header.Get("x-model-key")

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data:" + sseFrame(`{"choices":[{"delta":{"content":"ok"}}]}`, 200)))
	}))
	defer srv.Close()

	m := NewSessionManager("", "global", nil)
	m.endpoints.ChatStreamURL = srv.URL + "/algo/api/v2/x?Encode=1"
	sess, _ := NewSession(Identity{UID: "u"}, "m", "t", "ty")
	m.mu.Lock()
	m.cache["cred"] = &cachedSession{session: sess, expiresAt: time.Now().Add(time.Hour)}
	m.mu.Unlock()

	body, err := BuildChatBody(ChatRequest{
		Model:    "qmodel_38max",
		Messages: []map[string]any{{"role": "user", "content": "hi"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := m.StartChatStream(context.Background(), "cred", "qmodel_38max", body, "")
	if err != nil {
		t.Fatalf("StartChatStream: %v", err)
	}
	defer resp.Body.Close()

	if bodyModel != "qmodel_38max" {
		t.Errorf("body model_config.key = %q, want qmodel_38max", bodyModel)
	}
	if hdrModel != "qmodel_38max" {
		t.Errorf("x-model-key = %q, want qmodel_38max", hdrModel)
	}
}

// TestStartChatStreamSurfacesNon200 keeps a transport-level rejection from being
// returned as a usable stream.
func TestStartChatStreamSurfacesNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"slow down"}`))
	}))
	defer srv.Close()

	m := NewSessionManager("", "global", nil)
	m.endpoints.ChatStreamURL = srv.URL + "/x?Encode=1"
	sess, err := NewSession(Identity{UID: "u"}, "m", "t", "ty")
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.cache["cred"] = &cachedSession{session: sess, expiresAt: time.Now().Add(time.Hour)}
	m.mu.Unlock()

	resp, err := m.StartChatStream(context.Background(), "cred", "auto", []byte("{}"), "")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected an error for a non-200 response")
	}
	ue, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("error is not an *UpstreamError: %T", err)
	}
	if ue.Status != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", ue.Status)
	}
}
