package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// registerRESTRoutes wires up the RESTful resource endpoints the SvelteKit
// dashboard expects (DELETE /api/keys/{id}, PATCH /api/plugins/{id}, etc).
//
// These were previously unregistered. Because Go 1.22's ServeMux is strict on
// method+path, every edit/delete/toggle button that used a REST sub-path fell
// through to the SPA catch-all and silently returned 405 — the buttons looked
// alive but did nothing. Each handler below routes to the same settings-backed
// storage the action-style handlers already use, so operations actually persist.
func (s *Server) registerRESTRoutes() {
	// API keys
	s.mux.HandleFunc("DELETE /api/keys/{id}", s.handleKeyDelete)

	// Webhooks
	s.mux.HandleFunc("PATCH /api/webhooks/{id}", s.handleWebhookPatch)
	s.mux.HandleFunc("POST /api/webhooks/{id}/test", s.handleWebhookTestByID)
	s.mux.HandleFunc("DELETE /api/webhooks/{id}", s.handleWebhookDelete)

	// Plugins
	s.mux.HandleFunc("PATCH /api/plugins/{id}", s.handlePluginPatch)
	s.mux.HandleFunc("DELETE /api/plugins/{id}", s.handlePluginDelete)
	s.mux.HandleFunc("PATCH /api/plugins/{id}/config", s.handlePluginConfig)
	s.mux.HandleFunc("POST /api/plugins/install", s.handlePluginInstall)

	// Backup lifecycle
	s.mux.HandleFunc("POST /api/backup/export", s.handleBackupExport)
	s.mux.HandleFunc("POST /api/backup/import", s.handleBackupImport)
	s.mux.HandleFunc("POST /api/backup/{id}/restore", s.handleBackupRestore)
	s.mux.HandleFunc("GET /api/backup/{id}/download", s.handleBackupDownload)
	s.mux.HandleFunc("DELETE /api/backup/{id}", s.handleBackupDelete)

	// Routing combos + aliases
	s.mux.HandleFunc("PATCH /api/routing/combos/{id}", s.handleRoutingComboPatch)
	s.mux.HandleFunc("PUT /api/routing/combos/reorder", s.handleRoutingComboReorder)
	s.mux.HandleFunc("POST /api/routing/aliases", s.handleRoutingAliasCreate)
	s.mux.HandleFunc("DELETE /api/routing/aliases/{id}", s.handleRoutingAliasDelete)

	// Fallback chains
	s.mux.HandleFunc("POST /api/fallback/model-chains", s.handleFallbackModelChainCreate)
	s.mux.HandleFunc("DELETE /api/fallback/model-chains/{id}", s.handleFallbackModelChainDelete)
	s.mux.HandleFunc("POST /api/fallback/connection-chains", s.handleFallbackConnChainCreate)
	s.mux.HandleFunc("DELETE /api/fallback/connection-chains/{id}", s.handleFallbackConnChainDelete)

	// Team members (remove a single member)
	s.mux.HandleFunc("DELETE /api/teams/{id}/members/{member}", s.handleTeamMemberDelete)

	// Quota stats (usage page)
	s.mux.HandleFunc("GET /api/quota/stats", s.handleQuotaStats)
}

// --- small type coercers for getJSONSetting (returns any from json.Unmarshal) ---

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return []any{}
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func orderValue(m map[string]any) float64 {
	switch v := m["order"].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

// ---------------------------------------------------------------- API keys

func (s *Server) handleKeyDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	arr := asSlice(s.getJSONSetting("api_keys", []any{}))
	out := make([]any, 0, len(arr))
	found := false
	for _, item := range arr {
		if fmt.Sprint(asMap(item)["id"]) == id {
			found = true
			continue
		}
		out = append(out, item)
	}
	if !found {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "key not found"})
		return
	}
	s.setJSONSetting("api_keys", out)
	s.audit("apikey.delete", "dashboard", id, nil)
	writeJSON(w, map[string]any{"id": id, "status": "deleted"})
}

// ---------------------------------------------------------------- Webhooks

func (s *Server) webhooksData() map[string]any {
	d := asMap(s.getJSONSetting("webhooks", map[string]any{"webhooks": []any{}, "history": []any{}}))
	if _, ok := d["webhooks"]; !ok {
		d["webhooks"] = []any{}
	}
	return d
}

func (s *Server) handleWebhookPatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	data := s.webhooksData()
	arr := asSlice(data["webhooks"])
	found := false
	for _, item := range arr {
		m := asMap(item)
		if fmt.Sprint(m["id"]) == id {
			for k, v := range in {
				m[k] = v
			}
			found = true
		}
	}
	if !found {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "webhook not found"})
		return
	}
	data["webhooks"] = arr
	s.setJSONSetting("webhooks", data)
	s.audit("webhook.update", "dashboard", id, in)
	writeJSON(w, map[string]any{"id": id, "status": "updated"})
}

func (s *Server) handleWebhookDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data := s.webhooksData()
	arr := asSlice(data["webhooks"])
	out := make([]any, 0, len(arr))
	found := false
	for _, item := range arr {
		if fmt.Sprint(asMap(item)["id"]) == id {
			found = true
			continue
		}
		out = append(out, item)
	}
	if !found {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "webhook not found"})
		return
	}
	data["webhooks"] = out
	s.setJSONSetting("webhooks", data)
	s.audit("webhook.delete", "dashboard", id, nil)
	writeJSON(w, map[string]any{"id": id, "status": "deleted"})
}

func (s *Server) handleWebhookTestByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data := s.webhooksData()
	var target map[string]any
	for _, item := range asSlice(data["webhooks"]) {
		m := asMap(item)
		if fmt.Sprint(m["id"]) == id {
			target = m
			break
		}
	}
	if target == nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "webhook not found"})
		return
	}
	if url, _ := target["url"].(string); url != "" {
		s.deliverWebhookTo(id, url, "test", map[string]any{"message": "Lintasan test webhook", "time": time.Now()})
	}
	writeJSON(w, map[string]any{"id": id, "status": "test_sent"})
}

// deliverWebhookTo fires a single webhook (used by the per-webhook test button),
// mirroring deliverWebhooks but targeting one endpoint instead of all.
func (s *Server) deliverWebhookTo(id, url, event string, payload map[string]any) {
	body, _ := json.Marshal(map[string]any{"event": event, "payload": payload, "timestamp": time.Now().Format(time.RFC3339)})
	go func() {
		req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		status := 0
		text := ""
		if err != nil {
			text = err.Error()
		} else {
			status = resp.StatusCode
			rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			text = string(rb)
			resp.Body.Close()
		}
		s.db.Conn().Exec("INSERT INTO webhook_deliveries(id, webhook_id, event, status, response, created_at) VALUES(?,?,?,?,?,datetime('now'))", uuid.New().String(), id, event, status, text)
	}()
}

// ---------------------------------------------------------------- Plugins

func (s *Server) handlePluginPatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	arr := asSlice(s.getJSONSetting("plugins", []any{}))
	found := false
	for _, item := range arr {
		m := asMap(item)
		if fmt.Sprint(m["id"]) == id {
			for k, v := range in {
				m[k] = v
			}
			found = true
		}
	}
	if !found {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "plugin not found"})
		return
	}
	s.setJSONSetting("plugins", arr)
	s.audit("plugin.update", "dashboard", id, in)
	writeJSON(w, map[string]any{"id": id, "status": "updated"})
}

func (s *Server) handlePluginConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	arr := asSlice(s.getJSONSetting("plugins", []any{}))
	found := false
	for _, item := range arr {
		m := asMap(item)
		if fmt.Sprint(m["id"]) == id {
			m["config"] = in["config"]
			found = true
		}
	}
	if !found {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "plugin not found"})
		return
	}
	s.setJSONSetting("plugins", arr)
	s.audit("plugin.config", "dashboard", id, nil)
	writeJSON(w, map[string]any{"id": id, "status": "configured"})
}

func (s *Server) handlePluginDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	arr := asSlice(s.getJSONSetting("plugins", []any{}))
	out := make([]any, 0, len(arr))
	found := false
	for _, item := range arr {
		if fmt.Sprint(asMap(item)["id"]) == id {
			found = true
			continue
		}
		out = append(out, item)
	}
	if !found {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "plugin not found"})
		return
	}
	s.setJSONSetting("plugins", out)
	s.audit("plugin.delete", "dashboard", id, nil)
	writeJSON(w, map[string]any{"id": id, "status": "deleted"})
}

func (s *Server) handlePluginInstall(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	pluginID := fmt.Sprint(in["pluginId"])
	if pluginID == "" || pluginID == "<nil>" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "pluginId required"})
		return
	}
	arr := asSlice(s.getJSONSetting("plugins", []any{}))
	for _, item := range arr {
		m := asMap(item)
		if fmt.Sprint(m["name"]) == pluginID || fmt.Sprint(m["id"]) == pluginID {
			writeJSON(w, map[string]any{"status": "already_installed"})
			return
		}
	}
	plugin := map[string]any{
		"id":      uuid.New().String(),
		"name":    pluginID,
		"enabled": true,
		"config":  map[string]any{},
	}
	arr = append(arr, plugin)
	s.setJSONSetting("plugins", arr)
	s.audit("plugin.install", "dashboard", pluginID, nil)
	writeJSON(w, map[string]any{"status": "installed", "plugin": plugin})
}

// ---------------------------------------------------------------- Backup

func backupRecord(name string, size int64, modTime time.Time) map[string]any {
	ts := modTime.Format(time.RFC3339)
	return map[string]any{
		"id":         name,
		"filename":   name,
		"size":       size,
		"type":       "snapshot",
		"status":     "available",
		"createdAt":  ts,
		"created_at": ts,
	}
}

func (s *Server) handleBackupExport(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(s.cfg.DataDir, "backups")
	os.MkdirAll(dir, 0755)
	name := fmt.Sprintf("lintasan-%s.db", time.Now().Format("20060102-150405"))
	data, err := os.ReadFile(s.cfg.DBPath)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "failed to read database"})
		return
	}
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, data, 0644); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "failed to write backup"})
		return
	}
	var size int64
	mt := time.Now()
	if info, e := os.Stat(dst); e == nil {
		size = info.Size()
		mt = info.ModTime()
	}
	s.audit("backup.export", "dashboard", name, nil)
	writeJSON(w, map[string]any{"backup": backupRecord(name, size, mt)})
}

func (s *Server) handleBackupImport(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	// Accept either {settings:{...}} or a flat map of setting keys.
	src := in
	if sm, ok := in["settings"].(map[string]any); ok {
		src = sm
	}
	imported := 0
	for k, v := range src {
		switch val := v.(type) {
		case string:
			s.db.SetSetting(k, val)
		default:
			if b, err := json.Marshal(val); err == nil {
				s.db.SetSetting(k, string(b))
			}
		}
		imported++
	}
	if s.proxy != nil {
		s.proxy.ReloadSmartRoutingConfig()
	}
	s.audit("backup.import", "dashboard", "import", map[string]any{"keys": imported})
	writeJSON(w, map[string]any{"status": "imported", "keys": imported})
}

func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := filepath.Join(s.cfg.DataDir, "backups", filepath.Base(id))
	if _, err := os.Stat(path); err != nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "backup not found"})
		return
	}
	src, err := sql.Open("sqlite3", path+"?_busy_timeout=5000")
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "cannot open backup"})
		return
	}
	defer src.Close()
	live := s.db.Conn()

	// Restore settings (api_keys, combos, aliases, plugins, webhooks,
	// fallback_chains, smart-routing config, etc all live here).
	settingsRestored := 0
	if rows, e := src.Query("SELECT key, value FROM settings"); e == nil {
		for rows.Next() {
			var k, v string
			if rows.Scan(&k, &v) == nil {
				if _, ie := live.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=?", k, v, v); ie == nil {
					settingsRestored++
				}
			}
		}
		rows.Close()
	}

	// Restore connections (full column set, upsert by id).
	connRestored := 0
	if rows, e := src.Query("SELECT id,name,base_url,api_key,format,chat_path,models_path,auth_header,auth_prefix,extra_headers,is_active,priority FROM connections"); e == nil {
		for rows.Next() {
			var id2, name, baseURL, apiKey, format, chatPath, modelsPath, authHeader, authPrefix, extraHeaders string
			var isActive, priority int
			if rows.Scan(&id2, &name, &baseURL, &apiKey, &format, &chatPath, &modelsPath, &authHeader, &authPrefix, &extraHeaders, &isActive, &priority) == nil {
				if _, ie := live.Exec(`INSERT INTO connections(id,name,base_url,api_key,format,chat_path,models_path,auth_header,auth_prefix,extra_headers,is_active,priority)
					VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
					ON CONFLICT(id) DO UPDATE SET name=excluded.name, base_url=excluded.base_url, api_key=excluded.api_key,
						format=excluded.format, chat_path=excluded.chat_path, models_path=excluded.models_path,
						auth_header=excluded.auth_header, auth_prefix=excluded.auth_prefix, extra_headers=excluded.extra_headers,
						is_active=excluded.is_active, priority=excluded.priority`,
					id2, name, baseURL, apiKey, format, chatPath, modelsPath, authHeader, authPrefix, extraHeaders, isActive, priority); ie == nil {
					connRestored++
				}
			}
		}
		rows.Close()
	}

	if s.proxy != nil {
		s.proxy.ReloadSmartRoutingConfig()
	}
	s.audit("backup.restore", "dashboard", id, map[string]any{"settings": settingsRestored, "connections": connRestored})
	writeJSON(w, map[string]any{"id": id, "status": "restored", "settings_restored": settingsRestored, "connections_restored": connRestored})
}

func (s *Server) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := filepath.Join(s.cfg.DataDir, "backups", filepath.Base(id))
	data, err := os.ReadFile(path)
	if err != nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "backup not found"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(id))
	w.Write(data)
}

func (s *Server) handleBackupDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := filepath.Join(s.cfg.DataDir, "backups", filepath.Base(id))
	if err := os.Remove(path); err != nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "backup not found"})
		return
	}
	s.audit("backup.delete", "dashboard", id, nil)
	writeJSON(w, map[string]any{"id": id, "status": "deleted"})
}

// ---------------------------------------------------------------- Routing

func (s *Server) handleRoutingComboPatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	combos := asSlice(s.getJSONSetting("combos", []any{}))
	found := false
	for _, item := range combos {
		m := asMap(item)
		if fmt.Sprint(m["id"]) == id {
			for k, v := range in {
				m[k] = v
			}
			found = true
		}
	}
	if !found {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "combo not found"})
		return
	}
	s.setJSONSetting("combos", combos)
	s.audit("combo.update", "dashboard", id, in)
	writeJSON(w, map[string]any{"id": id, "status": "updated"})
}

func (s *Server) handleRoutingComboReorder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Combos []struct {
			ID    string `json:"id"`
			Order int    `json:"order"`
		} `json:"combos"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	combos := asSlice(s.getJSONSetting("combos", []any{}))
	orderOf := map[string]int{}
	for _, c := range in.Combos {
		orderOf[c.ID] = c.Order
	}
	for _, item := range combos {
		m := asMap(item)
		if o, ok := orderOf[fmt.Sprint(m["id"])]; ok {
			m["order"] = o
		}
	}
	sort.SliceStable(combos, func(i, j int) bool {
		return orderValue(asMap(combos[i])) < orderValue(asMap(combos[j]))
	})
	s.setJSONSetting("combos", combos)
	s.audit("combo.reorder", "dashboard", "", map[string]any{"count": len(combos)})
	writeJSON(w, map[string]any{"status": "reordered", "count": len(combos)})
}

func (s *Server) handleRoutingAliasCreate(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	alias := fmt.Sprint(in["alias"])
	target := fmt.Sprint(in["target"])
	if alias == "" || alias == "<nil>" || target == "" || target == "<nil>" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "alias and target required"})
		return
	}
	aliases := asMap(s.getJSONSetting("aliases", map[string]any{}))
	aliases[alias] = map[string]any{"model": target}
	s.setJSONSetting("aliases", aliases)
	s.audit("alias.create", "dashboard", alias, map[string]any{"target": target})
	writeJSON(w, map[string]any{"alias": map[string]any{"id": alias, "alias": alias, "target": target}})
}

func (s *Server) handleRoutingAliasDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	aliases := asMap(s.getJSONSetting("aliases", map[string]any{}))
	if _, ok := aliases[id]; !ok {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "alias not found"})
		return
	}
	delete(aliases, id)
	s.setJSONSetting("aliases", aliases)
	s.audit("alias.delete", "dashboard", id, nil)
	writeJSON(w, map[string]any{"id": id, "status": "deleted"})
}

// ---------------------------------------------------------------- Fallback chains

func (s *Server) fallbackData() map[string]any {
	d := asMap(s.getJSONSetting("fallback_chains", map[string]any{"model_chains": []any{}, "connection_chains": []any{}}))
	if _, ok := d["model_chains"]; !ok {
		d["model_chains"] = []any{}
	}
	if _, ok := d["connection_chains"]; !ok {
		d["connection_chains"] = []any{}
	}
	return d
}

func (s *Server) handleFallbackModelChainCreate(w http.ResponseWriter, r *http.Request) {
	s.fallbackChainCreate(w, r, "model_chains")
}

func (s *Server) handleFallbackConnChainCreate(w http.ResponseWriter, r *http.Request) {
	s.fallbackChainCreate(w, r, "connection_chains")
}

func (s *Server) fallbackChainCreate(w http.ResponseWriter, r *http.Request, key string) {
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	if in == nil {
		in = map[string]any{}
	}
	in["id"] = uuid.New().String()
	in["usage_count"] = 0
	data := s.fallbackData()
	data[key] = append(asSlice(data[key]), in)
	s.setJSONSetting("fallback_chains", data)
	s.audit("fallback.create", "dashboard", fmt.Sprint(in["id"]), map[string]any{"type": key})
	writeJSON(w, map[string]any{"chain": in})
}

func (s *Server) handleFallbackModelChainDelete(w http.ResponseWriter, r *http.Request) {
	s.fallbackChainDelete(w, r, "model_chains")
}

func (s *Server) handleFallbackConnChainDelete(w http.ResponseWriter, r *http.Request) {
	s.fallbackChainDelete(w, r, "connection_chains")
}

func (s *Server) fallbackChainDelete(w http.ResponseWriter, r *http.Request, key string) {
	id := r.PathValue("id")
	data := s.fallbackData()
	arr := asSlice(data[key])
	out := make([]any, 0, len(arr))
	found := false
	for _, item := range arr {
		if fmt.Sprint(asMap(item)["id"]) == id {
			found = true
			continue
		}
		out = append(out, item)
	}
	if !found {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "chain not found"})
		return
	}
	data[key] = out
	s.setJSONSetting("fallback_chains", data)
	s.audit("fallback.delete", "dashboard", id, map[string]any{"type": key})
	writeJSON(w, map[string]any{"id": id, "status": "deleted"})
}

// ---------------------------------------------------------------- Team members

func (s *Server) handleTeamMemberDelete(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	member := r.PathValue("member")
	teams := asSlice(s.getJSONSetting("teams", []any{}))
	found := false
	for _, item := range teams {
		m := asMap(item)
		if fmt.Sprint(m["id"]) == teamID {
			members := asSlice(m["members"])
			out := make([]any, 0, len(members))
			for _, mem := range members {
				if fmt.Sprint(mem) == member {
					found = true
					continue
				}
				out = append(out, mem)
			}
			m["members"] = out
		}
	}
	if !found {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "member not found"})
		return
	}
	s.setJSONSetting("teams", teams)
	s.audit("team.member.delete", "dashboard", teamID, map[string]any{"member": member})
	writeJSON(w, map[string]any{"team_id": teamID, "member": member, "status": "removed"})
}

// ---------------------------------------------------------------- Quota stats

func (s *Server) handleQuotaStats(w http.ResponseWriter, r *http.Request) {
	writeData(w, map[string]any{
		"limits":        s.getJSONSetting("quota_limits", map[string]any{}),
		"usage":         map[string]any{"requests_today": 0, "tokens_today": 0},
		"total_today":   0,
		"by_connection": []any{},
	})
}
