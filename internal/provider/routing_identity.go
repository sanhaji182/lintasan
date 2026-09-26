package provider

import (
	"net/url"
	"strings"
)

// RoutingPoolIdentity returns the stable identity used by routing combos to
// target a logical provider/account pool. An explicit pool_id is authoritative;
// otherwise protocol + normalized upstream host/path keeps separate APIs on the
// same host isolated (for example CommandCode Alpha versus its OpenAI API).
func RoutingPoolIdentity(format, baseURL, chatPath, poolID string) string {
	if pool := strings.TrimSpace(poolID); pool != "" {
		return "pool:" + pool
	}

	protocol := strings.ToLower(strings.TrimSpace(format))
	effective := JoinUpstreamPath(baseURL, strings.TrimSpace(chatPath))
	endpoint := "unknown"
	if parsed, err := url.Parse(effective); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		scheme := strings.ToLower(parsed.Scheme)
		host := strings.ToLower(strings.TrimSuffix(parsed.Host, "."))
		path := "/" + strings.Trim(strings.ToLower(parsed.EscapedPath()), "/")
		if path == "/" {
			path = ""
		}
		endpoint = scheme + "://" + host + path
	}
	return "provider:" + protocol + ":" + endpoint
}
