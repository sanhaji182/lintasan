package provider

import "testing"

// The non-chat proxy endpoints (embeddings/images/audio) hardcode canonical
// "/v1/..." paths, while every catalogue preset stores a base URL that ALREADY
// ends in "/v1". Concatenating the two produced "/v1/v1/audio/speech" — a URL
// that 404s on every provider. These cases lock the join rule.
func TestJoinUpstreamPath(t *testing.T) {
	cases := []struct {
		name string
		base string
		path string
		want string
	}{
		{
			name: "bare host keeps the full versioned path",
			base: "https://api.openai.com",
			path: "/v1/audio/speech",
			want: "https://api.openai.com/v1/audio/speech",
		},
		{
			name: "versioned base does not double the version",
			base: "https://api.openai.com/v1",
			path: "/v1/audio/speech",
			want: "https://api.openai.com/v1/audio/speech",
		},
		{
			name: "versioned base with trailing slash",
			base: "https://api.together.xyz/v1/",
			path: "/v1/embeddings",
			want: "https://api.together.xyz/v1/embeddings",
		},
		{
			name: "nested versioned base",
			base: "https://api.groq.com/openai/v1",
			path: "/v1/audio/transcriptions",
			want: "https://api.groq.com/openai/v1/audio/transcriptions",
		},
		{
			name: "uppercase version segment still counts as versioned",
			base: "https://api.example.com/V1",
			path: "/v1/embeddings",
			want: "https://api.example.com/V1/embeddings",
		},
		{
			name: "version in the middle is not a suffix",
			base: "https://example.com/v1/openai",
			path: "/v1/embeddings",
			want: "https://example.com/v1/openai/v1/embeddings",
		},
		{
			name: "non-versioned nested base keeps the full path",
			base: "https://api.cline.bot/api",
			path: "/v1/images/generations",
			want: "https://api.cline.bot/api/v1/images/generations",
		},
		{
			name: "v1beta base is a version suffix",
			base: "https://generativelanguage.googleapis.com/v1beta",
			path: "/v1beta/embeddings",
			want: "https://generativelanguage.googleapis.com/v1beta/embeddings",
		},
		{
			name: "mismatched version is not trimmed",
			base: "https://api.example.com/v1",
			path: "/v2/embeddings",
			want: "https://api.example.com/v1/v2/embeddings",
		},
		{
			name: "path without leading slash is normalized",
			base: "https://api.example.com",
			path: "v1/embeddings",
			want: "https://api.example.com/v1/embeddings",
		},
		{
			name: "surrounding whitespace on base is ignored",
			base: "  https://api.example.com/v1  ",
			path: "/v1/embeddings",
			want: "https://api.example.com/v1/embeddings",
		},
		{
			name: "empty path returns the trimmed base",
			base: "https://api.example.com/v1/",
			path: "",
			want: "https://api.example.com/v1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := JoinUpstreamPath(tc.base, tc.path); got != tc.want {
				t.Errorf("JoinUpstreamPath(%q, %q) = %q, want %q", tc.base, tc.path, got, tc.want)
			}
		})
	}
}

// The concrete failure this exists to prevent: no join may ever emit a doubled
// version segment for the real catalogue base shapes.
func TestJoinUpstreamPathNeverDoublesVersion(t *testing.T) {
	bases := []string{
		"https://api.openai.com",
		"https://api.openai.com/v1",
		"https://api.groq.com/openai/v1",
		"https://api.together.xyz/v1",
		"https://api.deepinfra.com/v1",
		"https://api.mistral.ai/v1",
		"https://api.fireworks.ai/inference/v1",
	}
	paths := []string{
		"/v1/embeddings",
		"/v1/images/generations",
		"/v1/audio/speech",
		"/v1/audio/transcriptions",
	}
	for _, base := range bases {
		for _, p := range paths {
			joined := JoinUpstreamPath(base, p)
			if containsSeq(joined, "/v1/v1") {
				t.Errorf("JoinUpstreamPath(%q, %q) = %q doubles the version segment", base, p, joined)
			}
		}
	}
}

func containsSeq(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
