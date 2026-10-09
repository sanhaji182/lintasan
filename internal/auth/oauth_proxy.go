package auth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UpstreamCredential is the bearer (or raw) secret to attach on upstream requests.
type UpstreamCredential struct {
	Token      string
	AuthHeader string // empty => Authorization
	AuthPrefix string // empty => Bearer
	// SessionID identifies the account this token came from, so the proxy can
	// report a rejection against the right account (see MarkOAuthAccountRejected).
	SessionID string
}

const oauthRefreshSkew = 5 * time.Minute

// ResolveUpstreamCredential returns the active OAuth IDE token for proxy use.
// When OAuth IDE is disabled, returns (\"\", nil) so static api_key on the connection wins.
func (m *OAuthManager) ResolveUpstreamCredential(provider string, oauthIdeEnabled bool) (string, error) {
	cred, err := m.ResolveUpstreamCredentialFull(provider, oauthIdeEnabled)
	if err != nil || cred == nil {
		return "", err
	}
	return cred.Token, nil
}

// ResolveUpstreamCredentialFull picks an account for the provider by round-robin
// over its active sessions (see oauth_pool.go) and returns the bearer to attach,
// with auth header/prefix hints (github uses api-key style).
//
// This used to take the single most-recent active session, which made a
// multi-account provider behave as one account. It now rotates, so a rejected
// account no longer takes the whole provider down with it.
func (m *OAuthManager) ResolveUpstreamCredentialFull(provider string, oauthIdeEnabled bool) (*UpstreamCredential, error) {
	if !oauthIdeEnabled || m == nil || m.db == nil {
		return nil, nil
	}
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" || !IsIdeOAuthProvider(provider) {
		return nil, nil
	}

	sess, err := m.pickOAuthSession(provider)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, nil
	}

	// Expired (or about to expire) with a refresh token: refresh THIS session so
	// the rotation stays on the account it picked.
	if sess.RefreshToken != "" &&
		(!sess.ExpiresAt.IsZero() && time.Until(sess.ExpiresAt) < oauthRefreshSkew) {
		if err := m.refreshOAuthSession(sess); err == nil {
			if fresh, ferr := m.sessionByID(sess.ID); ferr == nil && fresh != nil {
				sess = fresh
			}
		} else if !sess.ExpiresAt.IsZero() && time.Now().After(sess.ExpiresAt) {
			// This account is unusable, but another active account may be healthy.
			// Mark only this session expired and retry selection once; do not
			// collapse the entire provider to nil because one account failed.
			_ = m.markSessionExpiredByID(sess.ID)
			return m.ResolveUpstreamCredentialFull(provider, oauthIdeEnabled)
		}
	} else if sess.RefreshToken == "" && !sess.ExpiresAt.IsZero() && time.Now().After(sess.ExpiresAt) {
		// An expired session without a refresh token is equally unusable. The
		// old code would still return its token forever; remove it and rotate.
		_ = m.markSessionExpiredByID(sess.ID)
		return m.ResolveUpstreamCredentialFull(provider, oauthIdeEnabled)
	}

	if strings.TrimSpace(sess.AccessToken) == "" {
		return nil, nil
	}
	return credentialForSession(sess), nil
}

// credentialForSession maps a session to the bearer/header hints the upstream
// expects. GitHub carries the short-lived copilot token inside flow_meta.
func credentialForSession(sess *OAuthSession) *UpstreamCredential {
	if sess == nil {
		return nil
	}
	token := strings.TrimSpace(sess.AccessToken)
	if sess.Provider == "github" {
		if t := copilotTokenFromFlowMeta(sess.FlowMeta); t != "" {
			token = t
		}
	}
	return &UpstreamCredential{
		Token:      token,
		AuthHeader: "Authorization",
		AuthPrefix: "Bearer ",
		SessionID:  sess.ID,
	}
}

// sessionByID loads one session by id (any status).
func (m *OAuthManager) sessionByID(id string) (*OAuthSession, error) {
	var s OAuthSession
	var access, refresh, expiresAt, createdAt, flowMeta sql.NullString
	err := m.db.Conn().QueryRow(
		`SELECT id, provider, access_token, refresh_token, expires_at, status, created_at, flow_meta
		 FROM oauth_sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.Provider, &access, &refresh, &expiresAt, &s.Status, &createdAt, &flowMeta)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("oauth session by id: %w", err)
	}
	s.AccessToken = access.String
	s.RefreshToken = refresh.String
	s.FlowMeta = flowMeta.String
	if expiresAt.String != "" {
		s.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt.String)
	}
	s.CreatedAt = createdAt.String
	return &s, nil
}

// refreshOAuthSession refreshes ONE session (the account the pool picked),
// preserving its identity so rotation state and health stay attached to the
// right account. Returns an error when the provider has no refresh path or the
// session carries no refresh token.
func (m *OAuthManager) refreshOAuthSession(sess *OAuthSession) error {
	if m == nil || m.db == nil || sess == nil {
		return fmt.Errorf("oauth manager not configured")
	}
	if strings.TrimSpace(sess.RefreshToken) == "" {
		return fmt.Errorf("no refresh token for session %s", sess.ID)
	}
	tok, expiresAt, err := refreshOAuthProvider(sess.Provider, sess.RefreshToken)
	if err != nil {
		return err
	}
	if tok == nil || tok.Access == "" {
		return fmt.Errorf("refresh returned empty access token for %s", sess.Provider)
	}
	_, err = m.db.Conn().Exec(
		`UPDATE oauth_sessions SET access_token = ?, refresh_token = CASE WHEN ? != '' THEN ? ELSE refresh_token END, expires_at = ?, status = ? WHERE id = ?`,
		tok.Access, tok.Refresh, tok.Refresh, expiresAt.Format(time.RFC3339), SessionStatusActive, sess.ID,
	)
	return err
}

func (m *OAuthManager) getLatestActiveSession(provider string) (*OAuthSession, error) {
	var s OAuthSession
	var access, refresh, expiresAt, createdAt, flowMeta string
	err := m.db.Conn().QueryRow(
		`SELECT id, provider, access_token, refresh_token, expires_at, status, created_at, flow_meta
		 FROM oauth_sessions WHERE provider = ? AND status = 'active'
		 ORDER BY datetime(created_at) DESC LIMIT 1`,
		provider,
	).Scan(&s.ID, &s.Provider, &access, &refresh, &expiresAt, &s.Status, &createdAt, &flowMeta)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("oauth active session: %w", err)
	}
	s.AccessToken = access
	s.RefreshToken = refresh
	s.FlowMeta = flowMeta
	if expiresAt != "" {
		s.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	}
	s.CreatedAt = createdAt
	return &s, nil
}

func (m *OAuthManager) markSessionExpired(provider, accessToken string) error {
	_, err := m.db.Conn().Exec(
		`UPDATE oauth_sessions SET status = 'expired' WHERE provider = ? AND access_token = ? AND status = 'active'`,
		provider, accessToken,
	)
	return err
}

// markSessionExpiredByID updates one account by stable identity. Access tokens
// can rotate and are not a safe account key for multi-session failover.
func (m *OAuthManager) markSessionExpiredByID(sessionID string) error {
	_, err := m.db.Conn().Exec(
		`UPDATE oauth_sessions SET status = 'expired' WHERE id = ? AND status = 'active'`,
		sessionID,
	)
	return err
}

func copilotTokenFromFlowMeta(flowMetaJSON string) string {
	if flowMetaJSON == "" {
		return ""
	}
	var meta map[string]any
	if json.Unmarshal([]byte(flowMetaJSON), &meta) != nil {
		return ""
	}
	copilot, _ := meta["copilot"].(map[string]any)
	if copilot == nil {
		return ""
	}
	if t, _ := copilot["token"].(string); t != "" {
		return t
	}
	return ""
}
