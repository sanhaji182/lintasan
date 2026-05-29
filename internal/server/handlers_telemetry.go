package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.proxy == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": "proxy unavailable"})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"data": s.proxy.telemetrySnapshot(),
	})
}
