package server

// The dashboard must be able to say "login ulang", not just "wired".
//
// Before this, /api/oauth/accounts reported `wired: true, total: 0` for a
// provider whose sessions were all gone, and the page rendered a generic
// "wired but no usable account" line that never named the required action. The
// connection's last_error_code is the fact that turns it into an instruction.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

func TestOAuthAccounts_ExposesReauthCode(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := database.SetSetting("oauth_ide_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Conn().Exec(
		`INSERT INTO connections (id, name, base_url, api_key, format, is_active, priority, oauth_provider, last_error_code, last_error_model, last_error_at)
		 VALUES ('ag-1', 'oauth-antigravity', 'https://example.invalid', '', 'openai', 1, 1, 'antigravity',
		         'oauth_reauth_required', 'gemini-2.5-flash', '2026-11-16 16:04:59')`,
	); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: database, cfg: &config.Config{OAuthIDEEnabled: true}}
	// requireAdmin needs an admin in the request context; this test exercises the
	// payload shape, so bypass the gate the same way the root admin does.
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/accounts", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey,
		&auth.User{Username: "admin", Role: "admin"}))
	rec := httptest.NewRecorder()

	s.handleOAuthAccounts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Providers []struct {
			Provider       string `json:"provider"`
			Wired          bool   `json:"wired"`
			Total          int    `json:"total"`
			LastErrorCode  string `json:"last_error_code"`
			LastErrorModel string `json:"last_error_model"`
			LastErrorAt    string `json:"last_error_at"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	var found bool
	for _, p := range got.Providers {
		if p.Provider != "antigravity" {
			continue
		}
		found = true
		if !p.Wired {
			t.Error("wired = false, want true")
		}
		if p.LastErrorCode != "oauth_reauth_required" {
			t.Errorf("last_error_code = %q, want oauth_reauth_required", p.LastErrorCode)
		}
		if p.LastErrorModel != "gemini-2.5-flash" {
			t.Errorf("last_error_model = %q", p.LastErrorModel)
		}
		if p.LastErrorAt == "" {
			t.Error("last_error_at empty")
		}
	}
	if !found {
		t.Fatal("antigravity provider missing from payload")
	}
}
