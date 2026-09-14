package hoplite

// ModelCatalogRevision identifies the authoritative Hoplite web-app contract
// snapshot used by this fallback catalog. Exact IDs and metadata were verified
// in https://app.hoplite.sh/assets/src-CxivfBFj.js on 2026-09-14 and display
// names/plan wording against https://hoplite.sh/docs/agent/models.
// Hoplite currently exposes no authenticated model-discovery endpoint in its
// public API.
const ModelCatalogRevision = "app-contract-2026-09-14"

// Model describes an exact thread model ID verified in Hoplite's shipped web
// application model registry. Availability is catalog eligibility, not a claim
// that a particular account is entitled to use the model.
type Model struct {
	ID            string `json:"id"`
	DisplayName   string `json:"display_name"`
	Provider      string `json:"provider"`
	ContextTokens int    `json:"context_window_tokens"`
	Plan          string `json:"catalog_eligibility"`
}

var modelCatalog = []Model{
	{ID: "gpt-5.6-sol", DisplayName: "GPT-5.6 Sol", Provider: "OpenAI", ContextTokens: 200000, Plan: "pro"},
	{ID: "gpt-5.6-sol-1m", DisplayName: "GPT-5.6 Sol · 1M context", Provider: "OpenAI", ContextTokens: 1000000, Plan: "pro"},
	{ID: "gpt-5.6-terra", DisplayName: "GPT-5.6 Terra", Provider: "OpenAI", ContextTokens: 200000, Plan: "free+pro"},
	{ID: "gpt-5.6-terra-1m", DisplayName: "GPT-5.6 Terra · 1M context", Provider: "OpenAI", ContextTokens: 1000000, Plan: "pro"},
	{ID: "gpt-5.6-luna", DisplayName: "GPT-5.6 Luna", Provider: "OpenAI", ContextTokens: 200000, Plan: "free+pro"},
	{ID: "gpt-5.6-luna-1m", DisplayName: "GPT-5.6 Luna · 1M context", Provider: "OpenAI", ContextTokens: 1000000, Plan: "pro"},
	{ID: "gpt-5.5", DisplayName: "GPT-5.5", Provider: "OpenAI", ContextTokens: 200000, Plan: "pro"},
	{ID: "gpt-5.5-1m", DisplayName: "GPT-5.5 · 1M context", Provider: "OpenAI", ContextTokens: 1000000, Plan: "pro"},
	{ID: "gpt-5.3-codex", DisplayName: "GPT-5.3 Codex", Provider: "OpenAI", ContextTokens: 400000, Plan: "pro"},
	{ID: "claude-fable-5-1", DisplayName: "Fable 5.1", Provider: "Anthropic", ContextTokens: 1000000, Plan: "pro"},
	{ID: "claude-fable-5", DisplayName: "Fable 5", Provider: "Anthropic", ContextTokens: 200000, Plan: "pro"},
	{ID: "claude-opus-5", DisplayName: "Opus 5", Provider: "Anthropic", ContextTokens: 1000000, Plan: "pro"},
	{ID: "claude-opus-4-8", DisplayName: "Opus 4.8", Provider: "Anthropic", ContextTokens: 200000, Plan: "pro"},
	{ID: "claude-sonnet-5", DisplayName: "Sonnet 5", Provider: "Anthropic", ContextTokens: 1000000, Plan: "free+pro"},
	{ID: "claude-haiku-4-5", DisplayName: "Haiku 4.5", Provider: "Anthropic", ContextTokens: 200000, Plan: "pro"},
	{ID: "meta/muse-spark-1.3", DisplayName: "Muse Spark 1.3", Provider: "Meta via OpenRouter", ContextTokens: 1048576, Plan: "free-via-contributor+pro"},
	{ID: "meta/muse-spark-1.3-contributor", DisplayName: "Muse Spark 1.3 Contributor", Provider: "Meta via OpenRouter", ContextTokens: 1048576, Plan: "free-only"},
	{ID: "z-ai/glm-5.3", DisplayName: "GLM 5.3", Provider: "OpenRouter", ContextTokens: 1048576, Plan: "free+pro"},
	{ID: "z-ai/glm-5.3-flash", DisplayName: "GLM 5.3 Flash", Provider: "OpenRouter", ContextTokens: 1048576, Plan: "free+pro"},
	{ID: "deepseek/deepseek-v4-flash-0731", DisplayName: "DeepSeek V4 Flash 0731", Provider: "OpenRouter", ContextTokens: 1048576, Plan: "free+pro"},
}

func Models() []Model {
	out := make([]Model, len(modelCatalog))
	copy(out, modelCatalog)
	return out
}

func IsKnownModel(id string) bool {
	for _, model := range modelCatalog {
		if model.ID == id {
			return true
		}
	}
	return false
}
