package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/expprovider"
	"github.com/sanhaji182/lintasan-go/internal/hoplite"
)

func hopliteRequest(t *testing.T, ts *httptest.Server, method, path, body, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func decodeEnvelope(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func TestHopliteRoutesRequireLintasanAuthentication(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-1234567890"}
	s, ts := newTestServer(t, cfg)
	makeKnownAdmin(t, s, "hoplite-admin", "correct horse battery")

	for _, path := range []string{
		"/api/experimental/cloud-agents/hoplite/status",
		"/api/experimental/cloud-agents/hoplite/projects",
		"/api/experimental/cloud-agents/hoplite/threads?projectId=proj_1",
	} {
		resp := hopliteRequest(t, ts, http.MethodGet, path, "", "")
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("anonymous GET %s = %d, want 401", path, resp.StatusCode)
		}
	}
}

func TestCredentialStoreFallsBackToConfiguredMasterKey(t *testing.T) {
	const configuredMaster = "config-only-master-key"
	s, _ := newTestServer(t, &config.Config{MasterKey: configuredMaster})
	if dbMaster, _ := s.db.GetSetting("master_key"); dbMaster != "" {
		t.Fatalf("test requires empty DB master key, got %q", dbMaster)
	}
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatalf("set credential: %v", err)
	}
	reader := expprovider.NewDashboardCredentialStore(s.db.Conn(), configuredMaster)
	if secret, ok := reader.GetCredential(context.Background(), "hoplite"); !ok || secret != "hop_test" {
		t.Fatalf("credential cannot be decrypted with configured master key: secret=%q ok=%v", secret, ok)
	}
}

func TestHopliteRoutesRequireAdminForDashboardUsers(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	if _, err := s.userMgr.CreateUser("hoplite-viewer", "correct horse battery", "user"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, _, err := s.userMgr.Authenticate("hoplite-viewer", "correct horse battery")
	if err != nil {
		t.Fatalf("authenticate user: %v", err)
	}
	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/experimental/cloud-agents/hoplite/status", ""},
		{http.MethodPut, "/api/experimental/credentials/hoplite", `{"credential":"hop_test"}`},
		{http.MethodPost, "/api/experimental/cloud-agents/hoplite/threads", `{"projectId":"proj_1","prompt":"do work"}`},
	} {
		resp := hopliteRequest(t, ts, request.method, request.path, request.body, token)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("non-admin %s %s = %d, want 403", request.method, request.path, resp.StatusCode)
		}
	}
}

func TestHopliteStatusAndTestAreReadOnlyAndCredentialBacked(t *testing.T) {
	const hopliteKey = "hop_test_secret"
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/api/projects" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != hopliteKey {
			t.Fatalf("upstream key = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"projects":[{"id":"proj_1","name":"acme/app"}]}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{MasterKey: "test-master-key-1234567890"}
	s, ts := newTestServer(t, cfg)
	token := makeKnownAdmin(t, s, "hoplite-admin-2", "correct horse battery")
	s.hopliteBaseURL = upstream.URL
	s.hopliteHTTPClient = upstream.Client()

	resp := hopliteRequest(t, ts, http.MethodGet, "/api/experimental/cloud-agents/hoplite/status", "", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status before credential = %d", resp.StatusCode)
	}
	statusBody := decodeEnvelope(t, resp)
	status := statusBody["data"].(map[string]any)
	if status["configured"] != false || calls != 0 {
		t.Fatalf("status must be local/read-only: body=%#v calls=%d", statusBody, calls)
	}

	resp = hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/test", `{}`, token)
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("test without credential = %d, want 412", resp.StatusCode)
	}
	resp.Body.Close()
	if calls != 0 {
		t.Fatalf("missing credential contacted upstream %d times", calls)
	}

	resp = hopliteRequest(t, ts, http.MethodPut, "/api/experimental/credentials/hoplite", `{"credential":"`+hopliteKey+`"}`, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set Hoplite credential = %d", resp.StatusCode)
	}
	resp.Body.Close()
	var stored string
	if err := s.db.Conn().QueryRow(`SELECT encrypted_value FROM experimental_credentials WHERE provider_name = 'hoplite'`).Scan(&stored); err != nil {
		t.Fatalf("read stored credential: %v", err)
	}
	if stored == hopliteKey || strings.Contains(stored, hopliteKey) {
		t.Fatal("Hoplite credential was stored in plaintext")
	}
	resp = hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/test", `{}`, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("test with credential = %d", resp.StatusCode)
	}
	body := decodeEnvelope(t, resp)
	data := body["data"].(map[string]any)
	if data["ok"] != true || data["project_count"] != float64(1) || calls != 1 {
		t.Fatalf("unexpected test response=%#v calls=%d", body, calls)
	}
	encoded, _ := json.Marshal(body)
	if strings.Contains(string(encoded), hopliteKey) {
		t.Fatal("response leaked Hoplite key")
	}
}

func TestHopliteProjectAndThreadHandlersProxyTypedOperations(t *testing.T) {
	var createPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/projects":
			_, _ = w.Write([]byte(`{"ok":true,"projects":[{"id":"proj_1","name":"acme/app"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads":
			if r.URL.Query().Get("projectId") != "proj_1" {
				t.Fatalf("missing project filter: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"ok":true,"threads":[{"id":"thr_1","projectId":"proj_1","status":"running"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/threads":
			if err := json.NewDecoder(r.Body).Decode(&createPayload); err != nil {
				t.Fatalf("decode create payload: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_2","projectId":"proj_1","status":"queued"},"run":{"id":"run_2","status":"queued"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_1":
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_1","projectId":"proj_1","status":"succeeded"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_1/messages":
			_, _ = w.Write([]byte(`{"ok":true,"messages":[{"id":"msg_1","role":"assistant","content":"Done"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	token := makeKnownAdmin(t, s, "hoplite-admin-3", "correct horse battery")
	s.hopliteBaseURL = upstream.URL
	s.hopliteHTTPClient = upstream.Client()
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatalf("set credential: %v", err)
	}

	checks := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodGet, "/api/experimental/cloud-agents/hoplite/projects", "", 200},
		{http.MethodGet, "/api/experimental/cloud-agents/hoplite/threads?projectId=proj_1", "", 200},
		{http.MethodPost, "/api/experimental/cloud-agents/hoplite/threads", `{"projectId":"proj_1","prompt":"Fix tests","title":"Fix tests"}`, 201},
		{http.MethodGet, "/api/experimental/cloud-agents/hoplite/threads/thr_1", "", 200},
		{http.MethodGet, "/api/experimental/cloud-agents/hoplite/threads/thr_1/messages", "", 200},
	}
	for _, check := range checks {
		resp := hopliteRequest(t, ts, check.method, check.path, check.body, token)
		if resp.StatusCode != check.want {
			body := decodeEnvelope(t, resp)
			t.Fatalf("%s %s = %d want %d: %#v", check.method, check.path, resp.StatusCode, check.want, body)
		}
		resp.Body.Close()
	}
	if createPayload["autoFix"] != false || createPayload["autoMerge"] != false {
		t.Fatalf("unsafe create defaults: %#v", createPayload)
	}
	if strings.TrimSpace(createPayload["clientOperationId"].(string)) == "" {
		t.Fatalf("missing generated idempotency key: %#v", createPayload)
	}
}

func TestHopliteUpstreamAuthFailureDoesNotExpireLintasanSession(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		recorder := httptest.NewRecorder()
		writeHopliteError(recorder, &hoplite.UpstreamError{StatusCode: status, Code: "auth_required", RequestID: "hop_req"})
		if recorder.Code != http.StatusBadGateway {
			t.Fatalf("upstream status %d mapped to %d, want 502 to preserve dashboard session", status, recorder.Code)
		}
	}
}

func TestHopliteCreateThreadRejectsInvalidInputBeforeUpstream(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer upstream.Close()

	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	token := makeKnownAdmin(t, s, "hoplite-admin-4", "correct horse battery")
	s.hopliteBaseURL = upstream.URL
	s.hopliteHTTPClient = upstream.Client()
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatalf("set credential: %v", err)
	}

	for _, body := range []string{`{}`, `{"projectId":"proj_1"}`, `{"prompt":"do it"}`} {
		resp := hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/threads", body, token)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid body %s = %d, want 400", body, resp.StatusCode)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid requests reached upstream %d times", calls)
	}
}
