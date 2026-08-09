package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// resetRequest is the POST body for /api/system/reset.
//
// Confirm is deliberately a phrase the caller must type, not a boolean: a
// stray `{"mode":"factory"}` from a curl retry or a mis-wired UI button must
// not be able to wipe an install. The UI asks the operator to type it.
type resetRequest struct {
	Mode    string `json:"mode"`
	Confirm string `json:"confirm"`
}

// confirmPhraseFor returns the exact phrase the caller must send for a mode.
// Different phrases per mode so a soft-reset confirmation cannot be replayed
// into a factory reset.
func confirmPhraseFor(mode ResetMode) string {
	switch mode {
	case ResetModeFactory:
		return "FACTORY RESET"
	default:
		return "RESET"
	}
}

// handleSystemReset returns the install to a baseline. Admin-only, explicitly
// confirmed, backed up first, and transactional.
//
// The response carries the backup path and, for a factory reset, the newly
// seeded admin credentials — shown exactly once, mirroring first-install
// behavior, because after this call the caller's own session is dead.
func (s *Server) handleSystemReset(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "database unavailable"})
		return
	}

	var req resetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}

	var mode ResetMode
	switch strings.ToLower(strings.TrimSpace(req.Mode)) {
	case "soft":
		mode = ResetModeSoft
	case "factory":
		mode = ResetModeFactory
	default:
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error": `mode must be "soft" or "factory"`,
		})
		return
	}

	// Typed confirmation, compared exactly. No normalization beyond trimming
	// surrounding whitespace: the operator has to mean it.
	want := confirmPhraseFor(mode)
	if strings.TrimSpace(req.Confirm) != want {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":            "confirmation phrase does not match",
			"required_confirm": want,
		})
		return
	}

	conn := s.db.Conn()

	// Count BEFORE deleting: the audit table is itself cleared, so this is the
	// only record of how much was destroyed.
	counts := countRows(conn, append(tablesFor(mode), "settings"))

	// Back up first. A reset that cannot be backed up does not run — this is
	// the operator's only undo.
	backupDir := "."
	if s.cfg != nil && s.cfg.DBPath != "" {
		backupDir = filepath.Dir(s.cfg.DBPath)
	}
	backupPath, err := backupBeforeReset(conn, backupDir)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{
			"error": "reset aborted: " + err.Error(),
		})
		return
	}

	if err := performReset(conn, mode); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{
			"error":       "reset failed and was rolled back: " + err.Error(),
			"backup_path": backupPath,
		})
		return
	}

	report := ResetReport{
		Mode:          mode,
		BackupPath:    backupPath,
		RowsDeleted:   counts,
		SettingsReset: settingsFor(mode),
		CompletedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	// Re-seed built-in presets so the install matches a fresh one rather than
	// an empty shell. Seeding is idempotent and additive.
	report.PresetsSeeded = s.reseedBuiltins()

	if mode == ResetModeFactory {
		// The one-way "setup complete" latch must be released, or the server
		// would keep believing it is configured while having no users at all.
		atomic.StoreInt32(&s.setup.active, 0)

		// Recreate the bootstrap admin exactly as a first install does:
		// random password, flagged must-change. Without this the dashboard
		// would be unreachable — a factory reset should return you to the
		// first-run screen, not lock you out permanently.
		if s.userMgr != nil {
			username := "admin"
			if pw, err := s.userMgr.SeedAdmin(username); err == nil && pw != "" {
				report.AdminUsername = username
				report.AdminPassword = pw
			}
		}
	}

	// Audit AFTER the wipe: the pre-reset audit rows are gone, so this row is
	// the first entry of the install's new life and records who did it.
	s.audit("system.reset", admin.Username, string(mode), map[string]any{
		"backup_path":  backupPath,
		"rows_deleted": counts,
	})

	writeJSONStatus(w, http.StatusOK, report)
}

// reseedBuiltins restores the built-in provider presets and categories that a
// fresh install ships with. Returns the number of presets present afterwards.
func (s *Server) reseedBuiltins() int {
	conn := s.db.Conn()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	for _, p := range seedCatalogue() {
		var exists int
		if err := conn.QueryRow(
			"SELECT COUNT(*) FROM provider_presets WHERE lower(name) = lower(?)", p.Name,
		).Scan(&exists); err == nil && exists > 0 {
			continue
		}
		id, _ := generatePresetID()
		conn.Exec(`
			INSERT INTO provider_presets (id, name, domain, base_url, format, key_label, category, is_builtin, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			id, p.Name, p.Domain, p.BaseURL, p.Format, p.KeyLabel, p.Category, now, now)
	}

	var total int
	conn.QueryRow("SELECT COUNT(*) FROM provider_presets").Scan(&total)
	return total
}
