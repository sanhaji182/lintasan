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
	host := ""
	basePath := ""
	if parsed, err := url.Parse(strings.TrimSpace(baseURL)); err == nil {
		host = strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
		basePath = strings.Trim(strings.ToLower(parsed.EscapedPath()), "/")
		if basePath == "v1" {
			basePath = ""
		}
	}
	path := "/" + strings.Trim(strings.ToLower(strings.TrimSpace(chatPath)), "/")
	if path == "/" {
		path = ""
	}
	if host == "" {
		host = "unknown"
	}
	if basePath != "" {
		host += "/" + basePath
	}
	return "provider:" + protocol + ":" + host + ":" + path
}
