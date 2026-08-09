package server

import (
	"net/http"
	"strings"
	"testing"
)

// provider_header_test.go — "which provider served this?" response headers.
//
// The security-relevant case is sanitization: connection names are
// operator-typed free text stored in SQLite, so they can contain CR/LF. Those
// must never reach a header value verbatim.

func TestSanitizeHeaderValueStripsCRLF(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Cerebras", "Cerebras"},
		{"crlf injection", "Cerebras\r\nX-Injected: yes", "CerebrasX-Injected: yes"},
		{"bare LF", "a\nb", "ab"},
		{"bare CR", "a\rb", "ab"},
		{"NUL", "a\x00b", "ab"},
		{"tab", "a\tb", "ab"},
		{"DEL", "a\x7fb", "ab"},
		{"surrounding space", "  Cerebras  ", "Cerebras"},
		{"only control chars", "\r\n\t", ""},
		{"empty", "", ""},
		{"unicode kept", "Provider — ünïcode", "Provider — ünïcode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeHeaderValue(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeHeaderValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.ContainsAny(got, "\r\n") {
				t.Errorf("result still contains CR/LF: %q", got)
			}
		})
	}
}

// A pathological name must not inflate every response's header block.
func TestSanitizeHeaderValueBounded(t *testing.T) {
	got := sanitizeHeaderValue(strings.Repeat("x", 500))
	if len(got) > 128 {
		t.Errorf("len = %d, want <= 128", len(got))
	}
}

// The sanitized value must actually survive net/http's own validation —
// otherwise Go drops the header at write time and the feature silently
// disappears for exactly the malformed names we were trying to handle.
func TestSanitizedValueIsWritable(t *testing.T) {
	h := http.Header{}
	setProviderHeaders(h, "conn-1", "Evil\r\nX-Injected: yes")

	got := h.Get(ProviderHeader)
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("header value carries CR/LF: %q", got)
	}
	if h.Get("X-Injected") != "" {
		t.Error("injected header materialized")
	}
}

func TestSetProviderHeaders(t *testing.T) {
	h := http.Header{}
	setProviderHeaders(h, "conn-abc", "Cerebras")

	if got := h.Get(ProviderHeader); got != "Cerebras" {
		t.Errorf("%s = %q, want Cerebras", ProviderHeader, got)
	}
	if got := h.Get(ProviderIDHeader); got != "conn-abc" {
		t.Errorf("%s = %q, want conn-abc", ProviderIDHeader, got)
	}
}

// A blank name must not produce an empty header — an empty value is worse than
// no header, since a client can't distinguish "unknown" from "named nothing".
func TestSetProviderHeadersSkipsEmpty(t *testing.T) {
	h := http.Header{}
	setProviderHeaders(h, "", "   ")

	if _, ok := h[http.CanonicalHeaderKey(ProviderHeader)]; ok {
		t.Error("provider header set for blank name")
	}
	if _, ok := h[http.CanonicalHeaderKey(ProviderIDHeader)]; ok {
		t.Error("provider id header set for blank id")
	}
}

// Set (not Add) semantics: if an upstream echoes our header name, ours must
// replace it rather than stack, or the client sees two conflicting answers.
func TestSetProviderHeadersOverwrites(t *testing.T) {
	h := http.Header{}
	h.Add(ProviderHeader, "UpstreamClaim")
	setProviderHeaders(h, "conn-1", "RealProvider")

	vals := h.Values(ProviderHeader)
	if len(vals) != 1 {
		t.Fatalf("got %d values %v, want exactly 1", len(vals), vals)
	}
	if vals[0] != "RealProvider" {
		t.Errorf("value = %q, want RealProvider", vals[0])
	}
}
