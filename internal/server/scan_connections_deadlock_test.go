package server

// Regression test for the SQLite pool deadlock in findAlternateConnectionsForModel.
//
// WHY THIS EXISTS
//
// db.Open sets SetMaxOpenConns(1). scanConnections used to call
// applyConnectionAuth for every row WHILE the *sql.Rows from the same pool was
// still open. applyConnectionAuth -> oauthIdeEnabled -> DB.GetSetting asks the
// pool for a second connection, but the only connection is held by the open
// rowset, so database/sql blocks in DB.conn() with no timeout. The request
// hangs until the client gives up (observed: 60s+ timeouts, goroutine parked in
// database/sql.(*DB).conn with nothing logged).
//
// It only bit connections carrying oauth_provider (the gate short-circuits for
// every other row), which is why OAuth IDE providers — Antigravity in
// production — were the only ones that ever hung.
//
// The fix defers auth overlay until AFTER the rowset is closed. This test fails
// (times out) on the old code and passes on the fixed code.

import (
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

func TestFindAlternateConnectionsForModel_NoSQLitePoolDeadlock(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Two connections serving the same model: one plain, one OAuth-backed.
	// The OAuth one is the row that used to trigger the nested DB call.
	if _, err := database.Conn().Exec(
		`INSERT INTO connections (id, name, base_url, api_key, format, chat_path, auth_header, auth_prefix, is_active, priority, oauth_provider)
		 VALUES ('plain-1', 'plain', 'https://example.invalid', 'sk-plain', 'openai', '/v1/chat/completions', 'Authorization', 'Bearer ', 1, 1, '')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Conn().Exec(
		`INSERT INTO connections (id, name, base_url, api_key, format, chat_path, auth_header, auth_prefix, is_active, priority, oauth_provider)
		 VALUES ('oauth-1', 'oauth', 'https://example.invalid', '', 'openai', '/v1/chat/completions', 'Authorization', 'Bearer ', 1, 2, 'antigravity')`,
	); err != nil {
		t.Fatal(err)
	}
	for _, cid := range []string{"plain-1", "oauth-1"} {
		if _, err := database.Conn().Exec(
			`INSERT INTO discovered_models (connection_id, model_id, model_name, is_active) VALUES (?, 'test-model', 'test-model', 1)`,
			cid,
		); err != nil {
			t.Fatal(err)
		}
	}

	// OAuth IDE enabled in the DB setting: this is what makes applyConnectionAuth
	// go on to touch the DB a second time.
	if err := database.SetSetting("oauth_ide_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	mgr := auth.NewOAuthManager(database)
	p := NewProxyHandler(&config.Config{OAuthIDEEnabled: true}, database)
	p.SetOAuthManager(mgr)

	type result struct {
		conns []*Connection
	}
	done := make(chan result, 1)
	go func() {
		done <- result{conns: p.findAlternateConnectionsForModel("test-model", []string{"already-tried"})}
	}()

	select {
	case got := <-done:
		if len(got.conns) != 2 {
			t.Fatalf("alternates = %d, want 2", len(got.conns))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("findAlternateConnectionsForModel deadlocked on the SQLite pool " +
			"(nested DB call while the rowset still holds the only connection)")
	}
}
