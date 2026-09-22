package server

import "net/http"

// handleChatCompletions serves POST /v1/chat/completions (and its /api/v1 alias).
//
// This used to live in the Hoplite Cloud Agent file and grew a large branch that
// decoded the body to look for cloud-agent models, rewrote thread/agent headers, and
// handed off to a sandbox-backed path. Hoplite was retired on 2026-09-22, so all of
// that is gone and this is a straight pass-through to the LLM gateway.
//
// It stays a separate handler rather than registering p.HandleChatCompletions directly
// because the route is intentionally named and worth a comment: a future reader who
// finds only `s.proxy.HandleChatCompletions` on the mux may wonder whether the
// cloud-agent interception was dropped by accident. It was dropped on purpose.
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	s.proxy.HandleChatCompletions(w, r)
}
