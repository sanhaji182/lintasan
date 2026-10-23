package server

import (
	"time"

	"github.com/sanhaji182/lintasan-go/internal/db"
)

// SeedBuiltinPresets brings provider_presets up to the current catalogue.
//
// It is safe to call on every startup: a built-in whose name and category
// already match is left untouched (no updated_at churn), and a manual preset
// that shares an endpoint is never rewritten.
func SeedBuiltinPresets(database *db.DB) (inserted, updated, skipped int) {
	if database == nil {
		return
	}
	conn := database.Conn()
	if conn == nil {
		return
	}

	// oldName -> currentName for presets that have been renamed. Matching the
	// old name first is what lets a rename update the existing row instead of
	// inserting a second one under the new name.
	renames := map[string]string{
		"gitlawb":                     "GitLawb",
		"Alibaba Model Studio (Intl)": "Alibaba Model Studio Intl",
		"GLM (China)":                 "GLM China",
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	for _, p := range seedCatalogue() {
		// Find the existing row, preferring the current name, then a prior
		// name, then the endpoint. base_url is the stable identity.
		var id, currentName, currentCategory string
		var isBuiltin int
		row := conn.QueryRow(
			`SELECT id, is_builtin, name, category FROM provider_presets
			   WHERE lower(name) = lower(?)
			      OR lower(name) = lower(?)
			      OR lower(rtrim(base_url, '/')) = lower(rtrim(?, '/'))
			   LIMIT 1`,
			p.Name, oldNameFor(renames, p.Name), p.BaseURL,
		)
		if err := row.Scan(&id, &isBuiltin, &currentName, &currentCategory); err == nil && id != "" {
			if isBuiltin != 1 {
				skipped++ // manual preset shares the endpoint; leave it alone
				continue
			}
			if currentName == p.Name && currentCategory == p.Category {
				continue // already current; no write on repeated boots
			}
			// Refresh a built-in in place: name and category only. Never the
			// base_url — that is the identity we matched on — and never a
			// manual preset.
			if _, err := conn.Exec(
				`UPDATE provider_presets SET name = ?, category = ?, updated_at = ? WHERE id = ? AND is_builtin = 1`,
				p.Name, p.Category, now, id,
			); err == nil {
				updated++
			}
			continue
		}

		newID, _ := generatePresetID()
		_, err := conn.Exec(`
			INSERT INTO provider_presets (id, name, domain, base_url, format, key_label, category, is_builtin, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
		`, newID, p.Name, p.Domain, p.BaseURL, p.Format, p.KeyLabel, p.Category, now, now)
		if err == nil {
			inserted++
		}
	}
	return inserted, updated, skipped
}

// oldNameFor returns the previous name of a preset if it was renamed, so the
// seeder can find and update the existing row. Returns the current name when
// there is no rename, which makes the OR clause a harmless no-op.
func oldNameFor(renames map[string]string, current string) string {
	for old, cur := range renames {
		if cur == current {
			return old
		}
	}
	return current
}
