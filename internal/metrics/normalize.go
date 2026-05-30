package metrics

import "strings"

// NormalizeEndpoint maps a raw request path to a BOUNDED endpoint group label.
//
// Prometheus cardinality safety is a hard review gate: a latency histogram
// labeled by raw path (/api/connections/{id}, /v1/memory/{key}) would create
// unbounded series and blow up the TSDB. This function collapses every path to
// one of a small, fixed set of group strings. Any path we don't explicitly
// recognize collapses to "other" — we NEVER pass an arbitrary string through.
//
// The returned value is the ONLY thing that becomes the `endpoint` label, so by
// construction the label set is bounded regardless of what a client sends.
func NormalizeEndpoint(path string) string {
	switch {
	// --- OpenAI-compatible proxy (/v1/*) ---
	case path == "/v1/chat/completions":
		return "/v1/chat/completions"
	case path == "/v1/embeddings":
		return "/v1/embeddings"
	case path == "/v1/models":
		return "/v1/models"
	case path == "/v1/images/generations":
		return "/v1/images"
	case path == "/v1/audio/speech", path == "/v1/audio/transcriptions":
		return "/v1/audio"
	case path == "/v1/web/search":
		return "/v1/web"
	case strings.HasPrefix(path, "/v1/memory"):
		// /v1/memory, /v1/memory/search, /v1/memory/stats, /v1/memory/{key}
		// all collapse to one group — {key} is unbounded, must not leak.
		return "/v1/memory"

	// --- Dashboard API (/api/*) ---
	case strings.HasPrefix(path, "/api/auth"):
		return "/api/auth"
	case strings.HasPrefix(path, "/api/connections"):
		// /api/connections/{id} -> group, never the id.
		return "/api/connections"
	case strings.HasPrefix(path, "/api/combos"):
		return "/api/combos"
	case strings.HasPrefix(path, "/api/setup"):
		return "/api/setup"
	case strings.HasPrefix(path, "/api/translate"):
		return "/api/translate"
	case strings.HasPrefix(path, "/api/savings"):
		return "/api/savings"
	case strings.HasPrefix(path, "/api/plugins"):
		return "/api/plugins"
	case strings.HasPrefix(path, "/api/providers"), strings.HasPrefix(path, "/api/discover"):
		return "/api/discover"
	case strings.HasPrefix(path, "/api/mcp"):
		return "/api/mcp"
	case strings.HasPrefix(path, "/api/"):
		// Catch-all for the remaining ~40 stable dashboard endpoints
		// (/api/stats, /api/logs, /api/settings, ...). Bounded because every
		// such path is a fixed route with no dynamic id segment we surface.
		return "/api"

	// --- MCP protocol ---
	case strings.HasPrefix(path, "/mcp"):
		return "/mcp"

	// --- Infra ---
	case path == "/metrics":
		return "/metrics"
	case path == "/health":
		return "/health"
	}
	// Anything else (UI assets proxied through, unknown paths) collapses to a
	// single bucket so the label stays bounded.
	return "other"
}

// StatusClass maps an HTTP status code to a bounded class label ("2xx", "4xx",
// etc.) so the histogram never carries one series per exact code.
func StatusClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500:
		return "5xx"
	default:
		return "1xx"
	}
}
