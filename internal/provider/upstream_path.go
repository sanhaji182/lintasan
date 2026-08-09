package provider

import "strings"

// JoinUpstreamPath joins a connection base URL with a canonical OpenAI-style
// endpoint path without producing a doubled version segment.
//
// The non-chat endpoints (embeddings, images, audio) hardcode canonical paths
// like "/v1/audio/speech". Connection base URLs, however, are stored in two
// shapes: bare host ("https://api.openai.com") and already-versioned
// ("https://api.openai.com/v1" — which is what every catalogue preset uses).
// Naive concatenation turns the second shape into "/v1/v1/audio/speech", which
// 404s on every provider. This mirrors the chat-path rule in the server's
// apiPathsFor: when the base already ends in the version segment, the path's
// leading version segment is dropped.
//
// A version segment appearing in the MIDDLE of the base ("https://x/v1/openai")
// is not a suffix and must not trigger the trim — the full path is appended.
func JoinUpstreamPath(baseURL, canonicalPath string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if canonicalPath == "" {
		return base
	}
	if !strings.HasPrefix(canonicalPath, "/") {
		canonicalPath = "/" + canonicalPath
	}

	baseVer, ok := versionSuffix(base)
	if !ok {
		return base + canonicalPath
	}
	// Only trim when the path's leading segment is the SAME version the base
	// already carries. A "/v1" base with a "/v2/..." path must keep both, or
	// the request would silently target the wrong API version.
	prefix := "/" + baseVer + "/"
	if strings.HasPrefix(strings.ToLower(canonicalPath), prefix) {
		return base + canonicalPath[len(baseVer)+1:]
	}
	return base + canonicalPath
}

// versionSuffix reports the trailing version segment of a base URL, lowercased
// ("v1", "v2", "v1beta"), and whether one is present. A segment counts as a
// version when it starts with "v" followed by a digit.
func versionSuffix(base string) (string, bool) {
	i := strings.LastIndex(base, "/")
	if i < 0 {
		return "", false
	}
	seg := strings.ToLower(base[i+1:])
	if len(seg) < 2 || seg[0] != 'v' || seg[1] < '0' || seg[1] > '9' {
		return "", false
	}
	return seg, true
}
