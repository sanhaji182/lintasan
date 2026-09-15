package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestModelsExposeStableQuickstartProvenance(t *testing.T) {
	s, _ := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	_, err := s.db.Conn().Exec(`INSERT INTO connections (id,name,base_url,api_key,format,is_active,priority) VALUES (?,?,?,?,?,1,10)`, "quickstart-conn", "Renamed Account", "https://example.test", "secret", "openai")
	if err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	_, err = s.db.Conn().Exec(`INSERT INTO discovered_models (id,connection_id,model_id,model_name) VALUES (?,?,?,?)`, "quickstart-model", "quickstart-conn", "callable-model", "Callable")
	if err != nil {
		t.Fatalf("seed model: %v", err)
	}

	rec := httptest.NewRecorder()
	s.handleModels(rec, httptest.NewRequest("GET", "/v1/models", nil))
	var body struct {
		Data []struct {
			ID           string `json:"id"`
			ConnectionID string `json:"connection_id"`
			Source       string `json:"source"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	for _, model := range body.Data {
		if model.ID == "callable-model" {
			if model.ConnectionID != "quickstart-conn" || model.Source != "discovered" {
				t.Fatalf("model provenance = connection_id %q source %q", model.ConnectionID, model.Source)
			}
			return
		}
	}
	t.Fatal("seeded callable model missing")
}
