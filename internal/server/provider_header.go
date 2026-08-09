package server

import (
	"net/http"
	"strings"
)

// Response headers that tell the caller WHICH upstream actually served the
// request. Routing here is dynamic — load balancing, task-class reordering,
// hedging, and failover can all send two identical requests to two different
// providers — so without this the caller cannot tell who answered, and an
// operator debugging a bad reply has to correlate against server logs by
// timestamp.
const (
	// ProviderHeader carries the connection's human label ("Cerebras").
	ProviderHeader = "X-Lintasan-Provider"
	// ProviderIDHeader carries the stable connection id. The label is
	// editable and can be duplicated; the id is what you filter logs by.
	ProviderIDHeader = "X-Lintasan-Provider-Id"
)

// sanitizeHeaderValue makes an arbitrary string safe to use as an HTTP header
// value. Connection names are operator-typed free text, so they can contain
// CR/LF; splicing those into a header unescaped is response-splitting. Go's
// net/http would reject the write and silently drop the header, which fails
// closed but also means the header quietly disappears for exactly the
// connections whose names are malformed. Stripping instead keeps the header
// present and honest.
//
// Control characters are removed rather than replaced so a name that is pure
// whitespace collapses to "" and the caller can skip the header entirely.
func sanitizeHeaderValue(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		// Drop C0 controls (incl. CR, LF, NUL, tab) and DEL.
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	// Bound the value. A pathological name shouldn't be able to inflate every
	// response's header block.
	const maxLen = 128
	if len(out) > maxLen {
		out = strings.TrimSpace(out[:maxLen])
	}
	return out
}

// setProviderHeaders records the serving upstream on the response. It must be
// called BEFORE WriteHeader — for streaming responses the header block is
// flushed with the first byte, so a later call is a no-op.
func setProviderHeaders(h http.Header, connID, connName string) {
	if name := sanitizeHeaderValue(connName); name != "" {
		h.Set(ProviderHeader, name)
	}
	if id := sanitizeHeaderValue(connID); id != "" {
		h.Set(ProviderIDHeader, id)
	}
}
