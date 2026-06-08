package oauthide

// StartBrowserPKCE prepares authorize URL for a PKCE browser provider.
func StartBrowserPKCE(nbytes int, build func(redirectURI, state, challenge string) string, publicBase, provider, sessionID string, setPKCE func(verifier string) error) (string, error) {
	pkce, err := NewPKCE(nbytes)
	if err != nil {
		return "", err
	}
	if err := setPKCE(pkce.Verifier); err != nil {
		return "", err
	}
	redirect := publicBase + "/api/oauth/callback/" + provider
	return build(redirect, sessionID, pkce.Challenge), nil
}