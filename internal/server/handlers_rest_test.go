package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// newRESTTestServer builds a minimal Server backed by an in-memory DB for
// exercising the RESTful resource handlers directly (no middleware/auth — these
// tests verify the handler logic + persistence, not the auth chain).
func newRESTTestServer(t *testing.T) *Server {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return New(&config.Config{}, database)
}

// reqWithPath builds a request carrying the given mux path values.
func reqWithPath(method, target string, body any, pathVals map[string]string) *http.Request {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, target, rdr)
	for k, v := range pathVals {
		r.SetPathValue(k, v)
	}
	return r
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
	}
	return out
}

// ---------------------------------------------------------------- API keys

func TestKeyDelete_RemovesAndPersists(t *testing.T) {
	s := newRESTTestServer(t)
	s.setJSONSetting("api_keys", []any{
		map[string]any{"id": "k1", "name": "one"},
		map[string]any{"id": "k2", "name": "two"},
	})
	rec := httptest.NewRecorder()
	s.handleKeyDelete(rec, reqWithPath("DELETE", "/api/keys/k1", nil, map[string]string{"id": "k1"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete existing key: got %d, want 200", rec.Code)
	}
	remaining := asSlice(s.getJSONSetting("api_keys", []any{}))
	if len(remaining) != 1 || asMap(remaining[0])["id"] != "k2" {
		t.Fatalf("expected only k2 to remain, got %v", remaining)
	}
}

func TestKeyDelete_NotFound(t *testing.T) {
	s := newRESTTestServer(t)
	s.setJSONSetting("api_keys", []any{map[string]any{"id": "k1"}})
	rec := httptest.NewRecorder()
	s.handleKeyDelete(rec, reqWithPath("DELETE", "/api/keys/zzz", nil, map[string]string{"id": "zzz"}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing key: got %d, want 404", rec.Code)
	}
}

// ---------------------------------------------------------------- Plugins

func TestPluginToggle_Persists(t *testing.T) {
	s := newRESTTestServer(t)
	s.setJSONSetting("plugins", []any{map[string]any{"id": "p1", "enabled": true}})
	rec := httptest.NewRecorder()
	s.handlePluginPatch(rec, reqWithPath("PATCH", "/api/plugins/p1", map[string]any{"enabled": false}, map[string]string{"id": "p1"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch plugin: got %d, want 200", rec.Code)
	}
	arr := asSlice(s.getJSONSetting("plugins", []any{}))
	if asMap(arr[0])["enabled"] != false {
		t.Fatalf("expected plugin disabled after patch, got %v", arr[0])
	}
}

func TestPluginDelete_Persists(t *testing.T) {
	s := newRESTTestServer(t)
	s.setJSONSetting("plugins", []any{
		map[string]any{"id": "p1"}, map[string]any{"id": "p2"},
	})
	rec := httptest.NewRecorder()
	s.handlePluginDelete(rec, reqWithPath("DELETE", "/api/plugins/p1", nil, map[string]string{"id": "p1"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete plugin: got %d, want 200", rec.Code)
	}
	arr := asSlice(s.getJSONSetting("plugins", []any{}))
	if len(arr) != 1 || asMap(arr[0])["id"] != "p2" {
		t.Fatalf("expected only p2 to remain, got %v", arr)
	}
}

func TestPluginConfig_Persists(t *testing.T) {
	s := newRESTTestServer(t)
	s.setJSONSetting("plugins", []any{map[string]any{"id": "p1"}})
	rec := httptest.NewRecorder()
	cfg := map[string]any{"threshold": float64(5)}
	s.handlePluginConfig(rec, reqWithPath("PATCH", "/api/plugins/p1/config", map[string]any{"config": cfg}, map[string]string{"id": "p1"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("config plugin: got %d, want 200", rec.Code)
	}
	arr := asSlice(s.getJSONSetting("plugins", []any{}))
	stored := asMap(asMap(arr[0])["config"])
	if stored["threshold"] != float64(5) {
		t.Fatalf("expected config persisted, got %v", asMap(arr[0])["config"])
	}
}

func TestPluginInstall_AddsOnce(t *testing.T) {
	s := newRESTTestServer(t)
	// First install
	rec := httptest.NewRecorder()
	s.handlePluginInstall(rec, reqWithPath("POST", "/api/plugins/install", map[string]any{"pluginId": "Request Logger"}, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("install: got %d", rec.Code)
	}
	if got := len(asSlice(s.getJSONSetting("plugins", []any{}))); got != 1 {
		t.Fatalf("expected 1 plugin after install, got %d", got)
	}
	// Duplicate install must not add a second entry
	rec2 := httptest.NewRecorder()
	s.handlePluginInstall(rec2, reqWithPath("POST", "/api/plugins/install", map[string]any{"pluginId": "Request Logger"}, nil))
	if got := len(asSlice(s.getJSONSetting("plugins", []any{}))); got != 1 {
		t.Fatalf("expected install to be idempotent, got %d plugins", got)
	}
}

// ---------------------------------------------------------------- Webhooks

func TestWebhookPatchAndDelete(t *testing.T) {
	s := newRESTTestServer(t)
	s.setJSONSetting("webhooks", map[string]any{
		"webhooks": []any{map[string]any{"id": "w1", "active": true, "url": "http://x"}},
		"history":  []any{},
	})
	// toggle active=false
	rec := httptest.NewRecorder()
	s.handleWebhookPatch(rec, reqWithPath("PATCH", "/api/webhooks/w1", map[string]any{"active": false}, map[string]string{"id": "w1"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch webhook: got %d", rec.Code)
	}
	data := asMap(s.getJSONSetting("webhooks", map[string]any{}))
	if asMap(asSlice(data["webhooks"])[0])["active"] != false {
		t.Fatalf("expected webhook deactivated")
	}
	// delete
	rec2 := httptest.NewRecorder()
	s.handleWebhookDelete(rec2, reqWithPath("DELETE", "/api/webhooks/w1", nil, map[string]string{"id": "w1"}))
	if rec2.Code != http.StatusOK {
		t.Fatalf("delete webhook: got %d", rec2.Code)
	}
	data = asMap(s.getJSONSetting("webhooks", map[string]any{}))
	if len(asSlice(data["webhooks"])) != 0 {
		t.Fatalf("expected webhook removed")
	}
}

// ---------------------------------------------------------------- Routing

func TestRoutingAliasCreateAndDelete(t *testing.T) {
	s := newRESTTestServer(t)
	rec := httptest.NewRecorder()
	s.handleRoutingAliasCreate(rec, reqWithPath("POST", "/api/routing/aliases", map[string]any{"alias": "fast", "target": "openai/gpt-4o"}, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("create alias: got %d", rec.Code)
	}
	aliases := asMap(s.getJSONSetting("aliases", map[string]any{}))
	if _, ok := aliases["fast"]; !ok {
		t.Fatalf("expected alias 'fast' stored, got %v", aliases)
	}
	// delete
	rec2 := httptest.NewRecorder()
	s.handleRoutingAliasDelete(rec2, reqWithPath("DELETE", "/api/routing/aliases/fast", nil, map[string]string{"id": "fast"}))
	if rec2.Code != http.StatusOK {
		t.Fatalf("delete alias: got %d", rec2.Code)
	}
	aliases = asMap(s.getJSONSetting("aliases", map[string]any{}))
	if _, ok := aliases["fast"]; ok {
		t.Fatalf("expected alias removed, got %v", aliases)
	}
}

func TestRoutingAliasCreate_Validation(t *testing.T) {
	s := newRESTTestServer(t)
	rec := httptest.NewRecorder()
	s.handleRoutingAliasCreate(rec, reqWithPath("POST", "/api/routing/aliases", map[string]any{"alias": "x"}, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing target, got %d", rec.Code)
	}
}

func TestRoutingComboReorder(t *testing.T) {
	s := newRESTTestServer(t)
	s.setJSONSetting("combos", []any{
		map[string]any{"id": "a", "order": float64(0)},
		map[string]any{"id": "b", "order": float64(1)},
	})
	rec := httptest.NewRecorder()
	body := map[string]any{"combos": []any{
		map[string]any{"id": "a", "order": 1},
		map[string]any{"id": "b", "order": 0},
	}}
	s.handleRoutingComboReorder(rec, reqWithPath("PUT", "/api/routing/combos/reorder", body, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("reorder: got %d", rec.Code)
	}
	combos := asSlice(s.getJSONSetting("combos", []any{}))
	if asMap(combos[0])["id"] != "b" {
		t.Fatalf("expected b first after reorder, got %v", combos)
	}
}

// ---------------------------------------------------------------- Fallback chains

func TestFallbackChainCreateAndDelete(t *testing.T) {
	s := newRESTTestServer(t)
	rec := httptest.NewRecorder()
	s.handleFallbackModelChainCreate(rec, reqWithPath("POST", "/api/fallback/model-chains", map[string]any{"name": "primary", "chain": []any{"a", "b"}}, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("create chain: got %d", rec.Code)
	}
	out := decodeBody(t, rec)
	chain := asMap(out["chain"])
	id := chain["id"].(string)
	if id == "" {
		t.Fatal("expected chain id assigned")
	}
	data := s.fallbackData()
	if len(asSlice(data["model_chains"])) != 1 {
		t.Fatalf("expected 1 model chain stored")
	}
	// delete it
	rec2 := httptest.NewRecorder()
	s.handleFallbackModelChainDelete(rec2, reqWithPath("DELETE", "/api/fallback/model-chains/"+id, nil, map[string]string{"id": id}))
	if rec2.Code != http.StatusOK {
		t.Fatalf("delete chain: got %d", rec2.Code)
	}
	data = s.fallbackData()
	if len(asSlice(data["model_chains"])) != 0 {
		t.Fatalf("expected model chain removed")
	}
}

func TestFallbackConnChainSeparateBucket(t *testing.T) {
	s := newRESTTestServer(t)
	rec := httptest.NewRecorder()
	s.handleFallbackConnChainCreate(rec, reqWithPath("POST", "/api/fallback/connection-chains", map[string]any{"name": "c"}, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("create conn chain: got %d", rec.Code)
	}
	data := s.fallbackData()
	if len(asSlice(data["connection_chains"])) != 1 {
		t.Fatalf("expected 1 connection chain")
	}
	if len(asSlice(data["model_chains"])) != 0 {
		t.Fatalf("connection chain must not leak into model_chains bucket")
	}
}

// ---------------------------------------------------------------- Team members

func TestTeamMemberAddAndDelete(t *testing.T) {
	s := newRESTTestServer(t)
	s.setJSONSetting("teams", []any{
		map[string]any{"id": "t1", "name": "core", "members": []any{"alice"}},
	})
	// add member via POST
	recAdd := httptest.NewRecorder()
	s.handleTeamMembers(recAdd, reqWithPath("POST", "/api/teams/t1/members", map[string]any{"username": "bob"}, map[string]string{"id": "t1"}))
	if recAdd.Code != http.StatusOK {
		t.Fatalf("add member: got %d", recAdd.Code)
	}
	teams := asSlice(s.getJSONSetting("teams", []any{}))
	if len(asSlice(asMap(teams[0])["members"])) != 2 {
		t.Fatalf("expected 2 members after add, got %v", asMap(teams[0])["members"])
	}
	// remove alice
	recDel := httptest.NewRecorder()
	s.handleTeamMemberDelete(recDel, reqWithPath("DELETE", "/api/teams/t1/members/alice", nil, map[string]string{"id": "t1", "member": "alice"}))
	if recDel.Code != http.StatusOK {
		t.Fatalf("remove member: got %d", recDel.Code)
	}
	teams = asSlice(s.getJSONSetting("teams", []any{}))
	members := asSlice(asMap(teams[0])["members"])
	if len(members) != 1 || members[0] != "bob" {
		t.Fatalf("expected only bob to remain, got %v", members)
	}
}

func TestTeamDelete_Persists(t *testing.T) {
	s := newRESTTestServer(t)
	s.setJSONSetting("teams", []any{
		map[string]any{"id": "t1"}, map[string]any{"id": "t2"},
	})
	rec := httptest.NewRecorder()
	r := reqWithPath("DELETE", "/api/teams/t1", nil, map[string]string{"id": "t1"})
	s.handleTeamByID(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete team: got %d", rec.Code)
	}
	teams := asSlice(s.getJSONSetting("teams", []any{}))
	if len(teams) != 1 || asMap(teams[0])["id"] != "t2" {
		t.Fatalf("expected only t2 to remain, got %v", teams)
	}
}

// ---------------------------------------------------------------- Backup

func TestBackupExportImportRoundTrip(t *testing.T) {
	s := newRESTTestServer(t)
	// import a couple of settings
	rec := httptest.NewRecorder()
	body := map[string]any{"settings": map[string]any{
		"lb_strategy": "round-robin",
		"aliases":     map[string]any{"x": map[string]any{"model": "openai/x"}},
	}}
	s.handleBackupImport(rec, reqWithPath("POST", "/api/backup/import", body, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("import: got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if v, _ := s.db.GetSetting("lb_strategy"); v != "round-robin" {
		t.Fatalf("expected lb_strategy imported, got %q", v)
	}
	// aliases should be stored as JSON
	aliases := asMap(s.getJSONSetting("aliases", map[string]any{}))
	if _, ok := aliases["x"]; !ok {
		t.Fatalf("expected alias x imported, got %v", aliases)
	}
}

// TestWebhookCreate_NoActionField guards the staging-caught bug: the SvelteKit
// form posts {url, events, secret} with no action/name field, and the create
// path must still persist the webhook (previously fell through to {status:ok}).
func TestWebhookCreate_NoActionField(t *testing.T) {
	s := newRESTTestServer(t)
	rec := httptest.NewRecorder()
	body := map[string]any{"url": "http://127.0.0.1:9/hook", "events": []any{"request.completed"}}
	s.handleWebhooksAction(rec, reqWithPath("POST", "/api/webhooks", body, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("create webhook: got %d", rec.Code)
	}
	data := asMap(s.getJSONSetting("webhooks", map[string]any{}))
	hooks := asSlice(data["webhooks"])
	if len(hooks) != 1 {
		t.Fatalf("expected 1 webhook persisted from {url,events} payload, got %d", len(hooks))
	}
	if asMap(hooks[0])["active"] != true {
		t.Fatalf("expected new webhook active=true, got %v", hooks[0])
	}
}
