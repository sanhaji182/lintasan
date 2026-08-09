package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/sanhaji182/lintasan-go/internal/migrate"
)

// handlers_migrate.go — import a competitor router's export.
//
// Two endpoints, deliberately split:
//
//	POST /api/migrate/preview  → parse and report, write nothing
//	POST /api/migrate/import   → parse and apply
//
// The split exists because most of a real export cannot be migrated: in the
// sample that drove this feature, 973 connections yielded 16 usable ones (the
// rest were OAuth-based or already dead upstream). Importing silently would
// leave the user staring at a mostly-empty dashboard with no explanation, so the
// preview is a first-class step rather than a convenience.
//
// The uploaded file is never written to disk. It contains live API keys, and a
// temp file is one stray backup away from being a credential leak.

// maxImportBytes caps the upload. The reference export is 1.2 MB; 16 MB leaves
// room for far larger setups while keeping a hostile upload from exhausting RAM.
const maxImportBytes = 16 << 20

// registerMigrateRoutes wires the importer.
//
// No per-route auth wrapper: authMiddleware already gates every /api/* path
// fail-closed (see server.go). Adding a second check here would imply the
// global one is optional, which is exactly the assumption the security boundary
// tests exist to prevent.
func (s *Server) registerMigrateRoutes() {
	s.mux.HandleFunc("POST /api/migrate/preview", s.handleMigratePreview)
	s.mux.HandleFunc("POST /api/migrate/import", s.handleMigrateImport)
}

// --- preview ---------------------------------------------------------------

func (s *Server) handleMigratePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMigrateError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	plan, err := s.parseUpload(r)
	if err != nil {
		writeMigrateError(w, http.StatusBadRequest, err.Error())
		return
	}

	includeUnusable := r.URL.Query().Get("include_unusable") == "true"
	writeJSON(w, map[string]any{
		"success": true,
		"data": map[string]any{
			"source":   plan.Source,
			"summary":  plan.Summarize(),
			"selected": plan.Selected(includeUnusable),
			"healthy":  plan.Healthy,
			"unusable": plan.Unusable,
			"blocked":  plan.Blocked,
			"combos":   plan.Combos,
			"warnings": plan.Warnings,
		},
	})
}

// --- import ----------------------------------------------------------------

func (s *Server) handleMigrateImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMigrateError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	if s.db == nil {
		writeMigrateError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	plan, err := s.parseUpload(r)
	if err != nil {
		writeMigrateError(w, http.StatusBadRequest, err.Error())
		return
	}

	includeUnusable := r.URL.Query().Get("include_unusable") == "true"
	importCombos := r.URL.Query().Get("skip_combos") != "true"
	selected := plan.Selected(includeUnusable)

	if len(selected) == 0 {
		writeMigrateError(w, http.StatusUnprocessableEntity,
			"nothing to import: every connection in this export is either OAuth-based, "+
				"already failing, or uses a provider Lintasan has no endpoint for")
		return
	}

	res, err := s.applyImport(selected, plan.Combos, importCombos)
	if err != nil {
		writeMigrateError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"success": true, "data": res})
}

type importResult struct {
	ConnectionsImported int      `json:"connections_imported"`
	ConnectionsSkipped  int      `json:"connections_skipped"`
	CombosImported      int      `json:"combos_imported"`
	CombosSkipped       int      `json:"combos_skipped"`
	SkippedReasons      []string `json:"skipped_reasons,omitempty"`
}

// applyImport writes the plan in a single transaction.
//
// All-or-nothing matters here: a partial import leaves the user with an unclear
// state and no obvious way to retry, since re-running would duplicate whatever
// already landed.
func (s *Server) applyImport(conns []migrate.Connection, combos []migrate.Combo, importCombos bool) (importResult, error) {
	var res importResult

	tx, err := s.db.Conn().Begin()
	if err != nil {
		return res, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() // no-op once committed

	// Existing base URLs, so re-running an import does not create duplicates.
	// Matching on URL rather than name because the name is cosmetic and the user
	// may well have renamed a connection after a previous run.
	existing := map[string]bool{}
	rows, err := tx.Query("SELECT base_url, api_key FROM connections")
	if err == nil {
		for rows.Next() {
			var url, key string
			if rows.Scan(&url, &key) == nil {
				existing[connFingerprint(url, key)] = true
			}
		}
		rows.Close()
	}

	insert, err := tx.Prepare(`
		INSERT INTO connections (id, name, base_url, api_key, format, priority, is_active,
			chat_path, models_path, auth_header, auth_prefix)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'Authorization', 'Bearer ')`)
	if err != nil {
		return res, fmt.Errorf("prepare insert: %w", err)
	}
	defer insert.Close()

	// prefixToName lets combo members be rewritten from the source router's
	// "<prefix>/<model>" form into something meaningful in Lintasan.
	imported := map[string]bool{}

	for _, c := range conns {
		fp := connFingerprint(c.BaseURL, c.APIKey)
		if existing[fp] {
			res.ConnectionsSkipped++
			res.SkippedReasons = appendUnique(res.SkippedReasons,
				"some connections already existed and were left untouched")
			continue
		}
		active := 0
		if c.IsActive && c.Health == migrate.HealthOK {
			active = 1
		}
		chatPath, modelsPath := apiPathsFor(c.BaseURL)
		if _, err := insert.Exec(uuid.New().String(), c.Name, c.BaseURL, c.APIKey,
			c.Format, c.Priority, active, chatPath, modelsPath); err != nil {
			return res, fmt.Errorf("insert connection %q: %w", c.Name, err)
		}
		existing[fp] = true
		if c.Prefix != "" {
			imported[c.Prefix] = true
		}
		res.ConnectionsImported++
	}

	if importCombos && len(combos) > 0 {
		n, skipped, err := s.mergeCombos(tx, combos, imported)
		if err != nil {
			return res, err
		}
		res.CombosImported = n
		res.CombosSkipped = skipped
		if skipped > 0 {
			res.SkippedReasons = appendUnique(res.SkippedReasons,
				"some combos were skipped because a combo with that name already exists")
		}
	}

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("commit: %w", err)
	}
	return res, nil
}

// mergeCombos appends imported combos to the existing settings.combos array.
//
// Lintasan stores combos as a JSON array in settings rather than a table, so
// this is a read-modify-write. It runs inside the caller's transaction to stay
// consistent with the connection inserts.
func (s *Server) mergeCombos(tx *sql.Tx, combos []migrate.Combo, importedPrefixes map[string]bool) (int, int, error) {
	var raw string
	_ = tx.QueryRow("SELECT value FROM settings WHERE key = 'combos'").Scan(&raw)

	var existing []map[string]any
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &existing); err != nil {
			// Malformed existing value: refuse rather than clobber whatever is
			// there, since combos are user configuration.
			return 0, 0, fmt.Errorf("existing combos setting is not valid JSON; refusing to overwrite")
		}
	}

	taken := map[string]bool{}
	for _, c := range existing {
		if n, ok := c["name"].(string); ok {
			taken[strings.ToLower(n)] = true
		}
	}

	added, skipped := 0, 0
	for _, c := range combos {
		if taken[strings.ToLower(c.Name)] {
			skipped++
			continue
		}
		models := make([]string, 0, len(c.Models))
		for _, m := range c.Models {
			models = append(models, stripPrefix(m, importedPrefixes))
		}
		entry := map[string]any{
			"id":       uuid.New().String(),
			"name":     c.Name,
			"models":   models,
			"strategy": "priority",
		}
		if c.Partial {
			entry["description"] = fmt.Sprintf(
				"Imported from 9router — %d model(s) skipped (not migratable)", len(c.SkippedModels))
		} else {
			entry["description"] = "Imported from 9router"
		}
		existing = append(existing, entry)
		taken[strings.ToLower(c.Name)] = true
		added++
	}

	if added == 0 {
		return 0, skipped, nil
	}

	out, err := json.Marshal(existing)
	if err != nil {
		return 0, 0, fmt.Errorf("encode combos: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO settings (key, value) VALUES ('combos', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(out)); err != nil {
		return 0, 0, fmt.Errorf("save combos: %w", err)
	}
	return added, skipped, nil
}

// stripPrefix converts a source combo member ("sumo/gpt-4o") into the bare model
// id Lintasan routes on. The prefix is a 9router routing artifact and means
// nothing here.
func stripPrefix(model string, importedPrefixes map[string]bool) string {
	i := strings.Index(model, "/")
	if i <= 0 {
		return model
	}
	if importedPrefixes[model[:i]] {
		return model[i+1:]
	}
	return model
}

// --- helpers ---------------------------------------------------------------

// parseUpload reads the body and hands it to the migrate package.
//
// Accepts either a raw JSON body or a multipart form field named "file", since
// the dashboard uses a file input and CLI users will just pipe the file.
func (s *Server) parseUpload(r *http.Request) (migrate.Plan, error) {
	var data []byte
	var err error

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(maxImportBytes); err != nil {
			return migrate.Plan{}, fmt.Errorf("read upload: %w", err)
		}
		f, _, ferr := r.FormFile("file")
		if ferr != nil {
			return migrate.Plan{}, fmt.Errorf("no file field in upload")
		}
		defer f.Close()
		data, err = io.ReadAll(io.LimitReader(f, maxImportBytes))
	} else {
		data, err = io.ReadAll(io.LimitReader(r.Body, maxImportBytes))
	}
	if err != nil {
		return migrate.Plan{}, fmt.Errorf("read upload: %w", err)
	}
	if len(data) == 0 {
		return migrate.Plan{}, fmt.Errorf("empty upload")
	}

	plan, err := migrate.Parse(data, s.presetLookup())
	if err != nil {
		return migrate.Plan{}, err
	}
	return plan, nil
}

// presetLookup exposes the provider_presets table to the importer, which is how
// built-in providers (whose endpoint the export omits) get an endpoint.
func (s *Server) presetLookup() migrate.PresetLookup {
	m := map[string]string{}
	if s.db == nil {
		return migrate.MapPresets(m)
	}
	rows, err := s.db.Conn().Query("SELECT name, domain, base_url FROM provider_presets")
	if err != nil {
		return migrate.MapPresets(m)
	}
	defer rows.Close()
	for rows.Next() {
		var name, domain, baseURL string
		if rows.Scan(&name, &domain, &baseURL) != nil {
			continue
		}
		// Index under several spellings: a competitor names providers as
		// "xiaomi-mimo" where Lintasan's preset is "Xiaomi MiMo" on domain
		// "xiaomimimo.com".
		for _, k := range presetAliases(name, domain) {
			if _, exists := m[k]; !exists {
				m[k] = baseURL
			}
		}
	}
	return migrate.MapPresets(m)
}

// presetAliases produces the lookup keys a preset should answer to.
func presetAliases(name, domain string) []string {
	var out []string
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out = append(out, s)
		}
	}
	add(name)
	add(strings.ReplaceAll(name, " ", "-"))
	add(strings.ReplaceAll(name, " ", ""))
	add(domain)
	if i := strings.Index(domain, "."); i > 0 {
		add(domain[:i]) // "xiaomimimo.com" → "xiaomimimo"
	}
	return out
}

// apiPathsFor picks the chat/models paths that match a base URL. A competitor
// export stores endpoints like "https://api.example.com/v1", and appending the
// usual "/v1/chat/completions" to that yields a doubled "/v1/v1/..." path that
// 404s. When the base URL already carries the version segment we append the
// bare paths instead.
func apiPathsFor(baseURL string) (chatPath, modelsPath string) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(trimmed), "/v1") {
		return "/chat/completions", "/models"
	}
	return "/v1/chat/completions", "/v1/models"
}

func connFingerprint(baseURL, apiKey string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/")) + "\x00" + strings.TrimSpace(apiKey)
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func writeMigrateError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": msg})
}
