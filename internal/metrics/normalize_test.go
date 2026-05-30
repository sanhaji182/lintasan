package metrics

import "testing"

func TestNormalizeEndpoint_BoundsCardinality(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		// Dynamic segments must collapse to their group.
		{"/api/connections/abc-123", "/api/connections"},
		{"/api/connections", "/api/connections"},
		{"/v1/memory/sha256deadbeef", "/v1/memory"},
		{"/v1/memory/stats", "/v1/memory"},
		{"/v1/memory/search", "/v1/memory"},
		{"/v1/chat/completions", "/v1/chat/completions"},
		{"/v1/embeddings", "/v1/embeddings"},
		{"/v1/audio/speech", "/v1/audio"},
		{"/v1/audio/transcriptions", "/v1/audio"},
		{"/api/auth/login", "/api/auth"},
		{"/api/auth/me", "/api/auth"},
		{"/api/stats", "/api"},
		{"/api/logs", "/api"},
		{"/mcp/sse", "/mcp"},
		{"/metrics", "/metrics"},
		{"/health", "/health"},
		// Unknown paths must not pass through verbatim.
		{"/some/random/unmapped/thing", "other"},
		{"/", "other"},
	}
	for _, c := range cases {
		got := NormalizeEndpoint(c.path)
		if got != c.want {
			t.Errorf("NormalizeEndpoint(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// TestNormalizeEndpoint_NeverEchoesIDs ensures a path carrying a high-entropy
// id never surfaces that id in the returned group label.
func TestNormalizeEndpoint_NeverEchoesIDs(t *testing.T) {
	ids := []string{"abc-123-def-456", "sha256deadbeefcafe", "user_99887766"}
	paths := []string{
		"/api/connections/abc-123-def-456",
		"/v1/memory/sha256deadbeefcafe",
		"/api/combos/user_99887766",
	}
	for _, p := range paths {
		group := NormalizeEndpoint(p)
		for _, id := range ids {
			if group == id || contains(group, id) {
				t.Errorf("group %q leaked an id from path %q", group, p)
			}
		}
	}
}

func TestStatusClass(t *testing.T) {
	cases := map[int]string{
		200: "2xx", 201: "2xx", 204: "2xx",
		301: "3xx", 304: "3xx",
		400: "4xx", 401: "4xx", 404: "4xx", 429: "4xx",
		500: "5xx", 503: "5xx",
		100: "1xx",
	}
	for code, want := range cases {
		if got := StatusClass(code); got != want {
			t.Errorf("StatusClass(%d) = %q, want %q", code, got, want)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
