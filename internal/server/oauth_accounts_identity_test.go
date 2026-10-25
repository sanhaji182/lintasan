package server

import (
	"encoding/base64"
	"testing"
)

func TestOAuthAccountLabel(t *testing.T) {
	jwtPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"jwt@example.com","name":"JWT User"}`))
	jwt := "eyJhbGciOiJub25lIn0." + jwtPayload + ".signature"

	cases := []struct {
		name     string
		provider string
		meta     string
		want     string
	}{
		{"antigravity userinfo", "antigravity", `{"userInfo":{"email":"a@example.com","name":"A"},"projectId":"p"}`, "a@example.com"},
		{"github login fallback", "github", `{"github_user":{"login":"octocat","email":""}}`, "octocat"},
		{"cline email", "cline", `{"email":"c@example.com"}`, "c@example.com"},
		{"kilocode email", "kilocode", `{"email":"k@example.com","orgId":"o"}`, "k@example.com"},
		{"codex id token", "codex", `{"id_token":"` + jwt + `"}`, "jwt@example.com"},
		{"cursor machine", "cursor", `{"machineId":"abcdef123456","authMethod":"imported"}`, "Cursor · abcdef12…"},
		{"email beats name", "xai", `{"name":"Name","email":"mail@example.com"}`, "mail@example.com"},
		{"empty meta", "xai", ``, ""},
		{"invalid meta", "xai", `not-json`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := oauthAccountLabel(tc.provider, tc.meta); got != tc.want {
				t.Fatalf("oauthAccountLabel(%q) = %q, want %q", tc.meta, got, tc.want)
			}
		})
	}
}
