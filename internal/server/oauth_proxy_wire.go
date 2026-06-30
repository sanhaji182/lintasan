package server

import (
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/lb"
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

// initMultiAccountPools loads all connections with pool_id, groups them,
// and creates a MultiAccountLB instance per pool.
func (p *ProxyHandler) initMultiAccountPools() {
	p.mabMu.Lock()
	defer p.mabMu.Unlock()

	p.multiAccountLBs = make(map[string]*lb.MultiAccountLB)

	rows, err := p.db.Conn().Query(
		`SELECT id, name, api_key, pool_id, priority, is_active
		 FROM connections
		 WHERE pool_id != '' AND is_active = 1
		 ORDER BY pool_id, priority ASC`)
	if err != nil {
		return
	}
	defer rows.Close()

	pools := make(map[string][]lb.Account)
	for rows.Next() {
		var id, name, apiKey, poolID string
		var priority int
		var isActive int
		if err := rows.Scan(&id, &name, &apiKey, &poolID, &priority, &isActive); err != nil {
			continue
		}
		pools[poolID] = append(pools[poolID], lb.Account{
			ID:       id,
			APIKey:   apiKey,
			Priority: priority,
			Active:   isActive == 1,
		})
	}

	for poolID, accounts := range pools {
		p.multiAccountLBs[poolID] = lb.NewMultiAccountLB(poolID, accounts)
	}
}

// RefreshMultiAccountPools re-reads multi-account pools from the database.
// Call after connection CRUD operations that change pool membership.
func (p *ProxyHandler) RefreshMultiAccountPools() {
	p.initMultiAccountPools()
}

// pickMultiAccountAPIKey selects an account from the pool and returns its API key.
// Returns the original API key if pool is not found or all accounts are rate-limited.
func (p *ProxyHandler) pickMultiAccountAPIKey(poolID, fallbackKey string) (string, string) {
	if poolID == "" {
		return fallbackKey, ""
	}
	p.mabMu.RLock()
	mab := p.multiAccountLBs[poolID]
	p.mabMu.RUnlock()
	if mab == nil {
		return fallbackKey, ""
	}
	acct, err := mab.Pick()
	if err != nil {
		return fallbackKey, ""
	}
	return acct.APIKey, acct.ID
}

// recordMultiAccountResult records success or failure for a pool account.
func (p *ProxyHandler) recordMultiAccountResult(poolID, accountID string, success bool, isRateLimit bool) {
	if poolID == "" || accountID == "" {
		return
	}
	p.mabMu.RLock()
	mab := p.multiAccountLBs[poolID]
	p.mabMu.RUnlock()
	if mab == nil {
		return
	}
	if success {
		mab.RecordSuccess(accountID)
	} else {
		mab.RecordFailure(accountID)
		if isRateLimit {
			mab.MarkRateLimited(accountID)
		}
	}
}