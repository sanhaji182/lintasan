package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestOAuthAccountsExpiresStalePendingSessions(t *testing.T) {
	s, _ := newTestServer(t, &config.Config{OAuthIDEEnabled: true})

	if _, err := s.db.Conn().Exec(`INSERT INTO oauth_sessions (id, provider, access_token, status, created_at) VALUES
		('old-pending', 'xai', 'x', 'pending', datetime('now','localtime','-1 hour')),
		('fresh-pending', 'xai', 'x', 'pending', datetime('now','localtime')),
		('active', 'xai', 'x', 'active', datetime('now','localtime'))`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/oauth/accounts", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{Username: "admin", Role: "admin"}))
	rec := httptest.NewRecorder()
	s.handleOAuthAccounts(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("accounts: got %d (%s)", rec.Code, rec.Body.String())
	}

	statuses := map[string]string{}
	rows, err := s.db.Conn().Query(`SELECT id, status FROM oauth_sessions`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			t.Fatal(err)
		}
		statuses[id] = status
	}
	if statuses["old-pending"] != "expired" {
		t.Fatalf("old pending session = %q, want expired", statuses["old-pending"])
	}
	if statuses["fresh-pending"] != "pending" {
		t.Fatalf("fresh pending session = %q, want pending", statuses["fresh-pending"])
	}

	var body struct {
		Accounts []struct {
			ID     string `json:"id"`
			Health string `json:"health"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, account := range body.Accounts {
		if account.ID == "old-pending" && account.Health != "expired" {
			t.Fatalf("old pending health = %q, want expired", account.Health)
		}
	}
}

func TestOAuthIdeDisabledHTMLEscapesProviderErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	oauthIdeDisabledHTML(rec, `<script>alert("xss")</script>`)
	body := rec.Body.String()
	if strings.Contains(body, "<script>") {
		t.Fatalf("error page contains unescaped script: %s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("error page did not escape the message: %s", body)
	}
}

func TestOAuthRevokePurgesAlreadyRevokedSessions(t *testing.T) {
	s, _ := newTestServer(t, &config.Config{OAuthIDEEnabled: true})
	if _, err := s.db.Conn().Exec(`INSERT INTO oauth_sessions (id, provider, access_token, status) VALUES
		('active-1', 'xai', 'x', 'active'), ('revoked-1', 'xai', 'x', 'revoked')`); err != nil {
		t.Fatal(err)
	}
	admin := context.WithValue(context.Background(), auth.UserContextKey, &auth.User{Username: "admin", Role: "admin"})

	del := func(id string) *httptest.ResponseRecorder {
		req := reqWithPath(http.MethodDelete, "/api/oauth/sessions/"+id, nil, map[string]string{"id": id})
		rec := httptest.NewRecorder()
		s.handleOAuthRevokeSession(rec, req.WithContext(admin))
		return rec
	}

	rec := del("revoked-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("remove revoked: got %d (%s)", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "deleted" {
		t.Fatalf("remove revoked status = %q, want deleted", body["status"])
	}
	var count int
	s.db.Conn().QueryRow(`SELECT COUNT(*) FROM oauth_sessions WHERE id='revoked-1'`).Scan(&count)
	if count != 0 {
		t.Fatalf("revoked session row still exists")
	}

	rec = del("active-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke active: got %d (%s)", rec.Code, rec.Body.String())
	}
	var status string
	s.db.Conn().QueryRow(`SELECT status FROM oauth_sessions WHERE id='active-1'`).Scan(&status)
	if status != "revoked" {
		t.Fatalf("active session status = %q, want revoked", status)
	}
}
