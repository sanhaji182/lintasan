package server

import "strings"

const (
	// claudeMessagesAPISettingKey gates POST /v1/messages (Anthropic Messages
	// ingress). Default false: the surface answers 404 until an operator opts in.
	claudeMessagesAPISettingKey = "claude_messages_api_enabled"
	// geminiNativeAPISettingKey gates the native Gemini generateContent and
	// streamGenerateContent ingresses. Default false.
	geminiNativeAPISettingKey = "gemini_native_api_enabled"
)

// nativeIngressEnabled reads a dashboard/DB setting on EVERY request. That is
// deliberate: an operator can disable an accidentally exposed surface without a
// restart, and there is no second env/startup latch that can diverge from the UI.
func (p *ProxyHandler) nativeIngressEnabled(key string) bool {
	if p == nil || p.db == nil {
		return false
	}
	v, err := p.db.GetSetting(key)
	if err != nil || strings.TrimSpace(v) == "" {
		return false
	}
	b, ok := parseBoolSetting(v)
	return ok && b
}

func (p *ProxyHandler) claudeMessagesAPIEnabled() bool {
	return p.nativeIngressEnabled(claudeMessagesAPISettingKey)
}

func (p *ProxyHandler) geminiNativeAPIEnabled() bool {
	return p.nativeIngressEnabled(geminiNativeAPISettingKey)
}
