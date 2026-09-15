package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestHopliteLegacyCredentialMigratesWithoutCiphertextRewrite(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	token := makeKnownAdmin(t, s, "legacy-account-admin", "correct horse battery")
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_legacy_secret"); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := s.db.Conn().QueryRow(`SELECT encrypted_value FROM experimental_credentials WHERE provider_name='hoplite'`).Scan(&before); err != nil {
		t.Fatal(err)
	}

	resp := hopliteRequest(t, ts, http.MethodGet, "/api/experimental/cloud-agents/hoplite/accounts", "", token)
	body := decodeEnvelope(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%#v", resp.StatusCode, body)
	}
	encoded, _ := json.Marshal(body)
	for _, want := range []string{`"id":"hoplite-cloud-agent"`, `"name":"Hoplite"`, `"credential_masked":"hop_le*******cret"`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("missing %s: %s", want, encoded)
		}
	}
	var after, credentialName string
	if err := s.db.Conn().QueryRow(`SELECT encrypted_value FROM experimental_credentials WHERE provider_name='hoplite'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("legacy ciphertext was rewritten")
	}
	if err := s.db.Conn().QueryRow(`SELECT credential_name FROM hoplite_accounts WHERE id=?`, hopliteConnectionID).Scan(&credentialName); err != nil {
		t.Fatal(err)
	}
	if credentialName != "hoplite" {
		t.Fatalf("credential name=%q", credentialName)
	}
}

func TestHopliteAccountCRUDIsIsolatedAndMasked(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	token := makeKnownAdmin(t, s, "account-crud-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/accounts", `{"name":"Work","credential":"hop_work_secret_123"}`, token)
	data := decodeEnvelope(t, resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create=%d %#v", resp.StatusCode, data)
	}
	account := data["data"].(map[string]any)
	id := account["id"].(string)
	if id == hopliteConnectionID || !strings.HasPrefix(id, hopliteConnectionID+"-") {
		t.Fatalf("bad id=%q", id)
	}
	encoded, _ := json.Marshal(data)
	if strings.Contains(string(encoded), "hop_work_secret_123") {
		t.Fatal("plaintext leaked")
	}

	resp = hopliteRequest(t, ts, http.MethodPatch, "/api/experimental/cloud-agents/hoplite/accounts/"+id, `{"name":"Renamed","credential":"hop_wo********t_123","is_active":false}`, token)
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("masked patch=%d %s", resp.StatusCode, b)
	}
	key, ok := s.hopliteCredentialForAccount(context.Background(), id)
	if !ok || key != "hop_work_secret_123" {
		t.Fatalf("secret changed key=%q ok=%v", key, ok)
	}

	resp = hopliteRequest(t, ts, http.MethodPatch, "/api/experimental/cloud-agents/hoplite/accounts/"+id, `{"name":"Renamed","is_active":false}`, token)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("patch=%d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
	var name string
	var active int
	if err := s.db.Conn().QueryRow(`SELECT name,is_active FROM hoplite_accounts WHERE id=?`, id).Scan(&name, &active); err != nil {
		t.Fatal(err)
	}
	if name != "Renamed" || active != 0 {
		t.Fatalf("name=%s active=%d", name, active)
	}

	resp = hopliteRequest(t, ts, http.MethodDelete, "/api/experimental/cloud-agents/hoplite/accounts/"+id, "", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete=%d", resp.StatusCode)
	}
	resp.Body.Close()
	if _, ok := s.hopliteCredentialForAccount(context.Background(), id); ok {
		t.Fatal("deleted account credential remains")
	}
}

func TestHopliteAccountQualifiedModelsDoNotCollide(t *testing.T) {
	id1 := hopliteSelectedModelIDForAccount(hopliteConnectionID, "same-project", "meta/muse-spark-1.3")
	id2 := hopliteSelectedModelIDForAccount("hoplite-cloud-agent/account-two", "same-project", "meta/muse-spark-1.3")
	if id1 == id2 {
		t.Fatal("account models collide")
	}
	account, project, model, selected, ok := parseHopliteRoutedModelID(id2)
	if !ok || !selected || account != "hoplite-cloud-agent/account-two" || project != "same-project" || model != "meta/muse-spark-1.3" {
		t.Fatalf("round trip failed: %q %q %q %v %v", account, project, model, selected, ok)
	}
}

func TestHopliteInactiveAccountCannotRoute(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer upstream.Close()
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	token := makeKnownAdmin(t, s, "inactive-route-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/accounts", `{"name":"Off","credential":"hop_off_secret"}`, token)
	account := decodeEnvelope(t, resp)["data"].(map[string]any)
	id := account["id"].(string)
	_, _ = s.db.Conn().Exec(`UPDATE hoplite_accounts SET is_active=0 WHERE id=?`, id)
	model := hopliteProjectModelIDForAccount(id, "proj")
	resp = hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", `{"model":"`+model+`","messages":[{"role":"user","content":"do work"}]}`, token)
	if resp.StatusCode != http.StatusServiceUnavailable {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
	if calls != 0 {
		t.Fatalf("inactive account contacted upstream %d times", calls)
	}
}

func TestHopliteAccountRoutesRequireAuthentication(t *testing.T) {
	_, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	for _, tc := range []struct{ method, path, body string }{{"GET", "/api/experimental/cloud-agents/hoplite/accounts", ""}, {"POST", "/api/experimental/cloud-agents/hoplite/accounts", `{"name":"x","credential":"secret"}`}} {
		resp := hopliteRequest(t, ts, tc.method, tc.path, tc.body, "")
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s %s=%d", tc.method, tc.path, resp.StatusCode)
		}
	}
}

func TestHopliteAccountPatchRejectsInvalidFieldTypes(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	token := makeKnownAdmin(t, s, "account-patch-admin", "correct horse battery")
	created := hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/accounts", `{"name":"Typed","credential":"hop_typed_secret"}`, token)
	id := decodeEnvelope(t, created)["data"].(map[string]any)["id"].(string)

	for _, body := range []string{`{"name":42}`, `{"is_active":"yes"}`, `{"unknown":true}`} {
		resp := hopliteRequest(t, ts, http.MethodPatch, "/api/experimental/cloud-agents/hoplite/accounts/"+id, body, token)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d, want 400", body, resp.StatusCode)
		}
	}
}

func TestHopliteDisabledAccountCannotUseManagementOperations(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	token := makeKnownAdmin(t, s, "disabled-management-admin", "correct horse battery")
	created := hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/accounts", `{"name":"Disabled","credential":"hop_disabled_secret"}`, token)
	id := decodeEnvelope(t, created)["data"].(map[string]any)["id"].(string)
	_, _ = s.db.Conn().Exec(`UPDATE hoplite_accounts SET is_active=0 WHERE id=?`, id)

	resp := hopliteRequest(t, ts, http.MethodGet, "/api/experimental/cloud-agents/hoplite/projects?account_id="+id, "", token)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", resp.StatusCode)
	}
}
