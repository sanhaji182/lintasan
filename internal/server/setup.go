package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
)

// Bootstrap / Active state machine.
//
// The server has exactly two states:
//
//	BOOTSTRAP — first-run. Setup is INCOMPLETE because EITHER there is no admin
//	            user OR no master key is configured. Only setup-related endpoints
//	            (/health, /, /api/auth/login, /api/setup/*) are reachable; every
//	            management and proxy endpoint returns 503 setup_required.
//
//	ACTIVE    — setup is COMPLETE: an admin user exists AND a master key exists.
//	            Auth is fully fail-CLOSED. Every management/proxy endpoint requires
//	            a valid JWT, master key, or dashboard API key.
//
// There is no third state. The transition is a ONE-WAY LATCH: once the server
// observes (hasAdmin && hasMasterKey) it pins to ACTIVE for the remainder of the
// process lifetime. Deleting the admin or clearing the master key afterwards does
// NOT reopen the bootstrap surface — that closes the "delete credentials to
// re-open setup" attack. A genuine reset requires an operator-driven restart on
// an empty database.
//
// By construction it is impossible to reach a fail-open state from ACTIVE without
// a regression test failing (see security_boundary_test.go).

// active is the latched state flag: 0 = not-yet-active, 1 = ACTIVE (pinned).
type setupState struct {
	active int32
}

// setupPaths are the only endpoints reachable while in BOOTSTRAP.
func isSetupPath(path, method string) bool {
	switch {
	case path == "/health":
		return true
	case path == "/":
		return true
	case path == "/api/auth/login" && method == http.MethodPost:
		return true
	case path == "/api/setup/status":
		return true
	case path == "/api/setup" && method == http.MethodPost:
		return true
	case path == "/api/auth/change-password" && method == http.MethodPost:
		// Allowed so the seeded admin can rotate the random bootstrap password
		// (after authenticating via login) before a master key is set.
		return true
	}
	return false
}

// isActive reports whether the server has completed setup. Once true it is
// pinned true (one-way latch). The expensive DB checks only run until the latch
// trips, after which it is a single atomic load.
func (s *Server) isActive() bool {
	if atomic.LoadInt32(&s.setup.active) == 1 {
		return true
	}
	hasAdmin := s.userMgr != nil && s.userMgr.AdminCount() > 0
	hasMasterKey := s.masterKeyConfigured()
	if hasAdmin && hasMasterKey {
		atomic.StoreInt32(&s.setup.active, 1)
		return true
	}
	return false
}

// masterKeyConfigured reports whether a non-empty master key exists, either in
// the DB settings or in the loaded config (env LINTASAN_MASTER_KEY).
func (s *Server) masterKeyConfigured() bool {
	if s.cfg != nil && strings.TrimSpace(s.cfg.MasterKey) != "" {
		return true
	}
	if v, _ := s.db.GetSetting("master_key"); strings.TrimSpace(v) != "" {
		return true
	}
	return false
}

// --- Setup HTTP endpoints ---

// handleSetupStatus reports the current bootstrap/active state. Public (safe):
// it leaks no secrets, only booleans the login UI needs to render first-run.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	hasAdmin := s.userMgr != nil && s.userMgr.AdminCount() > 0
	hasMasterKey := s.masterKeyConfigured()
	state := "bootstrap"
	if s.isActive() {
		state = "active"
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{
		"state":          state,
		"has_admin":      hasAdmin,
		"has_master_key": hasMasterKey,
		"setup_required": state == "bootstrap",
	})
}

// handleSetupComplete sets the master key during first-run. It is only reachable
// in BOOTSTRAP (the middleware blocks it once ACTIVE) and requires the caller to
// already be an authenticated admin — i.e. the seeded admin must log in first.
// This avoids an anonymous attacker setting the master key during the bootstrap
// window.
func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	if s.isActive() {
		writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "setup already complete"})
		return
	}
	// Require an authenticated admin even during bootstrap.
	user := s.requestUser(r)
	if user == nil || user.Role != "admin" {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "admin authentication required to complete setup"})
		return
	}
	var req struct {
		MasterKey string `json:"master_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if len(strings.TrimSpace(req.MasterKey)) < 16 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "master key must be at least 16 characters"})
		return
	}
	if err := s.db.SetSetting("master_key", strings.TrimSpace(req.MasterKey)); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "failed to persist master key"})
		return
	}
	s.audit("setup.master_key_set", user.Username, "settings/master_key", map[string]any{"by": user.ID})
	// Re-evaluate; this may latch ACTIVE.
	active := s.isActive()
	writeJSONStatus(w, http.StatusOK, map[string]any{"ok": true, "active": active})
}
