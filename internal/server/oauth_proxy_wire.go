package server

import (
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/auth"
)

// applyConnectionAuth overlays OAuth IDE session tokens when connection.oauth_provider is set.
// Static api_key remains if OAuth is disabled, unresolved, or empty.
func (p *ProxyHandler) applyConnectionAuth(conn *Connection) {
	if conn == nil {
		return
	}
	oauthProvider := strings.TrimSpace(strings.ToLower(conn.OAuthProvider))
	if oauthProvider == "" {
		return
	}
	if p.oauthMgr == nil || p.cfg == nil || !p.cfg.OAuthIDEEnabled {
		return
	}
	cred, err := p.oauthMgr.ResolveUpstreamCredentialFull(oauthProvider, true)
	if err != nil || cred == nil || strings.TrimSpace(cred.Token) == "" {
		return
	}
	conn.APIKey = cred.Token
	if cred.AuthHeader != "" {
		conn.AuthHeader = cred.AuthHeader
	}
	if cred.AuthPrefix != "" {
		conn.AuthPrefix = cred.AuthPrefix
	}
}

func (p *ProxyHandler) connForUpstream(conn *Connection) *Connection {
	if conn == nil {
		return nil
	}
	c := *conn
	p.applyConnectionAuth(&c)
	return &c
}

// SetOAuthManager wires IDE OAuth sessions into the proxy (called from Server.New).
func (p *ProxyHandler) SetOAuthManager(m *auth.OAuthManager) {
	p.oauthMgr = m
}