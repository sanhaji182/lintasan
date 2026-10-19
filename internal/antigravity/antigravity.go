package antigravity

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Model id bookkeeping mirrors the 9router v0.5.104 registry table.
// Upstream rows = unique wire ids; parenthesised effort is NOT part of the
// wire id (Cloud Code 404s suffixed ids — luqman's comment pins this too).

// UpstreamModelIDs maps Lintasan catalog ids (and their bare forms) to
// the exact cloud code backend id. Bare catalog ids route to medium tier
// (matching 9router's chatCore applyThinking behaviour) but the (high)
// caller needed to get high STILL carries its suffix. Ports upstream rows
// a07ed95b (base table) and refresh #24034f69.
var UpstreamModelIDs = map[string]string{
	// Gemini 4 Argon — one backend id per tier; bare catalog id = medium.
	"gemini-4-argon":        "gemini-4-argon-medium",
	"gemini-4-argon-high":   "gemini-4-argon-high",
	"gemini-4-argon-medium": "gemini-4-argon-medium",
	"gemini-4-argon-low":    "gemini-4-argon-low",
	// Gemini 3.8 — one backend id per tier.
	"gemini-3.8-flash":        "gemini-3.8-flash-medium",
	"gemini-3.8-flash-high":   "gemini-3.8-flash-high",
	"gemini-3.8-flash-medium": "gemini-3.8-flash-medium",
	"gemini-3.8-flash-low":    "gemini-3.8-flash-low",
	// 3.7 and 3.6 share the tiered backend id per family.
	"gemini-3.7-flash":        "gemini-3.7-flash-tiered",
	"gemini-3.7-flash-high":   "gemini-3.7-flash-tiered",
	"gemini-3.7-flash-medium": "gemini-3.7-flash-tiered",
	"gemini-3.7-flash-low":    "gemini-3.7-flash-tiered",
	"gemini-3.6-flash":        "gemini-3.6-flash-tiered",
	"gemini-3.6-flash-high":   "gemini-3.6-flash-tiered",
	"gemini-3.6-flash-medium": "gemini-3.6-flash-tiered",
	"gemini-3.6-flash-low":    "gemini-3.6-flash-tiered",
	// Pro is not tiered; the high id is served under the agent alias.
	"gemini-3.1-pro-high": "gemini-pro-agent",
	"gemini-3.1-pro-low":  "gemini-3.1-pro-low",
	// Claude 5.5 — tuples; bare catalog id is the (high) thinking tier.
	"claude-sonnet-5-5":        "claude-sonnet-5-5-high",
	"claude-opus-5-5":          "claude-opus-5-5-high",
	"claude-sonnet-5-5-high":   "claude-sonnet-5-5-high",
	"claude-sonnet-5-5-low":    "claude-sonnet-5-5-low",
	"claude-sonnet-5-5-medium": "claude-sonnet-5-5-medium",
	"claude-opus-5-5-high":     "claude-opus-5-5-high",
	"claude-opus-5-5-low":      "claude-opus-5-5-low",
	"claude-opus-5-5-medium":   "claude-opus-5-5-medium",
}

// ResolveUpstreamModel mirrors ANTIGRAVITY (the Lintasan catalog id) to the
// exact upstream backend id Google serves. Unknown ids fall through so a
// new model still routable. A parenthesised effort suffix ("...-high(high)")
// resolves to its BASE id — the suffix carries only thinking hint
// information; Lintasan's chat-core already sex it like upstream does.
func ResolveUpstreamModel(model string) string {
	base := StripParenthesisedSuffix(strings.ToLower(strings.TrimSpace(model)))
	if resolved, ok := UpstreamModelIDs[base]; ok {
		return StripParenthesisedSuffix(resolved)
	}
	return base
}

// StripParenthesisedSuffix removes a trailing "(level)" from an id, mirroring
// stripParenthesisedSuffix in the 9router-go port. Idempotent.
func StripParenthesisedSuffix(s string) string {
	if len(s) == 0 {
		return s
	}
	idx := strings.LastIndex(s, "(")
	if idx < 0 {
		return s
	}
	if strings.HasSuffix(s, ")") {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

// IsImageModel mirrors 9router's isImageModel: a model id whose name carries
// "image" or "imagen" gets a completely different request shape (no
// streamGenerateContent, image retry counter, etc.).
func IsImageModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(m, "image") || strings.Contains(m, "imagen")
}

// RequestEnvelope is the PROTO shape of a /v1internal:generateContent
// (and :streamGenerateContent) Antigravity call. 9router's fields, in
// wire order the caller ultimately serialises. The apiKey/projectID/
// model are the renderer identity; the userEmail is where Lintasan's
// oauthSessions.hasNextPage() knows who to bill.
type RequestEnvelope struct {
	Project   string          `json:"project"`   // cloud code project id
	Model     string          `json:"model"`     // upstream model id (NO (level) suffix)
	UserAgent string          `json:"userAgent"` // "antigravity" constant
	RequestID string          `json:"requestId"` // agent/<type>/<stage>/<counter>/<step>
	Request   json.RawMessage `json:"request"`   // the inner Gemini contents block
	UserEmail string          `json:"userEmail,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Extra     json.RawMessage `json:"extra,omitempty"`
}

// WrapGeminiRequest is the Lintasan caller's entry: it turns an OpenAI-shaped
// request body (already OpenAI→Gemini-translated upstream) into the
// Antigravity envelope shape /v1internal:generateContent expects.
//   - projectID may be "" when the caller has not resolved one (the OAuth IDE
//     path sets it from cloudcode project discovery).
//   - model IS the Lintasan catalog id; it resolves to the wire via
//     ResolveUpstreamModel so a bare id and a suffixed id both land.
//   - request blocks pass through untyped; Lintasan's
//     internal/server/handlers_native_ingress.go owns the OpenAI→Gemini
//     translation BEFORE this wrapper does anything.
func WrapGeminiRequest(geminiBody []byte, projectID, model, userEmail, sessionID string) ([]byte, error) {
	wireModel := ResolveUpstreamModel(model)
	env := RequestEnvelope{
		Project:   StripParenthesisedSuffix(projectID),
		Model:     wireModel,
		UserAgent: "antigravity",
		// Omit RequestType — see decolua/9router#3986: Google rejects
		// agent-bucketed requests with false 429s; the requestId keeps its
		// existing agent/<...>/<...>/<...>/<step> shape.
		RequestID: BuildRequestID(projectID, wireModel, "agent", 1),
		Request:   json.RawMessage(geminiBody),
		UserEmail: userEmail,
		SessionID: sessionID,
	}
	return json.Marshal(env)
}

// BuildRequestID mirrors the 9router executor's requestId shape
// (agent/<type>/<stage>/<counter>/<step>). Both are used for billing
// and cache keying; Lintasan keeps the same shape so a request in
// flight on the hot path can be audited.
func BuildRequestID(projectID, model, requestType string, counter int) string {
	// The 9router executor keeps the numeric segments opaque (anything
	// unique works) but the SHAPE is pinned: agent/<type>/<stage>/
	// <counter>/<step>. Use the stripped values here so the shape is
	// consistent across a retry chain.
	return fmt.Sprintf(
		"agent/%s/%d/%s/%d",
		requestType,
		counter,
		StripParenthesisedSuffix(model),
		1,
	)
}
