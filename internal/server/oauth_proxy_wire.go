package server

import (
	"io"
	"net/http"
	"strings"
	"time"

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

// markPoolAccountAuthFailed takes a rejected key out of the pool's rotation.
// Counts the failure too, so account health stats stay honest.
func (p *ProxyHandler) markPoolAccountAuthFailed(poolID, accountID string) {
	if poolID == "" || accountID == "" {
		return
	}
	p.mabMu.RLock()
	mab := p.multiAccountLBs[poolID]
	p.mabMu.RUnlock()
	if mab == nil {
		return
	}
	mab.RecordFailure(accountID)
	mab.MarkAuthFailed(accountID)
}

// poolAvailableAccounts reports how many accounts in the pool are still
// pickable. Zero means the pool is exhausted and the caller should fail over to
// a different connection instead of burning more requests here.
func (p *ProxyHandler) poolAvailableAccounts(poolID string) int {
	if poolID == "" {
		return 0
	}
	p.mabMu.RLock()
	mab := p.multiAccountLBs[poolID]
	p.mabMu.RUnlock()
	if mab == nil {
		return 0
	}
	return mab.AvailableCount()
}

// retryPoolAccounts is the auth-failure recovery loop for a multi-account
// connection. The first key already came back 401/403 and has been marked
// AuthFailed by the caller; this tries the remaining keys in the same pool,
// one upstream call each, until one succeeds or the pool is exhausted.
//
// Returns the successful *http.Response (body still open, caller owns closing
// it) and the winning account id, or (nil, "") when every remaining key was
// also rejected — the signal for the caller to fall over to another connection.
//
// Any key that also returns 401/403 is marked AuthFailed so the next Pick()
// skips it; a key that fails some other way (5xx, network) is left in rotation
// because that is not an auth problem. lastErr/lastStatus track the most recent
// failure so an exhausted pool reports a real upstream status, not a synthetic
// one. Bounded by the pickable-account count: no key is tried twice, so a pool
// of dead keys terminates instead of spinning.
func (p *ProxyHandler) retryPoolAccounts(
	r *http.Request,
	conn *Connection,
	body []byte,
	resolvedModel string,
	start time.Time,
	taskClass, modeLabel string,
	lastErr *string,
	lastStatus *int,
) (*http.Response, string) {
	if conn.PoolID == "" {
		return nil, ""
	}
	// One attempt per still-pickable account. AvailableCount already excludes
	// the key we just marked failed, so this is the exact number of untried
	// keys left.
	maxAttempts := p.poolAvailableAccounts(conn.PoolID)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		pickedKey, acctID := p.pickMultiAccountAPIKey(conn.PoolID, conn.APIKey)
		if acctID == "" {
			// Pool drained mid-loop (another request may have failed the last
			// key). Nothing left to try.
			return nil, ""
		}
		cpy := *conn
		cpy.APIKey = pickedKey

		resp, err := p.doUpstream(r, &cpy, body)
		if err != nil {
			*lastErr = err.Error()
			// Transport error is not an auth problem; count it but leave the
			// key in rotation. Do not mark AuthFailed or we would wrongly evict
			// a good key on a transient blip.
			p.recordMultiAccountResult(conn.PoolID, acctID, false, false)
			continue
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			*lastErr = string(b)
			*lastStatus = resp.StatusCode
			p.markPoolAccountAuthFailed(conn.PoolID, acctID)
			p.logRequest(resolvedModel, conn.ID, conn.Name, resp.StatusCode, time.Since(start).Milliseconds(), 0, 0, false, *lastErr, taskClass, modeLabel)
			continue
		}
		// Anything else (2xx, or even 429/5xx) is handed back to the main loop,
		// which already knows how to treat those on the returned response. The
		// win we care about — a working key — is covered, and non-auth statuses
		// keep their existing handling rather than being reinterpreted here.
		return resp, acctID
	}
	return nil, ""
}
