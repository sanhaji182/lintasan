package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestT3_DefaultFlagOff proves the kill-switch defaults to false: a freshly
// built handler with no provider_sdk_enabled setting must NOT engage the SDK.
func TestT3_DefaultFlagOff(t *testing.T) {
	p := buildHandler(t, false)
	if p.providerSDK {
		t.Fatal("providerSDK must default to false when setting is absent")
	}
	if p.providerReg == nil || p.defaultProvider == nil {
		t.Fatal("registry and default provider must still be constructed (just inert)")
	}
	// Eligibility must be false for any connection while the flag is off.
	if p.providerSDKEligible(openAICompatConn("http://x", "openai")) {
		t.Error("no connection should be SDK-eligible when flag is off")
	}
}

// TestT3_FlagParsing checks the accepted truthy spellings and that everything
// else stays off (fail-safe toward legacy).
func TestT3_FlagParsing(t *testing.T) {
	truthy := []string{"true", "1", "on", "yes", "TRUE", "On", " yes "}
	for _, v := range truthy {
		t.Run("on/"+strings.TrimSpace(v), func(t *testing.T) {
			p := buildHandlerWithFlag(t, v)
			if !p.providerSDK {
				t.Errorf("value %q should enable SDK", v)
			}
		})
	}
	falsy := []string{"", "false", "0", "off", "no", "nope", "enabled?"}
	for _, v := range falsy {
		t.Run("off/"+v, func(t *testing.T) {
			p := buildHandlerWithFlag(t, v)
			if p.providerSDK {
				t.Errorf("value %q must NOT enable SDK", v)
			}
		})
	}
}

// TestT3_CommandCodeStaysLegacy is the most important guard: even with the flag
// ON, a commandcode connection must take the legacy path. We assert this by
// confirming the upstream receives the CC-specific header and the transformed
// (threadId/config/params-shaped) body, which only the legacy branch produces.
func TestT3_CommandCodeStaysLegacy(t *testing.T) {
	var cap capturedReq
	srv := newCapturingUpstream(t, &cap)
	p := buildHandler(t, true) // flag ON
	conn := openAICompatConn(srv.URL, "commandcode")

	// Eligibility must reject commandcode regardless of flag.
	if p.providerSDKEligible(conn) {
		t.Fatal("commandcode must NEVER be SDK-eligible, even with flag on")
	}

	body := []byte(`{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hi"}],"max_tokens":16384}`)
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp, err := p.doUpstream(inbound, conn, body)
	if err != nil {
		t.Fatalf("doUpstream: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	// Legacy commandcode branch sets this header (proxy.go).
	if cap.header.Get("x-command-code-version") == "" {
		t.Error("commandcode header missing — SDK path wrongly engaged")
	}
	// Legacy branch transforms the body into the CC envelope (adds "system"/"stream").
	if !strings.Contains(string(cap.body), `"stream":true`) {
		t.Errorf("commandcode body not transformed by legacy path: %s", cap.body)
	}
}

// TestT3_FlagOffIgnoresSDK confirms that with the flag off, even an
// OpenAI-compatible connection takes the legacy path (SDK never engaged).
// We verify by checking the legacy path produced the request (Authorization
// present, no panic) and that providerSDKEligible is false.
func TestT3_FlagOffIgnoresSDK(t *testing.T) {
	var cap capturedReq
	srv := newCapturingUpstream(t, &cap)
	p := buildHandler(t, false)
	conn := openAICompatConn(srv.URL, "openai")
	if p.providerSDKEligible(conn) {
		t.Fatal("openai conn must not be eligible when flag off")
	}
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp, err := p.doUpstream(inbound, conn, []byte(`{"model":"gpt-4o"}`))
	if err != nil {
		t.Fatalf("doUpstream: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	if cap.header.Get("Authorization") != "Bearer sk-test-key" {
		t.Errorf("legacy path auth wrong: %q", cap.header.Get("Authorization"))
	}
}
