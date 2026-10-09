package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

func newOAuthGateProxyForTest(t *testing.T, envEnabled bool) (*ProxyHandler, *db.DB) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "gate.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	cfg := &config.Config{OAuthIDEEnabled: envEnabled}
	p := &ProxyHandler{cfg: cfg, db: database, oauthMgr: auth.NewOAuthManager(database)}
	return p, database
}

func TestOAuthWireUsesDBGateWhenEnvUnset(t *testing.T) {
	p, database := newOAuthGateProxyForTest(t, false)
	if err := database.SetSetting(oauthIdeSettingKey, "true"); err != nil {
		t.Fatal(err)
	}
	_, err := database.Conn().Exec(
		`INSERT INTO oauth_sessions (id,provider,access_token,refresh_token,expires_at,status,created_at,flow_meta)
		 VALUES ('gate-session','codex','db-token','',?,'active',datetime('now'),'')`,
		time.Now().Add(time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatal(err)
	}
	conn := &Connection{OAuthProvider: "codex", APIKey: "static-token"}
	p.applyConnectionAuth(conn)
	if conn.APIKey != "db-token" {
		t.Fatalf("APIKey=%q, want DB-enabled OAuth token", conn.APIKey)
	}
	if conn.oauthSessionID != "gate-session" {
		t.Fatalf("oauthSessionID=%q", conn.oauthSessionID)
	}
}

func TestOAuthWireDBFalseOverridesEnvTrue(t *testing.T) {
	p, database := newOAuthGateProxyForTest(t, true)
	if err := database.SetSetting(oauthIdeSettingKey, "false"); err != nil {
		t.Fatal(err)
	}
	conn := &Connection{OAuthProvider: "codex", APIKey: "static-token"}
	p.applyConnectionAuth(conn)
	if conn.APIKey != "static-token" {
		t.Fatalf("APIKey=%q, want static token while DB gate false", conn.APIKey)
	}
}

func TestMaskTokenNeverRevealsShortSecret(t *testing.T) {
	if got := maskToken("shortsecret"); got != "••••" {
		t.Fatalf("short secret mask=%q", got)
	}
	if got := maskToken("abcd1234567890wxyz"); got != "abcd…wxyz" {
		t.Fatalf("long secret mask=%q", got)
	}
}
