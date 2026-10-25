package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// oauth_accounts_handler.go — GET /api/oauth/accounts
//
// WHY THIS EXISTS
//
// /api/oauth/sessions returns a flat list of session rows. That answers "which
// tokens exist" but not the two questions an operator actually has when a wired
// connection fails: "how many HEALTHY accounts does this provider have, and
// which account is gated?" A flat list hides the difference between one healthy
// account and one gated account plus two healthy ones — and it was exactly that
// blindness that let "connection looks wired but requests fail" go unexplained.
//
// This endpoint groups by provider and reports per-provider counts, the wire
// state of the matching connection, and each account's health. Access and
// refresh tokens are NEVER returned: only a masked hint plus last-4.

// oauthAccountView is one account as shown to the dashboard. No secrets.
type oauthAccountView struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	Status       string `json:"status"`
	Health       string `json:"health"` // healthy | expiring | restricted | expired | revoked
	CreatedAt    string `json:"created_at,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	MaskedToken  string `json:"masked_token,omitempty"`
	AccountLabel string `json:"account_label,omitempty"`
}

// oauthProviderView is the per-provider roll-up the dashboard groups on.
type oauthProviderView struct {
	Provider       string `json:"provider"`
	Name           string `json:"name,omitempty"`
	Active         int    `json:"active"`
	Expiring       int    `json:"expiring"`
	Restricted     int    `json:"restricted"`
	Expired        int    `json:"expired"`
	Revoked        int    `json:"revoked"`
	Total          int    `json:"total"`
	Wired          bool   `json:"wired"`
	ConnectionID   string `json:"connection_id,omitempty"`
	ConnectionName string `json:"connection_name,omitempty"`
}

// maskToken reveals only enough to tell accounts apart. A token short enough
// that any slice would leak most of it is fully masked.
func maskToken(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if len(t) <= 12 {
		return "••••"
	}
	return t[:4] + "…" + t[len(t)-4:]
}

func (s *Server) handleOAuthAccounts(w http.ResponseWriter, r *http.Request) {
	if !s.oauthIdeEnabled() {
		oauthIdeDisabledJSON(w)
		return
	}
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	rows, err := s.db.Conn().Query(
		`SELECT id, provider, access_token, status, created_at, COALESCE(expires_at,''), COALESCE(flow_meta,'')
		 FROM oauth_sessions ORDER BY provider ASC, datetime(created_at) DESC`)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	accounts := make([]oauthAccountView, 0, 16)
	byProvider := map[string]*oauthProviderView{}

	for rows.Next() {
		var id, provider, token, status, createdAt, expiresAt, flowMeta string
		if err := rows.Scan(&id, &provider, &token, &status, &createdAt, &expiresAt, &flowMeta); err != nil {
			continue
		}
		health := oauthAccountHealth(status, expiresAt, s)
		accounts = append(accounts, oauthAccountView{
			ID:           id,
			Provider:     provider,
			Status:       status,
			Health:       health,
			CreatedAt:    createdAt,
			ExpiresAt:    expiresAt,
			MaskedToken:  maskToken(token),
			AccountLabel: oauthAccountLabel(provider, flowMeta),
		})

		pv := byProvider[provider]
		if pv == nil {
			pv = &oauthProviderView{Provider: provider}
			if p := oauthProviderDisplayName(provider); p != "" {
				pv.Name = p
			}
			byProvider[provider] = pv
		}
		pv.Total++
		switch health {
		case "healthy":
			pv.Active++
		case "expiring":
			pv.Expiring++
		case "restricted":
			pv.Restricted++
		case "expired":
			pv.Expired++
		case "revoked":
			pv.Revoked++
		}
	}

	// Wire state per provider: does a connection point at this oauth_provider,
	// and is it active? This is what makes the dashboard's "wired" badge honest.
	connRows, err := s.db.Conn().Query(
		`SELECT id, name, oauth_provider FROM connections
		 WHERE oauth_provider != '' AND is_active = 1`)
	if err == nil {
		defer connRows.Close()
		for connRows.Next() {
			var cid, cname, prov string
			if connRows.Scan(&cid, &cname, &prov) != nil {
				continue
			}
			pv := byProvider[prov]
			if pv == nil {
				pv = &oauthProviderView{Provider: prov}
				if p := oauthProviderDisplayName(prov); p != "" {
					pv.Name = p
				}
				byProvider[prov] = pv
			}
			pv.Wired = true
			pv.ConnectionID = cid
			pv.ConnectionName = cname
		}
	}

	providers := make([]oauthProviderView, 0, len(byProvider))
	// Stable, human-friendly order: known catalog order first, then extras.
	for _, p := range oauthAccountsProviderOrder() {
		if pv, ok := byProvider[p]; ok {
			providers = append(providers, *pv)
			delete(byProvider, p)
		}
	}
	for _, pv := range byProvider {
		providers = append(providers, *pv)
	}

	writeJSON(w, map[string]any{
		"enabled":    true,
		"accounts":   accounts,
		"providers":  providers,
		"disclaimer": auth_IdeOAuthDisclaimer(),
	})
}

// oauthAccountHealth turns (status, expiry) into the single label the UI shows.
// Expiry is treated as "expiring" inside the refresh window so the dashboard
// warns before a request has to fail.
func oauthAccountHealth(status, expiresAt string, s *Server) string {
	switch status {
	case "restricted":
		return "restricted"
	case "revoked":
		return "revoked"
	case "expired":
		return "expired"
	case "active":
		if expiresAt != "" {
			if until, ok := parseExpiryUntil(expiresAt); ok {
				if until <= 0 {
					return "expired"
				}
				if until < int(oauthRefreshSkewSeconds) {
					return "expiring"
				}
			}
		}
		return "healthy"
	default:
		return status
	}
}

func oauthAccountsProviderOrder() []string {
	out := make([]string, 0, 8)
	for _, p := range oauthIdeProviderIDs() {
		out = append(out, p)
	}
	return out
}

// oauthAccountLabel extracts a human-readable account identity from the
// provider-specific flow_meta written during authorization. This is
// display-only: id_token payloads arrive directly from the provider over TLS,
// so the signature is not re-verified here.
func oauthAccountLabel(provider, flowMeta string) string {
	if strings.TrimSpace(flowMeta) == "" {
		return ""
	}
	var meta map[string]any
	if json.Unmarshal([]byte(flowMeta), &meta) != nil {
		return ""
	}

	paths := [][]string{
		{"email"},
		{"userEmail"},
		{"github_user", "email"},
		{"userInfo", "email"},
		{"user", "email"},
		{"login"},
		{"github_user", "login"},
		{"username"},
		{"preferred_username"},
		{"name"},
		{"github_user", "name"},
		{"userInfo", "name"},
		{"user", "name"},
		{"label"},
		{"account"},
	}
	for _, path := range paths {
		if value := oauthNestedString(meta, path...); value != "" {
			return value
		}
	}

	if idToken := oauthNestedString(meta, "id_token"); idToken != "" {
		if claims := oauthJWTClaims(idToken); claims != nil {
			for _, key := range []string{"email", "preferred_username", "name", "nickname"} {
				if value, ok := claims[key].(string); ok && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
	}

	if machineID := oauthNestedString(meta, "machineId"); machineID != "" {
		return oauthProviderLabel(provider) + " · " + oauthShortID(machineID)
	}
	return ""
}

func oauthProviderLabel(provider string) string {
	if strings.EqualFold(provider, "cursor") {
		return "Cursor"
	}
	if provider == "" {
		return "Account"
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
}

func oauthNestedString(meta map[string]any, path ...string) string {
	var current any = meta
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[key]
	}
	value, ok := current.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func oauthJWTClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return nil
	}
	return claims
}

func oauthShortID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return value
	}
	return value[:8] + "…"
}
