package server

import (
	"net/http"
	"testing"
)

// TestIsPublicUIPath locks the security contract of the embedded-SPA path
// allowlist: dashboard UI + static assets are public, but every API / proxy /
// protocol namespace stays gated. A regression here could silently expose an
// authenticated endpoint, so these cases are exhaustive on the boundaries.
func TestIsPublicUIPath(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   bool
	}{
		// --- public UI: static asset prefixes ---
		{"GET", "/_app/immutable/entry/start.abc123.js", true},
		{"GET", "/_app/immutable/chunks/x.js", true},
		{"GET", "/favicon.png", true},
		{"GET", "/favicon.svg", true},
		{"GET", "/favicon.ico", true},
		{"GET", "/favicon-32.png", true},
		{"GET", "/apple-touch-icon.png", true},
		{"GET", "/site.webmanifest", true},
		{"GET", "/lintasan-mark-512.png", true},
		{"GET", "/lintasan-wordmark-dark.svg", true},
		{"GET", "/robots.txt", true},
		{"GET", "/favicon-private.json", false},
		{"GET", "/lintasan-admin", false},
		// --- public UI: SPA routes ---
		{"GET", "/login", true},
		{"GET", "/change-password", true},
		{"GET", "/dashboard", true},
		{"GET", "/dashboard/users", true},
		{"GET", "/dashboard/connections", true},
		{"HEAD", "/dashboard", true},
		// --- gated: API namespace must NEVER be public ---
		{"GET", "/api/connections", false},
		{"GET", "/api/keys", false},
		{"GET", "/api/users", false},
		{"GET", "/api/auth/me", false},
		{"GET", "/api/settings", false},
		// --- gated: proxy + protocol namespaces ---
		{"GET", "/v1/models", false},
		{"GET", "/v1/chat/completions", false},
		{"GET", "/mcp", false},
		{"GET", "/mcp/sse", false},
		// --- gated: non-GET to a UI-looking path (no mutation via UI rule) ---
		{"POST", "/dashboard", false},
		{"DELETE", "/login", false},
		{"POST", "/_app/x.js", false},
		// --- not a known UI path ---
		{"GET", "/random/thing", false},
		{"GET", "/api", false},
	}
	for _, c := range cases {
		if got := isPublicUIPath(c.method, c.path); got != c.want {
			t.Errorf("isPublicUIPath(%q, %q) = %v, want %v", c.method, c.path, got, c.want)
		}
	}
}

// TestPublicUIPath_APIPrefixNeverPublic is a focused fuzz-ish guard: no path
// beginning with an API/proxy prefix may ever be classified public, regardless
// of method.
func TestPublicUIPath_APIPrefixNeverPublic(t *testing.T) {
	prefixes := []string{"/api/", "/v1/", "/mcp"}
	suffixes := []string{"", "x", "dashboard", "_app/y.js", "login", "../login"}
	methods := []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"}
	for _, p := range prefixes {
		for _, s := range suffixes {
			for _, m := range methods {
				path := p + s
				if isPublicUIPath(m, path) {
					t.Errorf("API-namespace path leaked as public: isPublicUIPath(%q,%q)=true", m, path)
				}
			}
		}
	}
	_ = http.MethodGet
}
