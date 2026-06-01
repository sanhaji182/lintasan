// Package errfmt defines the standard error envelope used by Lintasan's API
// surface (test endpoints, dashboard API, and the /v1/* proxy).
//
// The envelope is OpenAI-compatible: every error response carries an `error`
// object with the fields {message, type, code, param}. This matches what
// OpenAI's own API returns, what most OpenAI-compatible clients already
// parse, and what Anthropic/Google also adopt (with minor field renames that
// we normalize on the way in).
//
// Lintasan is an OpenAI-compatible proxy — its API is officially OpenAI-shape.
// We never want error responses that look like {"error": "some string"} or
// plain text 500s. Always go through this package so the shape stays uniform.
package errfmt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Error is the OpenAI-compatible error object.
type Error struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
	Param   string `json:"param,omitempty"`
}

// Error type constants — align with OpenAI's documented taxonomy.
// (https://platform.openai.com/docs/guides/error-codes/api-errors)
const (
	TypeInvalidRequestError = "invalid_request_error" // 400, 404
	TypeAuthenticationError = "invalid_request_error" // OpenAI: 401
	TypePermissionError     = "permission_denied_error"
	TypeNotFoundError       = "invalid_request_error" // OpenAI: 404 model
	TypeRateLimitError      = "rate_limit_error"      // 429
	TypeServerError         = "server_error"          // 500
	TypeUpstreamError       = "upstream_error"        // 502/503/504 from upstream
	TypeNetworkError        = "network_error"         // dial/timeout
	TypeConnectionError     = "connection_error"      // refused, DNS
)

// Code constants — actionable identifiers the dashboard can switch on.
const (
	CodeInvalidAPIKey    = "invalid_api_key"
	CodeModelNotFound    = "model_not_found"
	CodeEndpointNotFound = "endpoint_not_found"
	CodeRateLimited      = "rate_limit_exceeded"
	CodeQuotaExceeded    = "insufficient_quota"
	CodeUpstream         = "upstream_error"
	CodeTimeout          = "timeout"
	CodeNetworkUnreach   = "network_unreachable"
	CodeConnRefused      = "connection_refused"
	CodeBadFormat        = "invalid_format"
	CodeUnknown          = "unknown_error"
)

// New builds a fresh Error.
func New(message, errType, code string) *Error {
	if errType == "" {
		errType = TypeServerError
	}
	if code == "" {
		code = CodeUnknown
	}
	return &Error{Message: message, Type: errType, Code: code}
}

// FromStatus maps an upstream HTTP status + raw body into a Lintasan-standard
// Error. It inspects the body and detects OpenAI / Anthropic / Google / generic
// upstream error shapes, then normalizes into our shape.
//
// `baseMsg` is the fallback message used when the body is empty or
// unparseable (e.g. "upstream status 401").
func FromStatus(status int, body []byte, baseMsg string) *Error {
	msg, code, param := detectUpstream(body)
	if msg == "" {
		msg = baseMsg
	}
	if msg == "" {
		msg = fmt.Sprintf("upstream status %d", status)
	}
	return &Error{
		Message: msg,
		Type:    typeForStatus(status),
		Code:    codeFor(status, code, msg),
		Param:   param,
	}
}

// FromNetworkError maps a transport-level error (dial/timeout) into a
// Lintasan-standard Error.
func FromNetworkError(err error) *Error {
	if err == nil {
		return New("unknown network error", TypeNetworkError, CodeUnknown)
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return New("request timed out: "+truncate(msg, 120), TypeNetworkError, CodeTimeout)
	case strings.Contains(msg, "connection refused"):
		return New("connection refused: "+truncate(msg, 120), TypeConnectionError, CodeConnRefused)
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "dns"):
		return New("DNS lookup failed: "+truncate(msg, 120), TypeNetworkError, CodeNetworkUnreach)
	default:
		return New("network error: "+truncate(msg, 120), TypeNetworkError, CodeNetworkUnreach)
	}
}

// Write serializes a standard JSON response with the OpenAI-compatible error
// envelope. The shape is:
//
//	{
//	  "error":   {message, type, code, param} | null,
//	  "data":    <any> | null,
//	  "message": <any> | null,   // for non-error status messages
//	  <extra fields>             // merged at top level
//	}
//
// The HTTP status is set from httpStatus.
func Write(w http.ResponseWriter, httpStatus int, err *Error, data any, extra map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	out := map[string]any{}
	if err != nil {
		out["error"] = err
	} else {
		out["error"] = nil
	}
	if data != nil {
		out["data"] = data
	} else {
		out["data"] = nil
	}
	for k, v := range extra {
		out[k] = v
	}
	_ = json.NewEncoder(w).Encode(out)
}

// ---------------------------------------------------------------------------
// upstream detection
// ---------------------------------------------------------------------------

// detectUpstream attempts to extract {message, code, param} from a JSON error
// body. Recognized upstream shapes (in order of detection):
//
//  1. OpenAI / OpenAI-compatible:
//     {"error": {"message": "...", "type": "...", "code": "...", "param": "..."}}
//
//  2. Anthropic:
//     {"type": "error", "error": {"type": "...", "message": "..."}}
//
//  3. Google / Gemini / Vertex:
//     {"error": {"code": 401, "message": "...", "status": "UNAUTHENTICATED"}}
//
//  4. Generic JSON:
//     {"error": "string"} | {"message": "string"} | {"error_message": "string"}
//
// Returns ("", "", "") when the body is empty or no known shape matches.
func detectUpstream(body []byte) (message, code, param string) {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "", "", ""
	}

	// Try to parse as generic map first; then try each known shape.
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		// Not JSON — return body as message, truncated.
		return truncate(s, 300), "", ""
	}

	// 1. OpenAI / OpenAI-compatible: {"error": {"message":..., "type":..., "code":...}}
	//    Also captures Anthropic responses because they share {"error": {"message":...,
	//    "type":...}} — but Anthropic uses "type" as its machine identifier (no
	//    "code" field). We treat that as the code when present.
	if e, ok := m["error"].(map[string]any); ok {
		if msg, _ := e["message"].(string); msg != "" {
			message = msg
		}
		if cd, _ := e["code"].(string); cd != "" {
			code = cd
		} else if cd, ok := e["code"].(float64); ok {
			code = fmt.Sprintf("%v", cd)
		} else if cd, _ := e["type"].(string); cd != "" {
			// No "code" field — "type" carries the identifier (Anthropic pattern).
			code = cd
		}
		if p, _ := e["param"].(string); p != "" {
			param = p
		}
		if message != "" {
			return message, code, param
		}
	}

	// 2. Anthropic: {"type":"error","error":{"type":"authentication_error","message":"..."}}
	if t, _ := m["type"].(string); t == "error" {
		if e, ok := m["error"].(map[string]any); ok {
			if msg, _ := e["message"].(string); msg != "" {
				message = msg
			}
			if cd, _ := e["type"].(string); cd != "" {
				code = cd
			}
			if message != "" {
				return message, code, param
			}
		}
	}

	// 3. Google: {"error":{"code":401,"message":"...","status":"UNAUTHENTICATED"}}
	if e, ok := m["error"].(map[string]any); ok {
		if msg, _ := e["message"].(string); msg != "" {
			message = msg
		}
		if cd, ok := e["code"].(float64); ok {
			code = fmt.Sprintf("%v", int(cd))
		}
		if st, _ := e["status"].(string); st != "" && code == "" {
			code = strings.ToLower(st)
		}
		if message != "" {
			return message, code, param
		}
	}

	// 4. Generic: {"error": "string"} or {"message": "string"} or {"error_message": "..."}
	if v, ok := m["error"].(string); ok && v != "" {
		return v, "", ""
	}
	if v, ok := m["error_message"].(string); ok && v != "" {
		return v, "", ""
	}
	if v, ok := m["message"].(string); ok && v != "" {
		return v, "", ""
	}
	if v, ok := m["detail"].(string); ok && v != "" {
		return v, "", ""
	}

	// 5. Last resort: return the raw body, truncated.
	return truncate(s, 300), "", ""
}

// typeForStatus picks the OpenAI-style error type for a given HTTP status.
func typeForStatus(status int) string {
	switch {
	case status == 400:
		return TypeInvalidRequestError
	case status == 401:
		return TypeAuthenticationError
	case status == 403:
		return TypePermissionError
	case status == 404, status == 405, status == 406, status == 415, status == 422:
		return TypeNotFoundError
	case status == 408, status == 409, status == 429:
		return TypeRateLimitError
	case status >= 500 && status <= 599:
		return TypeUpstreamError
	default:
		return TypeServerError
	}
}

// codeFor chooses a stable machine-readable code. It tries the upstream's own
// code (when present and well-known), else falls back to a status-derived
// default.
func codeFor(status int, upstreamCode, msg string) string {
	// If the upstream gave us a code we already understand, keep it.
	known := map[string]bool{
		"invalid_api_key": true, "invalid_request_error": true,
		"authentication_error": true, "permission_error": true,
		"not_found_error": true, "rate_limit_error": true,
		"insufficient_quota": true, "model_not_found": true,
		"organization_restricted": true, "tokens_exceeded": true,
	}
	if upstreamCode != "" {
		l := strings.ToLower(upstreamCode)
		if known[l] {
			return l
		}
	}

	// No upstream code (or unrecognized). Pick by status, with a small heuristic
	// for 401/403 to surface an "invalid_api_key" hint when the body text
	// mentions auth/key.
	switch status {
	case 400:
		return CodeBadFormat
	case 401:
		ml := strings.ToLower(msg)
		if strings.Contains(ml, "api key") || strings.Contains(ml, "auth") || strings.Contains(ml, "token") {
			return CodeInvalidAPIKey
		}
		return CodeInvalidAPIKey
	case 403:
		return "permission_denied"
	case 404:
		if modelHintRe.MatchString(strings.ToLower(msg)) {
			return CodeModelNotFound
		}
		return CodeEndpointNotFound
	case 405:
		return "method_not_allowed"
	case 406:
		return "not_acceptable"
	case 408:
		return "request_timeout"
	case 409:
		return "conflict"
	case 415:
		return "unsupported_media_type"
	case 422:
		return "unprocessable_entity"
	case 429:
		return CodeRateLimited
	case 502, 503, 504:
		return CodeUpstream
	default:
		return CodeUnknown
	}
}

// apiKeyShape matches keys that look like OpenAI/Anthropic/etc. — used to
// give a slightly more helpful message when the body is empty.
var apiKeyShape = regexp.MustCompile(`(?i)(api[ _-]?key|auth|token|credential|unauthor|forbidden)`)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// HintForMessage returns a short human hint for common upstream messages.
func HintForMessage(msg string) string {
	ml := strings.ToLower(msg)
	switch {
	case apiKeyShape.MatchString(ml):
		return "check the API key in your provider's dashboard"
	case modelHintRe.MatchString(ml):
		// "model" as a word (not a path segment like /v1/models or model_id)
		return "verify the model id is supported by this provider"
	case strings.Contains(ml, "quota") || strings.Contains(ml, "billing") || strings.Contains(ml, "credit"):
		return "check your account balance / billing settings"
	case strings.Contains(ml, "rate limit") || strings.Contains(ml, "too many requests") || strings.Contains(ml, "throttl"):
		return "slow down — provider is throttling"
	}
	return ""
}

// modelHintRe matches "model" only when it appears as a word (not a path).
// e.g. matches "model not found", "unsupported model"; does NOT match "/v1/models".
var modelHintRe = regexp.MustCompile(`\bmodel\b`)
