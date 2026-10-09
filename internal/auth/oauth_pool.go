package auth

import (
	"database/sql"
	"sync"
	"time"
)

// oauth_pool.go — round-robin + failover across the MULTIPLE active OAuth
// accounts a provider can have.
//
// WHY THIS EXISTS
//
// Before this file, ResolveUpstreamCredentialFull asked the DB for the single
// most-recently-created active session per provider (LIMIT 1). A provider with
// three logged-in accounts therefore behaved as if it had one: if that account's
// token came back 401/403/429, the resolver simply handed back nothing and the
// request failed, even though two healthy accounts sat right there in
// oauth_sessions. CLIProxyAPI's whole value is that it load-balances across CLI
// accounts; a one-account resolver cannot.
//
// WHAT THIS ADDS
//
//   - A per-provider rotation cursor, so successive resolutions land on
//     DIFFERENT accounts (round-robin) rather than always the newest.
//   - Failure marking: a session that the upstream rejected is taken out of
//     rotation for a cooldown, so the cursor moves on instead of knocking on the
//     same locked door.
//   - A "restricted" state for accounts the upstream appears to have gated
//     (as opposed to a merely expired/refreshable token). Restricted accounts are
//     NOT auto-retried: silently rotating around a restricted account would hide
//     the very problem an operator needs to see. They stay visible in the
//     session list and in the health view.
//
// SAFETY
//
// Additive and inert with a single account: the rotation cursor degenerates to
// "the one active session", which is exactly the previous behaviour. Sessions
// in cooldown are skipped only while other healthy accounts exist; if every
// account is cooling down the resolver falls back to the newest active session
// so a transient cooldown cannot turn a working setup into a hard failure.

const (
	// SessionStatusActive is a usable account.
	SessionStatusActive = "active"
	// SessionStatusExpired is an access token past its expiry (refreshable).
	SessionStatusExpired = "expired"
	// SessionStatusRevoked is removed by the operator.
	SessionStatusRevoked = "revoked"
	// SessionStatusRestricted marks an account the upstream appears to have
	// gated (repeated auth rejections). Deliberately NOT auto-retried.
	SessionStatusRestricted = "restricted"
)

// oauthAccountCooldown is how long a rejected account stays out of rotation.
// Long enough that a request burst cannot hammer a dead key, short enough that a
// recovered account rejoins within a normal session.
const oauthAccountCooldown = 5 * time.Minute

// poolCursor is the rotation state for one provider.
type poolCursor struct {
	next int
}

var (
	oauthPoolMu      sync.Mutex
	oauthPoolCursors = map[string]*poolCursor{}
	// oauthPoolCooldown[provider][sessionID] = time until which the account is
	// skipped. In-memory on purpose: a restart clears cooldowns, which is the
	// safe default (re-try everything once) and keeps this off the DB hot path.
	oauthPoolCooldown = map[string]map[string]time.Time{}
)

// pickOAuthSession selects an active session for provider using round-robin over
// the healthy accounts. It returns (nil, nil) when the provider has no active
// session at all, matching the old resolver's contract.
//
// Ordering is deterministic (created_at DESC, id) so the round-robin is stable
// across processes and testable. Accounts in cooldown are skipped unless every
// account is cooling down, in which case the newest is returned so a transient
// cooldown cannot hard-fail a working provider.
func (m *OAuthManager) pickOAuthSession(provider string) (*OAuthSession, error) {
	sessions, err := m.listActiveSessions(provider)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}

	now := time.Now()
	healthy := make([]OAuthSession, 0, len(sessions))
	for _, s := range sessions {
		if oauthSessionInCooldown(provider, s.ID, now) {
			continue
		}
		healthy = append(healthy, s)
	}
	// Every account cooling down → do not hard-fail; fall back to the newest.
	if len(healthy) == 0 {
		return &sessions[0], nil
	}

	idx := rotateCursor(provider, len(healthy))
	return &healthy[idx], nil
}

// listActiveSessions returns every active session for a provider, newest first.
// The ORDER BY includes id as a tiebreaker so two sessions created in the same
// second still rotate predictably.
func (m *OAuthManager) listActiveSessions(provider string) ([]OAuthSession, error) {
	rows, err := m.db.Conn().Query(
		`SELECT id, provider, access_token, refresh_token, expires_at, status, created_at, flow_meta
		 FROM oauth_sessions WHERE provider = ? AND status = ?
		 ORDER BY datetime(created_at) DESC, id ASC`,
		provider, SessionStatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OAuthSession
	for rows.Next() {
		var s OAuthSession
		var access, refresh, expiresAt, createdAt, flowMeta sql.NullString
		if err := rows.Scan(&s.ID, &s.Provider, &access, &refresh, &expiresAt, &s.Status, &createdAt, &flowMeta); err != nil {
			return nil, err
		}
		s.AccessToken = access.String
		s.RefreshToken = refresh.String
		s.FlowMeta = flowMeta.String
		if expiresAt.String != "" {
			s.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt.String)
		}
		s.CreatedAt = createdAt.String
		out = append(out, s)
	}
	return out, rows.Err()
}

// rotateCursor advances the per-provider cursor and returns an index into a
// pool of size n. The cursor is only advanced on a successful pick (here), so
// concurrent resolutions still spread across accounts instead of all reading the
// same index.
func rotateCursor(provider string, n int) int {
	oauthPoolMu.Lock()
	defer oauthPoolMu.Unlock()
	c := oauthPoolCursors[provider]
	if c == nil {
		c = &poolCursor{}
		oauthPoolCursors[provider] = c
	}
	idx := c.next % n
	c.next = (c.next + 1) % n
	return idx
}

// markOAuthAccountFailed takes a rejected session out of rotation. restricted
// also flips the DB status so the account is visible as gated and is skipped by
// every subsequent query — this is the "do not hide upstream restriction" rule:
// the account does not silently keep being retried, and it stays reportable.
func (m *OAuthManager) markOAuthAccountFailed(provider, sessionID string, restricted bool) {
	if provider == "" || sessionID == "" {
		return
	}
	oauthPoolMu.Lock()
	if oauthPoolCooldown[provider] == nil {
		oauthPoolCooldown[provider] = map[string]time.Time{}
	}
	oauthPoolCooldown[provider][sessionID] = time.Now().Add(oauthAccountCooldown)
	oauthPoolMu.Unlock()

	if restricted {
		_, _ = m.db.Conn().Exec(
			`UPDATE oauth_sessions SET status = ? WHERE id = ? AND status = ?`,
			SessionStatusRestricted, sessionID, SessionStatusActive,
		)
	}
}

// oauthSessionInCooldown reports whether a session is currently skipped.
func oauthSessionInCooldown(provider, sessionID string, now time.Time) bool {
	oauthPoolMu.Lock()
	defer oauthPoolMu.Unlock()
	byProv := oauthPoolCooldown[provider]
	if byProv == nil {
		return false
	}
	until, ok := byProv[sessionID]
	if !ok {
		return false
	}
	if now.After(until) {
		delete(byProv, sessionID)
		return false
	}
	return true
}

// MarkOAuthAccountRejected is the exported entry point for the proxy: it records
// that the upstream rejected this account's token. restricted is true for a hard
// auth failure (401/403) — the account is treated as gated; false for a soft
// failure (429/quota) where the account may recover on its own.
func (m *OAuthManager) MarkOAuthAccountRejected(provider, sessionID string, restricted bool) {
	m.markOAuthAccountFailed(provider, sessionID, restricted)
}
