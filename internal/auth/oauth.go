package auth

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/db"
)

// OAuthManager manages OAuth token sessions for IDE integrations.
type OAuthManager struct {
	db *db.DB
}

// OAuthSession represents an OAuth token session.
type OAuthSession struct {
	ID           string    `json:"id"`
	Provider     string    `json:"provider"` // 9router OAuth id: claude, xai, github, ...
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	PKCEVerifier string    `json:"-"` // never expose
	DeviceCode   string    `json:"-"`
	FlowMeta     string    `json:"-"`
	ExpiresAt    time.Time `json:"expires_at"`
	Status       string    `json:"status"` // "pending", "active", "expired", "revoked"
	CreatedAt    string    `json:"created_at"`
}

// NewOAuthManager creates a new OAuthManager.
func NewOAuthManager(database *db.DB) *OAuthManager {
	return &OAuthManager{db: database}
}

// CreateSession creates a new pending OAuth session.
func (m *OAuthManager) CreateSession(provider string) (*OAuthSession, error) {
	session := &OAuthSession{
		ID:        fmt.Sprintf("oauth_%s_%d", provider, time.Now().UnixNano()),
		Provider:  provider,
		Status:    "pending",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	_, err := m.db.Conn().Exec(
		`INSERT INTO oauth_sessions (id, provider, access_token, refresh_token, expires_at, status, created_at) VALUES (?, ?, '', '', ?, ?, datetime('now', 'localtime'))`,
		session.ID, session.Provider, session.ExpiresAt.Format(time.RFC3339), session.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("create oauth session: %w", err)
	}

	return session, nil
}

// GetActiveToken returns the active access token for a provider, if any.
func (m *OAuthManager) GetActiveToken(provider string) (string, error) {
	var token string
	var expiresAt string
	var status string

	err := m.db.Conn().QueryRow(
		`SELECT access_token, expires_at, status FROM oauth_sessions WHERE provider = ? AND status = 'active' ORDER BY created_at DESC LIMIT 1`,
		provider,
	).Scan(&token, &expiresAt, &status)

	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get active token: %w", err)
	}

	// Check expiration
	if expiresAt != "" {
		exp, parseErr := time.Parse(time.RFC3339, expiresAt)
		if parseErr == nil && time.Now().After(exp) {
			// Mark as expired
			m.db.Conn().Exec(
				`UPDATE oauth_sessions SET status = 'expired' WHERE provider = ? AND access_token = ?`,
				provider, token,
			)
			return "", nil
		}
	}

	return token, nil
}

// RefreshToken exchanges refresh_token with the provider when supported.
func (m *OAuthManager) RefreshToken(provider string) error {
	if m == nil || m.db == nil {
		return fmt.Errorf("oauth manager not configured")
	}
	provider = strings.TrimSpace(strings.ToLower(provider))

	var sessionID, refreshToken string
	err := m.db.Conn().QueryRow(
		`SELECT id, refresh_token FROM oauth_sessions WHERE provider = ? AND status = 'active' ORDER BY datetime(created_at) DESC LIMIT 1`,
		provider,
	).Scan(&sessionID, &refreshToken)
	if err == sql.ErrNoRows {
		return fmt.Errorf("no active session for provider %s", provider)
	}
	if err != nil {
		return fmt.Errorf("refresh token: %w", err)
	}
	if refreshToken == "" {
		return fmt.Errorf("no refresh token available for provider %s", provider)
	}

	tok, expiresAt, err := refreshOAuthProvider(provider, refreshToken)
	if err != nil {
		return err
	}
	if tok == nil || tok.Access == "" {
		return fmt.Errorf("refresh returned empty access token for %s", provider)
	}
	_, err = m.db.Conn().Exec(
		`UPDATE oauth_sessions SET access_token = ?, refresh_token = CASE WHEN ? != '' THEN ? ELSE refresh_token END, expires_at = ?, status = 'active' WHERE id = ?`,
		tok.Access, tok.Refresh, tok.Refresh, expiresAt.Format(time.RFC3339), sessionID,
	)
	return err
}

// RevokeToken marks a provider's token as revoked.
func (m *OAuthManager) RevokeToken(provider string) error {
	_, err := m.db.Conn().Exec(
		`UPDATE oauth_sessions SET status = 'revoked' WHERE provider = ? AND status = 'active'`,
		provider,
	)
	return err
}

// RevokeSession revokes a specific OAuth session by ID.
func (m *OAuthManager) RevokeSession(id string) error {
	_, err := m.db.Conn().Exec(
		`UPDATE oauth_sessions SET status = 'revoked' WHERE id = ?`,
		id,
	)
	return err
}

// UpdateSessionTokens stores access and refresh tokens for a session.
func (m *OAuthManager) UpdateSessionTokens(id, accessToken, refreshToken string, expiresAt time.Time) error {
	_, err := m.db.Conn().Exec(
		`UPDATE oauth_sessions SET access_token = ?, refresh_token = ?, expires_at = ?, status = 'active' WHERE id = ?`,
		accessToken, refreshToken, expiresAt.Format(time.RFC3339), id,
	)
	return err
}

// ListSessions returns all non-revoked OAuth sessions.
func (m *OAuthManager) ListSessions() ([]OAuthSession, error) {
	rows, err := m.db.Conn().Query(
		`SELECT id, provider, access_token, refresh_token, expires_at, status, created_at FROM oauth_sessions WHERE status != 'revoked' ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []OAuthSession
	for rows.Next() {
		var s OAuthSession
		var accessToken, refreshToken, expiresAtStr, createdAt sql.NullString
		err := rows.Scan(&s.ID, &s.Provider, &accessToken, &refreshToken, &expiresAtStr, &s.Status, &createdAt)
		if err != nil {
			continue
		}
		if accessToken.Valid {
			s.AccessToken = maskToken(accessToken.String)
		}
		if refreshToken.Valid {
			s.RefreshToken = maskToken(refreshToken.String)
		}
		if expiresAtStr.Valid {
			s.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAtStr.String)
		}
		if createdAt.Valid {
			s.CreatedAt = createdAt.String
		}
		sessions = append(sessions, s)
	}

	if sessions == nil {
		sessions = []OAuthSession{}
	}

	return sessions, nil
}

// maskToken masks a token for display, showing only first 4 and last 4 characters.
func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return token[:1] + "***" + token[len(token)-1:]
	}
	return token[:4] + "..." + token[len(token)-4:]
}
