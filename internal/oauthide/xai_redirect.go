package oauthide

// XAI loopback redirect — only URI registered for the public grok-cli OAuth client (9router / CLIProxyAPI parity).
const (
	XAILoopbackPort    = 56121
	XAILoopbackPath    = "/callback"
	XAILoopbackRedirect = "http://127.0.0.1:56121/callback"
)