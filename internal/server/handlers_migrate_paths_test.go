package server

import "testing"

// A competitor export stores a provider endpoint as a full base URL, and that
// URL usually already ends in the version segment. Appending the conventional
// "/v1/chat/completions" to such a base yields "/v1/v1/chat/completions", which
// 404s against every provider we import. These cases lock the path selection so
// an imported connection is reachable without hand-editing it afterwards.
func TestAPIPathsFor(t *testing.T) {
	cases := []struct {
		name       string
		baseURL    string
		wantChat   string
		wantModels string
	}{
		{
			name:       "base already versioned",
			baseURL:    "https://api.xiaomimimo.com/v1",
			wantChat:   "/chat/completions",
			wantModels: "/models",
		},
		{
			name:       "versioned base with trailing slash",
			baseURL:    "https://api.cerebras.ai/v1/",
			wantChat:   "/chat/completions",
			wantModels: "/models",
		},
		{
			name:       "bare host needs the version segment",
			baseURL:    "https://api.commandcode.ai",
			wantChat:   "/v1/chat/completions",
			wantModels: "/v1/models",
		},
		{
			name:       "nested path that is not versioned",
			baseURL:    "https://api.cline.bot/api",
			wantChat:   "/v1/chat/completions",
			wantModels: "/v1/models",
		},
		{
			name:       "version segment in the middle is not a suffix",
			baseURL:    "https://example.com/v1/openai",
			wantChat:   "/v1/chat/completions",
			wantModels: "/v1/models",
		},
		{
			name:       "uppercase version segment",
			baseURL:    "https://api.example.com/V1",
			wantChat:   "/chat/completions",
			wantModels: "/models",
		},
		{
			name:       "surrounding whitespace is ignored",
			baseURL:    "  https://api.example.com/v1  ",
			wantChat:   "/chat/completions",
			wantModels: "/models",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat, models := apiPathsFor(tc.baseURL)
			if chat != tc.wantChat {
				t.Errorf("chat path for %q = %q, want %q", tc.baseURL, chat, tc.wantChat)
			}
			if models != tc.wantModels {
				t.Errorf("models path for %q = %q, want %q", tc.baseURL, models, tc.wantModels)
			}
		})
	}
}

// Whatever paths we choose, joining them to the base URL must never produce a
// doubled version segment — that was the concrete failure this guards against.
func TestAPIPathsForNeverDoublesVersionSegment(t *testing.T) {
	bases := []string{
		"https://api.xiaomimimo.com/v1",
		"https://api.cerebras.ai/v1/",
		"https://integrate.api.nvidia.com/v1",
		"https://api.commandcode.ai",
		"https://api.cline.bot/api",
	}
	for _, base := range bases {
		chat, models := apiPathsFor(base)
		for _, joined := range []string{
			trimRightSlash(base) + chat,
			trimRightSlash(base) + models,
		} {
			if containsDoubleVersion(joined) {
				t.Errorf("joined URL %q contains a doubled version segment", joined)
			}
		}
	}
}

func trimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func containsDoubleVersion(s string) bool {
	for i := 0; i+len("/v1/v1") <= len(s); i++ {
		if s[i:i+len("/v1/v1")] == "/v1/v1" {
			return true
		}
	}
	return false
}
