package server

import "github.com/sanhaji182/lintasan-go/internal/cache"

// cache_key.go — keeps the response-cache key honest.
//
// Bug this file fixes: the cache key was built from temperature/max_tokens/
// top_p only, and the params map handed to the cache was rebuilt from those
// same three fields at every call site. A request carrying `tools` therefore
// hashed identically to the identical request without `tools`, and the cached
// tool-less body was replayed. The model then reported that it had no such
// tool — which reads exactly like an upstream stripping the tools field, but
// was really our own cache answering a question it was never asked.
//
// The allow-list itself lives in internal/cache (responseAffectingParams) so
// the hash and the save path cannot drift apart again.

// cacheParamsFromRequest extracts every response-affecting param from the
// decoded request. Call sites must use this instead of hand-building a params
// map, or the fields they forget silently stop participating in the key.
func cacheParamsFromRequest(req map[string]any) map[string]any {
	params := make(map[string]any, len(req))
	for k, v := range req {
		params[k] = v
	}
	return params
}

// cacheRequestNeedsStrictKey reports whether this request may only ever use an
// exact-match cache entry, never a semantic/fuzzy one.
func cacheRequestNeedsStrictKey(req map[string]any) bool {
	return cache.RequestNeedsStrictKey(req)
}
