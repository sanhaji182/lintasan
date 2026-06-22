package server

import (
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

func TestApplyConnectionAuth_OverlaysOAuth(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	mgr := auth.NewOAuthManager(database)
	sess, err := mgr.CreateSession("xai")
	if err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(2 * time.Hour)
	if err := mgr.UpdateSessionTokens(sess.ID, "oauth_xai_token", "", exp); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{OAuthIDEEnabled: true}
	p := NewProxyHandler(cfg, database)
	p.SetOAuthManager(mgr)

	conn := &Connection{ID: "c1", APIKey: "static-key", OAuthProvider: "xai"}
	p.applyConnectionAuth(conn)
	if conn.APIKey != "oauth_xai_token" {
		t.Fatalf("api_key = %q", conn.APIKey)
	}

	pDisabled := NewProxyHandler(&config.Config{OAuthIDEEnabled: false}, database)
	pDisabled.SetOAuthManager(mgr)
	connStatic := &Connection{APIKey: "static-key", OAuthProvider: "xai"}
	pDisabled.applyConnectionAuth(connStatic)
	if connStatic.APIKey != "static-key" {
		t.Fatalf("flag off should keep static key, got %q", connStatic.APIKey)
	}
}