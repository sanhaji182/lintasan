package auth

import (
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/db"
)

func TestCopilotTokenFromFlowMeta(t *testing.T) {
	meta := `{"copilot":{"token":"ghu_copilot_secret"}}`
	if got := copilotTokenFromFlowMeta(meta); got != "ghu_copilot_secret" {
		t.Fatalf("got %q", got)
	}
	if copilotTokenFromFlowMeta("") != "" {
		t.Fatal("empty meta")
	}
}

func TestResolveUpstreamCredential_OAuthDisabled(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	m := NewOAuthManager(database)
	_, err = m.db.Conn().Exec(
		`INSERT INTO oauth_sessions (id, provider, access_token, refresh_token, expires_at, status, created_at)
		 VALUES ('s1', 'xai', 'tok_live', '', ?, 'active', datetime('now'))`,
		time.Now().Add(time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := m.ResolveUpstreamCredential("xai", false)
	if err != nil || tok != "" {
		t.Fatalf("want empty when disabled, got %q err=%v", tok, err)
	}
}

func TestResolveUpstreamCredential_ActiveSession(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	m := NewOAuthManager(database)
	exp := time.Now().Add(2 * time.Hour).Format(time.RFC3339)
	_, err = m.db.Conn().Exec(
		`INSERT INTO oauth_sessions (id, provider, access_token, refresh_token, expires_at, status, created_at, flow_meta)
		 VALUES ('s1', 'xai', 'tok_live', '', ?, 'active', datetime('now'), '')`,
		exp,
	)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := m.ResolveUpstreamCredential("xai", true)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "tok_live" {
		t.Fatalf("got %q", tok)
	}
}

func TestResolveUpstreamCredential_ExpiredWithoutRefreshDoesNotRecurse(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	m := NewOAuthManager(database)
	_, err = m.db.Conn().Exec(
		`INSERT INTO oauth_sessions (id, provider, access_token, refresh_token, expires_at, status, created_at)
		 VALUES ('old', 'xai', 'stale', '', ?, 'active', datetime('now', 'localtime'))`,
		time.Now().Add(-time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var got *UpstreamCredential
	var gotErr error
	go func() {
		got, gotErr = m.ResolveUpstreamCredentialFull("xai", true)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ResolveUpstreamCredentialFull did not return — expired-session retry is looping")
	}
	if gotErr != nil || got != nil {
		t.Fatalf("want no credential, got %+v err=%v", got, gotErr)
	}

	var status string
	if err := m.db.Conn().QueryRow(`SELECT status FROM oauth_sessions WHERE id='old'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "expired" {
		t.Fatalf("status = %q, want expired", status)
	}
}

func TestResolveUpstreamCredential_GitHubCopilotMeta(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	m := NewOAuthManager(database)
	meta := `{"copilot":{"token":"copilot_bearer"}}`
	exp := time.Now().Add(2 * time.Hour).Format(time.RFC3339)
	_, err = m.db.Conn().Exec(
		`INSERT INTO oauth_sessions (id, provider, access_token, refresh_token, expires_at, status, created_at, flow_meta)
		 VALUES ('s1', 'github', 'gh_oauth', '', ?, 'active', datetime('now'), ?)`,
		exp, meta,
	)
	if err != nil {
		t.Fatal(err)
	}
	cred, err := m.ResolveUpstreamCredentialFull("github", true)
	if err != nil || cred == nil || cred.Token != "copilot_bearer" {
		t.Fatalf("cred=%v err=%v", cred, err)
	}
}

func TestResolveUpstreamCredential_NousAgentKeyMeta(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	m := NewOAuthManager(database)
	meta := `{"agent_key":"nk-hermes-agent-key-secret"}`
	exp := time.Now().Add(2 * time.Hour).Format(time.RFC3339)
	_, err = m.db.Conn().Exec(
		`INSERT INTO oauth_sessions (id, provider, access_token, refresh_token, expires_at, status, created_at, flow_meta)
		 VALUES ('s1', 'nous', 'oauth_access_tok', 'oauth_refresh_tok', ?, 'active', datetime('now'), ?)`,
		exp, meta,
	)
	if err != nil {
		t.Fatal(err)
	}
	cred, err := m.ResolveUpstreamCredentialFull("nous", true)
	if err != nil || cred == nil || cred.Token != "nk-hermes-agent-key-secret" {
		t.Fatalf("cred=%v err=%v, want agent_key", cred, err)
	}
}
