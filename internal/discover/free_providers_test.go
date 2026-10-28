package discover

import (
	"testing"
)

func TestGetFreeProviders(t *testing.T) {
	providers := GetFreeProviders()
	if len(providers) < 5 {
		t.Fatalf("expected at least 5 free providers, got %d", len(providers))
	}

	// Check each provider has required fields
	for _, p := range providers {
		if p.Name == "" {
			t.Error("provider name should not be empty")
		}
		if p.Prefix == "" {
			t.Errorf("%s: prefix should not be empty", p.Name)
		}
		if p.BaseURL == "" {
			t.Errorf("%s: base_url should not be empty", p.Name)
		}
		if len(p.Models) == 0 {
			t.Errorf("%s: should have at least 1 model", p.Name)
		}
	}
}

func TestGetFreeProviderByPrefix(t *testing.T) {
	tests := []struct {
		prefix   string
		wantName string
		wantNil  bool
	}{
		{"kr/", "Kiro AI", false},
		{"pol/", "Pollinations", false},
		{"cf/", "Cloudflare AI", false},
		{"nvidia/", "NVIDIA NIM", false},
		{"cerebras/", "Cerebras", false},
		{"invalid/", "", true},
	}

	for _, tt := range tests {
		p := GetFreeProviderByPrefix(tt.prefix)
		if tt.wantNil {
			if p != nil {
				t.Errorf("prefix %s: expected nil, got %s", tt.prefix, p.Name)
			}
			continue
		}
		if p == nil {
			t.Errorf("prefix %s: expected %s, got nil", tt.prefix, tt.wantName)
			continue
		}
		if p.Name != tt.wantName {
			t.Errorf("prefix %s: expected name %s, got %s", tt.prefix, tt.wantName, p.Name)
		}
	}
}

func TestGetFreeProviderByName(t *testing.T) {
	p := GetFreeProviderByName("Kiro AI")
	if p == nil {
		t.Fatal("expected Kiro AI, got nil")
	}
	if p.Prefix != "kr/" {
		t.Errorf("expected prefix kr/, got %s", p.Prefix)
	}

	if p := GetFreeProviderByName("NonExistent"); p != nil {
		t.Error("expected nil for non-existent provider")
	}
}

func TestGetAllFreeModels(t *testing.T) {
	models := GetAllFreeModels()
	if len(models) < 5 {
		t.Fatalf("expected at least 5 providers in model map, got %d", len(models))
	}

	// Check Kiro has models
	kiroModels, ok := models["kr/"]
	if !ok {
		t.Fatal("expected kr/ in model map")
	}
	if len(kiroModels) == 0 {
		t.Error("Kiro should have at least 1 model")
	}
}

func TestFreeProviderAuthTypes(t *testing.T) {
	providers := GetFreeProviders()
	authTypes := make(map[string]bool)
	for _, p := range providers {
		authTypes[p.AuthType] = true
	}

	// Should have both "none" and "apikey" auth types
	if !authTypes["none"] {
		t.Error("expected at least one provider with auth_type=none")
	}
	if !authTypes["apikey"] {
		t.Error("expected at least one provider with auth_type=apikey")
	}
}

func TestFreeProviderPrefixes(t *testing.T) {
	providers := GetFreeProviders()
	seen := make(map[string]bool)
	for _, p := range providers {
		if seen[p.Prefix] {
			t.Errorf("duplicate prefix: %s", p.Prefix)
		}
		seen[p.Prefix] = true

		// Prefix should end with /
		if len(p.Prefix) == 0 || p.Prefix[len(p.Prefix)-1] != '/' {
			t.Errorf("%s: prefix should end with /, got %s", p.Name, p.Prefix)
		}
	}
}

func TestEnabledFreeProvidersAreConnectable(t *testing.T) {
	enabled := 0
	for _, p := range GetFreeProviders() {
		if !p.Enabled {
			continue
		}
		enabled++
		if p.BaseURL == "" || len(p.Models) == 0 {
			t.Errorf("%s: enabled provider is missing base URL or models", p.Name)
		}
		if p.Logo == "" {
			t.Errorf("%s: enabled provider is missing logo", p.Name)
		}
		if p.AuthType == "none" && (p.ChatPath == "" || p.ModelsPath == "") {
			t.Errorf("%s: no-auth provider needs explicit chat_path and models_path", p.Name)
		}
	}
	if enabled < 3 {
		t.Fatalf("expected at least 3 connectable providers, got %d", enabled)
	}
	pollinations := GetFreeProviderByName("Pollinations")
	if pollinations == nil {
		t.Fatal("Pollinations missing")
	}
	if pollinations.BaseURL != "https://text.pollinations.ai" ||
		pollinations.ChatPath != "/openai" || pollinations.ModelsPath != "/models" {
		t.Fatalf("Pollinations must use the OpenAI-compatible /openai endpoint, got %+v", pollinations)
	}
	if !pollinations.Enabled || pollinations.AuthType != "none" {
		t.Fatalf("Pollinations should be a no-auth enabled provider, got %+v", pollinations)
	}
}

func TestFreeProviderFreeModelFlags(t *testing.T) {
	openrouter := GetFreeProviderByName("OpenRouter")
	if openrouter == nil || !openrouter.Enabled {
		t.Fatal("OpenRouter should be enabled")
	}
	if !openrouter.FreeOnly || !openrouter.SeedOnly {
		t.Fatalf("OpenRouter must be free-only and seed-only, got %+v", openrouter)
	}
	if openrouter.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("OpenRouter base = %s", openrouter.BaseURL)
	}

	google := GetFreeProviderByName("Google AI Studio")
	if google == nil || !google.Enabled || !google.SeedOnly || !google.NoModelFetch {
		t.Fatalf("Google AI Studio flags = %+v", google)
	}
	if google.ChatPath != "/chat/completions" {
		t.Fatalf("Google chat path = %s", google.ChatPath)
	}

	for _, name := range []string{"Groq", "Mistral", "SambaNova"} {
		p := GetFreeProviderByName(name)
		if p == nil || !p.Enabled || p.AuthType != "apikey" || p.SeedOnly {
			t.Errorf("%s = %+v, want enabled apikey with live sync", name, p)
		}
	}
}

func TestFreeProviderResearchCorrections(t *testing.T) {
	nous := GetFreeProviderByName("Nous Research (Portal)")
	if nous == nil || !nous.Enabled || !nous.FreeOnly || !nous.SeedOnly {
		t.Fatalf("Nous flags = %+v", nous)
	}
	if nous.BaseURL != "https://inference-api.nousresearch.com/v1" || len(nous.Models) != 6 {
		t.Fatalf("Nous endpoint/models = %+v", nous)
	}
	zai := GetFreeProviderByName("Z.ai (GLM)")
	if zai == nil || !zai.Enabled || !zai.FreeOnly || !zai.SeedOnly || !zai.NoModelFetch {
		t.Fatalf("Z.ai flags = %+v", zai)
	}
	if zai.BaseURL != "https://api.z.ai/api/paas/v4" || zai.ChatPath != "/chat/completions" {
		t.Fatalf("Z.ai endpoint = %s%s", zai.BaseURL, zai.ChatPath)
	}
	if cerebras := GetFreeProviderByName("Cerebras"); cerebras == nil || cerebras.Enabled {
		t.Fatalf("Cerebras should be disabled (trial-only), got %+v", cerebras)
	}
	if longcat := GetFreeProviderByName("LongCat (Airforce)"); longcat == nil || longcat.Enabled {
		t.Fatalf("LongCat should be disabled until free availability is verified, got %+v", longcat)
	}
}
