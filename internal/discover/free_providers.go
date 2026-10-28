package discover

// FreeProvider represents a free AI provider that can be auto-connected.
type FreeProvider struct {
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	BaseURL    string   `json:"base_url"`
	Models     []string `json:"models"`
	AuthType   string   `json:"auth_type"` // "none", "apikey", "oauth"
	QuotaInfo  string   `json:"quota_info"`
	Enabled    bool     `json:"enabled"`
	ChatPath   string   `json:"chat_path,omitempty"`
	ModelsPath string   `json:"models_path,omitempty"`
	// FreeOnly filters fetched model IDs to the provider's free catalogue.
	FreeOnly bool `json:"free_only,omitempty"`
	// SeedOnly skips post-connect /models sync so paid models are not imported.
	SeedOnly bool `json:"seed_only,omitempty"`
	// NoModelFetch marks providers whose OpenAI-compatible base has no /models.
	NoModelFetch bool   `json:"no_model_fetch,omitempty"`
	Logo         string `json:"logo,omitempty"`
	// SyncSource/SyncUpdatedAt are filled at request time by ApplySyncOverlay.
	SyncSource    string `json:"sync_source,omitempty"`
	SyncUpdatedAt string `json:"sync_updated_at,omitempty"`
}

// GetFreeProviders returns the registry of known free AI providers.
func GetFreeProviders() []FreeProvider {
	return []FreeProvider{
		{
			Name:       "Pollinations",
			Logo:       "/provider-logos/pollinations.svg",
			Prefix:     "pol/",
			BaseURL:    "https://text.pollinations.ai",
			Models:     []string{"openai", "openai-fast"},
			AuthType:   "none",
			QuotaInfo:  "No key needed; public endpoint, rate-limited",
			Enabled:    true,
			ChatPath:   "/openai",
			ModelsPath: "/models",
		},
		{
			Name:      "NVIDIA NIM",
			Logo:      "/provider-logos/nvidia.svg",
			Prefix:    "nvidia/",
			BaseURL:   "https://integrate.api.nvidia.com/v1",
			Models:    []string{"deepseek-ai/deepseek-v4.1-flash", "meta/llama-3.2-90b-vision-instruct", "nvidia/llama-3.1-nemotron-70b-instruct"},
			AuthType:  "apikey",
			QuotaInfo: "Free API catalog; phone verification required; ~40 RPM",
			Enabled:   true,
		},
		{
			Name:      "Cerebras",
			Logo:      "/provider-logos/cerebras.svg",
			Prefix:    "cerebras/",
			BaseURL:   "https://api.cerebras.ai/v1",
			Models:    []string{"llama3.1-8b", "llama-3.3-70b", "qwen-3-235b-a22b-instruct-2507", "gpt-oss-120b"},
			AuthType:  "apikey",
			QuotaInfo: "Trial only: $5 credit / 30 days, card required",
			Enabled:   false,
		},
		{
			Name:      "LongCat (Airforce)",
			Logo:      "/provider-logos/longcat.svg",
			Prefix:    "lc/",
			BaseURL:   "https://api.airforce/v1",
			Models:    []string{"glm-5.3", "kimi-k2.7-code", "gpt-oss-20b"},
			AuthType:  "apikey",
			QuotaInfo: "API key required; free availability unverified",
			Enabled:   false,
		},
		{
			Name:      "Groq",
			Logo:      "/provider-logos/groq.svg",
			Prefix:    "groq/",
			BaseURL:   "https://api.groq.com/openai/v1",
			Models:    []string{"llama-3.3-70b-versatile", "llama-3.1-8b-instant", "openai/gpt-oss-120b", "openai/gpt-oss-20b"},
			AuthType:  "apikey",
			QuotaInfo: "Free: ~30 RPM / 1,000 RPD per model; phone verification at signup",
			Enabled:   true,
		},
		{
			Name:      "Mistral",
			Logo:      "/provider-logos/mistral.svg",
			Prefix:    "mistral/",
			BaseURL:   "https://api.mistral.ai/v1",
			Models:    []string{"mistral-small-latest", "open-mistral-nemo", "codestral-latest", "ministral-8b-latest"},
			AuthType:  "apikey",
			QuotaInfo: "Free mode with data-training opt-in; limits vary by account",
			Enabled:   true,
		},
		{
			Name:      "SambaNova",
			Logo:      "/provider-logos/sambanova.svg",
			Prefix:    "samba/",
			BaseURL:   "https://api.sambanova.ai/v1",
			Models:    []string{"DeepSeek-V3.2", "Meta-Llama-3.3-70B-Instruct", "gpt-oss-120b", "MiniMax-M3.1-Flash"},
			AuthType:  "apikey",
			QuotaInfo: "Free tier is very limited (documented ~20 requests/day); card raises limits",
			Enabled:   true,
		},
		{
			Name:         "Google AI Studio",
			Logo:         "/provider-logos/google.svg",
			Prefix:       "google/",
			BaseURL:      "https://generativelanguage.googleapis.com/v1beta/openai",
			Models:       []string{"gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-2.0-flash"},
			AuthType:     "apikey",
			QuotaInfo:    "Free tier; limits in AI Studio; free prompts may be used for training",
			Enabled:      true,
			ChatPath:     "/chat/completions",
			ModelsPath:   "/models",
			SeedOnly:     true,
			NoModelFetch: true,
		},
		{
			Name:    "Nous Research (Portal)",
			Logo:    "/provider-logos/nous.png",
			Prefix:  "nous/",
			BaseURL: "https://inference-api.nousresearch.com/v1",
			Models: []string{
				"poolside/laguna-s-2.1:free",
				"poolside/laguna-xs-2.1:free",
				"upstage/solar-mini4:free",
				"stepfun/step-3.7-flash:free",
				"reflection/beam-1.0-501b-a23b:free",
				"ornith/ornith-1.5-122b:free",
			},
			AuthType:  "apikey",
			QuotaInfo: "Free tier with 27+ models; :free variants are rate-limited",
			Enabled:   true,
			FreeOnly:  true,
			SeedOnly:  true,
		},
		{
			Name:         "Z.ai (GLM)",
			Logo:         "/provider-logos/zai.svg",
			Prefix:       "zai/",
			BaseURL:      "https://api.z.ai/api/paas/v4",
			Models:       []string{"glm-4.7-flash", "glm-4.5-flash", "glm-4.6v-flash"},
			AuthType:     "apikey",
			QuotaInfo:    "Permanent free GLM Flash models; 1 concurrent request",
			Enabled:      true,
			ChatPath:     "/chat/completions",
			ModelsPath:   "/models",
			FreeOnly:     true,
			SeedOnly:     true,
			NoModelFetch: true,
		},
		{
			Name:    "OpenRouter",
			Logo:    "/provider-logos/openrouter.svg",
			Prefix:  "or/",
			BaseURL: "https://openrouter.ai/api/v1",
			Models: []string{
				"google/gemma-4-31b-it:free",
				"nvidia/nemotron-3.5-super-vl:free",
				"thinkingmachines/inkling:free",
				"cohere/north-mini-code:free",
				"poolside/laguna-s-2.1:free",
			},
			AuthType:  "apikey",
			QuotaInfo: "Only :free models; ~20 RPM / 50 RPD (1,000/day after $10 top-up)",
			Enabled:   true,
			FreeOnly:  true,
			SeedOnly:  true,
		},
		{
			// Requires an OpenCode Go credential plus the proprietary
			// x-opencode-session header, so it cannot be one-click wired.
			Name:      "OpenCode Free",
			Logo:      "/provider-logos/opencode.svg",
			Prefix:    "oc/",
			BaseURL:   "https://opencode.ai/zen/go/v1",
			Models:    []string{"deepseek-v4.1-flash", "glm-5.3", "kimi-k2.7-code"},
			AuthType:  "apikey",
			QuotaInfo: "Requires an OpenCode Go credential",
			Enabled:   false,
		},
		{
			// Kiro speaks AWS CodeWhisperer (SigV4), not OpenAI chat completions.
			Name:      "Kiro AI",
			Logo:      "/provider-logos/kiro.svg",
			Prefix:    "kr/",
			BaseURL:   "https://runtime.us-east-1.kiro.dev/v1",
			Models:    []string{"claude-sonnet-4.5", "claude-haiku-4.5"},
			AuthType:  "apikey",
			QuotaInfo: "AWS CodeWhisperer free tier",
			Enabled:   false,
		},
		{
			// Cloudflare's OpenAI-compatible route is account-scoped
			// (/accounts/{id}/ai/v1), so a shared one-click URL cannot work.
			Name:      "Cloudflare AI",
			Logo:      "/provider-logos/cloudflare.svg",
			Prefix:    "cf/",
			BaseURL:   "https://api.cloudflare.com/client/v4/ai/v1",
			Models:    []string{"llama-3.3-70b", "qwen-2.5-32b", "mistral-7b"},
			AuthType:  "apikey",
			QuotaInfo: "10K neurons/day, account-scoped endpoint",
			Enabled:   false,
		},
	}
}

// GetFreeProviderByPrefix looks up a free provider by its model prefix.
func GetFreeProviderByPrefix(prefix string) *FreeProvider {
	for _, p := range GetFreeProviders() {
		if p.Prefix == prefix {
			return &p
		}
	}
	return nil
}

// GetFreeProviderByName looks up a free provider by name.
func GetFreeProviderByName(name string) *FreeProvider {
	for _, p := range GetFreeProviders() {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

// GetAllFreeModels returns all models from all free providers.
func GetAllFreeModels() map[string][]string {
	result := make(map[string][]string)
	for _, p := range GetFreeProviders() {
		result[p.Prefix] = p.Models
	}
	return result
}
