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
}

const oauthRefreshSkew = 5 * time.Minute

// ResolveUpstreamCredential returns the active OAuth IDE token for proxy use.
// When OAuth IDE is disabled, returns ("", nil) so static api_key on the connection wins.
func (m *OAuthManager) ResolveUpstreamCredential(provider string, oauthIdeEnabled bool) (string, error) {
	cred, err := m.ResolveUpstreamCredentialFull(provider, oauthIdeEnabled)
	if err != nil || cred == nil {
		return "", err
	}
	return cred.Token, nil
}

// ResolveUpstreamCredentialFull includes auth header/prefix hints (github uses api-key style).
func (m *OAuthManager) ResolveUpstreamCredentialFull(provider string, oauthIdeEnabled bool) (*UpstreamCredential, error) {
	if !oauthIdeEnabled || m == nil || m.db == nil {
		return nil, nil
	}
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" || !IsIdeOAuthProvider(provider) {
		return nil, nil
	}

	sess, err := m.getLatestActiveSession(provider)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, nil
	}

	if !sess.ExpiresAt.IsZero() && time.Now().After(sess.ExpiresAt) {
		_ = m.markSessionExpired(provider, sess.AccessToken)
		if err := m.RefreshToken(provider); err == nil {
			sess, err = m.getLatestActiveSession(provider)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, nil
		}
	}

	if sess == nil || strings.TrimSpace(sess.AccessToken) == "" {
		return nil, nil
	}

	if !sess.ExpiresAt.IsZero() && time.Until(sess.ExpiresAt) < oauthRefreshSkew && sess.RefreshToken != "" {
		_ = m.RefreshToken(provider) // best-effort; use current token if refresh fails
		if refreshed, _ := m.getLatestActiveSession(provider); refreshed != nil && refreshed.AccessToken != "" {
			sess = refreshed
		}
	}

	token := strings.TrimSpace(sess.AccessToken)
	switch provider {
	case "github":
		if t := copilotTokenFromFlowMeta(sess.FlowMeta); t != "" {
			token = t
		}
		return &UpstreamCredential{
			Token:      token,
			AuthHeader: "Authorization",
			AuthPrefix: "Bearer ",
		}, nil
	default:
		return &UpstreamCredential{
			Token:      token,
			AuthHeader: "Authorization",
			AuthPrefix: "Bearer ",
		}, nil
	}
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