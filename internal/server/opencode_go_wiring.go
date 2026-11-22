package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// OpenCode Go requires x-opencode-session on every chat request; without it
// the endpoint answers 400 MissingSessionID (live verified 2026-11-22).
// Derivation mirrors 9router's opencode-go executor:
//
//	sha256("opencode-go\0" + clientTool + "\0" + sessionId), first 16 bytes
//	hex-encoded, prefixed with "ses_".
//
// A client-supplied native-shaped id is reused verbatim.
func OpenCodeGoSessionID(sessionID, clientTool string) string {
	if sessionID != "" && strings.HasPrefix(sessionID, "ses_") && len(sessionID) == 36 {
		return sessionID
	}
	if clientTool == "" {
		clientTool = "generic"
	}
	digest := sha256.Sum256([]byte("opencode-go\x00" + clientTool + "\x00" + sessionID))
	return "ses_" + hex.EncodeToString(digest[:16])
}

// applyOpenCodeGoHeaders attaches the headers the OpenCode Go inference lane
// checks. Downstream UAs that are not a real opencode client are replaced by
// the pinned constant, mirroring the sibling Zen lane.
func applyOpenCodeGoHeaders(h http.Header, downstreamUA, sessionID, clientTool string) {
	if h == nil {
		return
	}
	ua := downstreamUA
	if !strings.HasPrefix(ua, "opencode/") {
		ua = "opencode/1.18.31"
	}
	h.Set("User-Agent", ua)
	h.Set("x-opencode-session", OpenCodeGoSessionID(sessionID, clientTool))
	h.Set("x-opencode-client", clientToolOrGeneric(clientTool))
	h.Set("x-opencode-project", "global")
}

func clientToolOrGeneric(s string) string {
	if s == "" {
		return "generic"
	}
	return s
}

// goEnabledSetting reads the OpenCode Go kill-switch. Default OFF, fail-closed,
// same parsing contract as the sibling Zen flag.
func goEnabledSetting(p *ProxyHandler) bool {
	if p == nil || p.db == nil {
		return false
	}
	v, err := p.db.GetSetting("opencode_go_enabled")
	if err != nil {
		return false
	}
	return parseOpencodeZenFlag(v)
}
