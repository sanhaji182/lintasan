package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestKeyListMasksPersistedSecrets(t *testing.T) {
	s, _ := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	secret := "sk-lintasan-1234567890abcdef"
	s.setJSONSetting("api_keys", []any{map[string]any{"id": "key-1", "name": "App", "key": secret}})
	rec := httptest.NewRecorder()
	s.handleKeys(rec, httptest.NewRequest("GET", "/api/keys", nil))
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatal("key list exposed persisted plaintext key")
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0]["key"] != nil || body.Data[0]["prefix"] == nil {
		t.Fatalf("expected masked key metadata, got %s", rec.Body.String())
	}
}
