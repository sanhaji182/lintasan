package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/oauthide"
)

func (s *Server) registerOAuthCursorRoutes() {
	s.mux.HandleFunc("GET /api/oauth/cursor/import", s.handleOAuthCursorImportInfo)
	s.mux.HandleFunc("POST /api/oauth/cursor/import", s.handleOAuthCursorImport)
}

func (s *Server) handleOAuthCursorImportInfo(w http.ResponseWriter, r *http.Request) {
	if !s.oauthIdeEnabled() {
		oauthIdeDisabledJSON(w)
		return
	}
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	out := oauthide.CursorImportInstructions()
	out["experimental"] = true
	out["disclaimer"] = auth.IdeOAuthDisclaimer
	writeJSON(w, out)
}

func (s *Server) handleOAuthCursorImport(w http.ResponseWriter, r *http.Request) {
	if !s.oauthIdeEnabled() {
		oauthIdeDisabledJSON(w)
		return
	}
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var input struct {
		AccessToken      string `json:"accessToken"`
		MachineID        string `json:"machineId"`
		AcknowledgeRisk  bool   `json:"acknowledge_risk"`
		AcknowledgeRisk2 bool   `json:"acknowledgeRisk"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !input.AcknowledgeRisk && !input.AcknowledgeRisk2 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error": "acknowledge_risk required", "disclaimer": auth.IdeOAuthDisclaimer,
		})
		return
	}
	payload, flowMeta, err := oauthide.ValidateCursorImport(input.AccessToken, input.MachineID)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	session, err := s.oauthMgr.CreateSession("cursor")
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	expires := time.Now().Add(30 * 24 * time.Hour)
	if err := s.oauthMgr.UpdateSessionTokensWithMeta(session.ID, payload.AccessToken, "", expires, flowMeta); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "store tokens failed"})
		return
	}
	s.audit("oauth.ide.cursor_import", admin.Username, session.ID, map[string]any{"provider": "cursor"})
	writeJSON(w, map[string]any{
		"status": "active", "experimental": true, "provider": "cursor", "session_id": session.ID,
	})
}