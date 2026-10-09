package auth

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/db"
)

func newOAuthPoolTestManager(t *testing.T) *OAuthManager {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "oauth-pool.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return NewOAuthManager(database)
}

func insertOAuthPoolSession(t *testing.T, m *OAuthManager, id, provider, token string) {
	t.Helper()
	_, err := m.db.Conn().Exec(
		`INSERT INTO oauth_sessions
		 (id, provider, access_token, refresh_token, expires_at, status, created_at, flow_meta)
		 VALUES (?, ?, ?, '', ?, 'active', datetime('now'), '')`,
		id, provider, token, time.Now().Add(time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func resetOAuthPoolTestState() {
	oauthPoolMu.Lock()
	defer oauthPoolMu.Unlock()
	oauthPoolCursors = map[string]*poolCursor{}
	oauthPoolCooldown = map[string]map[string]time.Time{}
}

func TestOAuthPoolRoundRobinAcrossActiveAccounts(t *testing.T) {
	resetOAuthPoolTestState()
	m := newOAuthPoolTestManager(t)
	for i := 1; i <= 3; i++ {
		insertOAuthPoolSession(t, m, fmt.Sprintf("s%d", i), "codex", fmt.Sprintf("token-%d", i))
	}

	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		cred, err := m.ResolveUpstreamCredentialFull("codex", true)
		if err != nil {
			t.Fatal(err)
		}
		if cred == nil {
			t.Fatal("nil credential")
		}
		seen[cred.SessionID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("round-robin picked %d distinct accounts, want 3: %#v", len(seen), seen)
	}
}

func TestOAuthPoolRestrictedAccountIsSkipped(t *testing.T) {
	resetOAuthPoolTestState()
	m := newOAuthPoolTestManager(t)
	insertOAuthPoolSession(t, m, "s1", "codex", "token-1")
	insertOAuthPoolSession(t, m, "s2", "codex", "token-2")

	first, err := m.ResolveUpstreamCredentialFull("codex", true)
	if err != nil || first == nil {
		t.Fatalf("first resolve: cred=%v err=%v", first, err)
	}
	m.MarkOAuthAccountRejected("codex", first.SessionID, true)

	second, err := m.ResolveUpstreamCredentialFull("codex", true)
	if err != nil || second == nil {
		t.Fatalf("second resolve: cred=%v err=%v", second, err)
	}
	if second.SessionID == first.SessionID {
		t.Fatalf("restricted account %q selected again", first.SessionID)
	}
	var status string
	if err := m.db.Conn().QueryRow(`SELECT status FROM oauth_sessions WHERE id=?`, first.SessionID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != SessionStatusRestricted {
		t.Fatalf("status=%q, want restricted", status)
	}
}

func TestOAuthPoolSingleAccountPreservesBehavior(t *testing.T) {
	resetOAuthPoolTestState()
	m := newOAuthPoolTestManager(t)
	insertOAuthPoolSession(t, m, "only", "codex", "single-token")

	for i := 0; i < 3; i++ {
		cred, err := m.ResolveUpstreamCredentialFull("codex", true)
		if err != nil || cred == nil {
			t.Fatalf("resolve %d: cred=%v err=%v", i, cred, err)
		}
		if cred.Token != "single-token" || cred.SessionID != "only" {
			t.Fatalf("resolve %d: got token=%q session=%q", i, cred.Token, cred.SessionID)
		}
	}
}

func TestOAuthPoolDisabledReturnsNil(t *testing.T) {
	resetOAuthPoolTestState()
	m := newOAuthPoolTestManager(t)
	insertOAuthPoolSession(t, m, "only", "codex", "single-token")
	cred, err := m.ResolveUpstreamCredentialFull("codex", false)
	if err != nil || cred != nil {
		t.Fatalf("disabled: cred=%v err=%v, want nil nil", cred, err)
	}
}

func TestOAuthPoolSkipsExpiredAccountWithoutRefresh(t *testing.T) {
	resetOAuthPoolTestState()
	m := newOAuthPoolTestManager(t)
	insertOAuthPoolSession(t, m, "expired", "codex", "expired-token")
	insertOAuthPoolSession(t, m, "healthy", "codex", "healthy-token")
	if _, err := m.db.Conn().Exec(
		`UPDATE oauth_sessions SET expires_at=?, refresh_token='' WHERE id='expired'`,
		time.Now().Add(-time.Hour).Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}

	// Cursor starts at the deterministic first session; make the expired row sort
	// first explicitly, then prove resolution continues to the healthy account.
	if _, err := m.db.Conn().Exec(`UPDATE oauth_sessions SET created_at='2099-01-01 00:00:00' WHERE id='expired'`); err != nil {
		t.Fatal(err)
	}
	cred, err := m.ResolveUpstreamCredentialFull("codex", true)
	if err != nil || cred == nil {
		t.Fatalf("resolve: cred=%v err=%v", cred, err)
	}
	if cred.SessionID != "healthy" || cred.Token != "healthy-token" {
		t.Fatalf("got session=%q token=%q, want healthy", cred.SessionID, cred.Token)
	}
	var status string
	if err := m.db.Conn().QueryRow(`SELECT status FROM oauth_sessions WHERE id='expired'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != SessionStatusExpired {
		t.Fatalf("expired account status=%q", status)
	}
}
