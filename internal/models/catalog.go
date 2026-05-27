package models

// ProviderInfo describes an LLM provider and its available models.
type ProviderInfo struct {
	ID         string
	Name       string
	BaseURL    string
	Format     string // "openai", "anthropic", "gemini", "commandcode"
	AuthHeader string
	AuthPrefix string
	Models     []ModelInfo
}

// ModelInfo describes a single model with pricing and capabilities.
type ModelInfo struct {
	ID            string
	Name          string
	ContextWindow int
	MaxTokens     int
	InputPrice    float64 // per 1M tokens
	OutputPrice   float64 // per 1M tokens
	Capabilities  []string // "chat", "vision", "tools", "streaming", "json_mode"
}

// Catalog returns a complete hardcoded registry of 73+ models across all providers.
func Catalog() []ProviderInfo {
	return []ProviderInfo{
		{
			ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com",
			Format: "openai", AuthHeader: "Authorization", AuthPrefix: "Bearer ",
			Models: []ModelInfo{
				{ID: "gpt-4o", Name: "GPT-4o", ContextWindow: 128000, MaxTokens: 16384, InputPrice: 2.50, OutputPrice: 10.00, Capabilities: []string{"chat", "vision", "tools", "streaming", "json_mode"}},
				{ID: "gpt-4o-2024-08-06", Name: "GPT-4o (Aug 2024)", ContextWindow: 128000, MaxTokens: 16384, InputPrice: 2.50, OutputPrice: 10.00, Capabilities: []string{"chat", "vision", "tools", "streaming", "json_mode"}},
				{ID: "gpt-4o-2024-05-13", Name: "GPT-4o (May 2024)", ContextWindow: 128000, MaxTokens: 4096, InputPrice: 5.00, OutputPrice: 15.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "gpt-4o-mini", Name: "GPT-4o Mini", ContextWindow: 128000, MaxTokens: 16384, InputPrice: 0.15, OutputPrice: 0.60, Capabilities: []string{"chat", "vision", "tools", "streaming", "json_mode"}},
				{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", ContextWindow: 128000, MaxTokens: 4096, InputPrice: 10.00, OutputPrice: 30.00, Capabilities: []string{"chat", "vision", "tools", "streaming", "json_mode"}},
				{ID: "gpt-4", Name: "GPT-4", ContextWindow: 8192, MaxTokens: 8192, InputPrice: 30.00, OutputPrice: 60.00, Capabilities: []string{"chat", "streaming"}},
				{ID: "gpt-4-0613", Name: "GPT-4 (Jun 2023)", ContextWindow: 8192, MaxTokens: 8192, InputPrice: 30.00, OutputPrice: 60.00, Capabilities: []string{"chat", "streaming"}},
				{ID: "gpt-3.5-turbo", Name: "GPT-3.5 Turbo", ContextWindow: 16385, MaxTokens: 4096, InputPrice: 0.50, OutputPrice: 1.50, Capabilities: []string{"chat", "streaming", "json_mode"}},
				{ID: "gpt-3.5-turbo-0125", Name: "GPT-3.5 Turbo (Jan 2025)", ContextWindow: 16385, MaxTokens: 4096, InputPrice: 0.50, OutputPrice: 1.50, Capabilities: []string{"chat", "streaming", "json_mode"}},
				{ID: "o3-mini", Name: "o3-mini", ContextWindow: 200000, MaxTokens: 100000, InputPrice: 1.10, OutputPrice: 4.40, Capabilities: []string{"chat", "streaming"}},
				{ID: "o1", Name: "o1", ContextWindow: 200000, MaxTokens: 100000, InputPrice: 15.00, OutputPrice: 60.00, Capabilities: []string{"chat", "vision"}},
				{ID: "o1-mini", Name: "o1-mini", ContextWindow: 128000, MaxTokens: 65536, InputPrice: 1.10, OutputPrice: 4.40, Capabilities: []string{"chat"}},
				{ID: "o1-pro", Name: "o1-pro", ContextWindow: 200000, MaxTokens: 100000, InputPrice: 150.00, OutputPrice: 600.00, Capabilities: []string{"chat", "vision"}},
			},
		},
		{
			ID: "anthropic", Name: "Anthropic", BaseURL: "https://api.anthropic.com",
			Format: "anthropic", AuthHeader: "x-api-key", AuthPrefix: "",
			Models: []ModelInfo{
				{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", ContextWindow: 200000, MaxTokens: 8192, InputPrice: 3.00, OutputPrice: 15.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", ContextWindow: 200000, MaxTokens: 8192, InputPrice: 15.00, OutputPrice: 75.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "claude-sonnet-4", Name: "Claude Sonnet 4 (latest)", ContextWindow: 200000, MaxTokens: 8192, InputPrice: 3.00, OutputPrice: 15.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "claude-opus-4", Name: "Claude Opus 4 (latest)", ContextWindow: 200000, MaxTokens: 8192, InputPrice: 15.00, OutputPrice: 75.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "claude-haiku-3-5", Name: "Claude 3.5 Haiku", ContextWindow: 200000, MaxTokens: 8192, InputPrice: 0.80, OutputPrice: 4.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "claude-3-5-sonnet", Name: "Claude 3.5 Sonnet", ContextWindow: 200000, MaxTokens: 8192, InputPrice: 3.00, OutputPrice: 15.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "claude-3-opus", Name: "Claude 3 Opus", ContextWindow: 200000, MaxTokens: 4096, InputPrice: 15.00, OutputPrice: 75.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
			},
		},
		{
			ID: "google", Name: "Google Gemini", BaseURL: "https://generativelanguage.googleapis.com",
			Format: "gemini", AuthHeader: "x-goog-api-key", AuthPrefix: "",
			Models: []ModelInfo{
				{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", ContextWindow: 1048576, MaxTokens: 8192, InputPrice: 1.25, OutputPrice: 10.00, Capabilities: []string{"chat", "vision", "tools", "streaming", "json_mode"}},
				{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", ContextWindow: 1048576, MaxTokens: 8192, InputPrice: 0.15, OutputPrice: 0.60, Capabilities: []string{"chat", "vision", "tools", "streaming", "json_mode"}},
				{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", ContextWindow: 1048576, MaxTokens: 8192, InputPrice: 0.15, OutputPrice: 0.60, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro", ContextWindow: 2097152, MaxTokens: 8192, InputPrice: 1.25, OutputPrice: 5.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "gemini-1.5-flash", Name: "Gemini 1.5 Flash", ContextWindow: 1048576, MaxTokens: 8192, InputPrice: 0.075, OutputPrice: 0.30, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
			},
		},
		{
			ID: "deepseek", Name: "DeepSeek", BaseURL: "https://api.deepseek.com",
			Format: "openai", AuthHeader: "Authorization", AuthPrefix: "Bearer ",
			Models: []ModelInfo{
				{ID: "deepseek-v3", Name: "DeepSeek V3", ContextWindow: 128000, MaxTokens: 8192, InputPrice: 0.27, OutputPrice: 1.10, Capabilities: []string{"chat", "tools", "streaming", "json_mode"}},
				{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro", ContextWindow: 200000, MaxTokens: 16384, InputPrice: 0.55, OutputPrice: 2.20, Capabilities: []string{"chat", "vision", "tools", "streaming", "json_mode"}},
				{ID: "deepseek-r1", Name: "DeepSeek R1", ContextWindow: 128000, MaxTokens: 8192, InputPrice: 0.55, OutputPrice: 2.20, Capabilities: []string{"chat", "streaming"}},
				{ID: "deepseek-coder", Name: "DeepSeek Coder", ContextWindow: 128000, MaxTokens: 8192, InputPrice: 0.14, OutputPrice: 0.28, Capabilities: []string{"chat", "tools", "streaming"}},
			},
		},
		{
			ID: "meta", Name: "Meta", BaseURL: "https://api.llama-api.com",
			Format: "openai", AuthHeader: "Authorization", AuthPrefix: "Bearer ",
			Models: []ModelInfo{
				{ID: "llama-3.3-70b", Name: "Llama 3.3 70B", ContextWindow: 128000, MaxTokens: 8192, InputPrice: 0.59, OutputPrice: 0.79, Capabilities: []string{"chat", "tools", "streaming", "json_mode"}},
				{ID: "llama-3.3-70b-instruct", Name: "Llama 3.3 70B Instruct", ContextWindow: 128000, MaxTokens: 8192, InputPrice: 0.59, OutputPrice: 0.79, Capabilities: []string{"chat", "tools", "streaming"}},
				{ID: "llama-3.1-405b", Name: "Llama 3.1 405B", ContextWindow: 128000, MaxTokens: 8192, InputPrice: 2.00, OutputPrice: 6.00, Capabilities: []string{"chat", "tools", "streaming"}},
				{ID: "llama-3.1-70b", Name: "Llama 3.1 70B", ContextWindow: 128000, MaxTokens: 8192, InputPrice: 0.59, OutputPrice: 0.79, Capabilities: []string{"chat", "tools", "streaming", "json_mode"}},
				{ID: "llama-3.1-8b", Name: "Llama 3.1 8B", ContextWindow: 128000, MaxTokens: 8192, InputPrice: 0.06, OutputPrice: 0.08, Capabilities: []string{"chat", "tools", "streaming"}},
				{ID: "llama-3.2-1b", Name: "Llama 3.2 1B", ContextWindow: 128000, MaxTokens: 4096, InputPrice: 0.01, OutputPrice: 0.01, Capabilities: []string{"chat", "streaming"}},
			},
		},
		{
			ID: "mistral", Name: "Mistral AI", BaseURL: "https://api.mistral.ai",
			Format: "openai", AuthHeader: "Authorization", AuthPrefix: "Bearer ",
			Models: []ModelInfo{
				{ID: "mistral-large", Name: "Mistral Large", ContextWindow: 128000, MaxTokens: 4096, InputPrice: 2.00, OutputPrice: 6.00, Capabilities: []string{"chat", "vision", "tools", "streaming", "json_mode"}},
				{ID: "mistral-medium", Name: "Mistral Medium", ContextWindow: 32000, MaxTokens: 4096, InputPrice: 0.90, OutputPrice: 0.90, Capabilities: []string{"chat", "streaming"}},
				{ID: "mistral-small", Name: "Mistral Small", ContextWindow: 32000, MaxTokens: 4096, InputPrice: 0.20, OutputPrice: 0.60, Capabilities: []string{"chat", "streaming", "json_mode"}},
				{ID: "codestral", Name: "Codestral", ContextWindow: 256000, MaxTokens: 8192, InputPrice: 0.30, OutputPrice: 0.90, Capabilities: []string{"chat", "tools", "streaming"}},
				{ID: "mixtral-8x22b", Name: "Mixtral 8x22B", ContextWindow: 65536, MaxTokens: 4096, InputPrice: 0.90, OutputPrice: 0.90, Capabilities: []string{"chat", "tools", "streaming", "json_mode"}},
			},
		},
		{
			ID: "qwen", Name: "Qwen (Alibaba)", BaseURL: "https://dashscope.aliyuncs.com",
			Format: "openai", AuthHeader: "Authorization", AuthPrefix: "Bearer ",
			Models: []ModelInfo{
				{ID: "qwen-2.5-72b", Name: "Qwen 2.5 72B", ContextWindow: 131072, MaxTokens: 8192, InputPrice: 0.90, OutputPrice: 2.70, Capabilities: []string{"chat", "tools", "streaming", "json_mode"}},
				{ID: "qwen-2.5-32b", Name: "Qwen 2.5 32B", ContextWindow: 131072, MaxTokens: 8192, InputPrice: 0.40, OutputPrice: 1.20, Capabilities: []string{"chat", "tools", "streaming", "json_mode"}},
				{ID: "qwen-2.5-14b", Name: "Qwen 2.5 14B", ContextWindow: 131072, MaxTokens: 8192, InputPrice: 0.20, OutputPrice: 0.60, Capabilities: []string{"chat", "streaming"}},
				{ID: "qwen-2.5-7b", Name: "Qwen 2.5 7B", ContextWindow: 131072, MaxTokens: 8192, InputPrice: 0.10, OutputPrice: 0.30, Capabilities: []string{"chat", "streaming"}},
				{ID: "qwen-coder-32b", Name: "Qwen Coder 32B", ContextWindow: 131072, MaxTokens: 8192, InputPrice: 0.40, OutputPrice: 1.20, Capabilities: []string{"chat", "tools", "streaming"}},
				{ID: "qwen-coder-7b", Name: "Qwen Coder 7B", ContextWindow: 131072, MaxTokens: 8192, InputPrice: 0.10, OutputPrice: 0.30, Capabilities: []string{"chat", "tools", "streaming"}},
			},
		},
		{
			ID: "xai", Name: "xAI", BaseURL: "https://api.x.ai",
			Format: "openai", AuthHeader: "Authorization", AuthPrefix: "Bearer ",
			Models: []ModelInfo{
				{ID: "grok-3", Name: "Grok 3", ContextWindow: 131072, MaxTokens: 4096, InputPrice: 3.00, OutputPrice: 15.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "grok-3-mini", Name: "Grok 3 Mini", ContextWindow: 131072, MaxTokens: 4096, InputPrice: 0.30, OutputPrice: 0.50, Capabilities: []string{"chat", "streaming"}},
				{ID: "grok-2", Name: "Grok 2", ContextWindow: 131072, MaxTokens: 4096, InputPrice: 2.00, OutputPrice: 10.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
			},
		},
		{
			ID: "cohere", Name: "Cohere", BaseURL: "https://api.cohere.ai",
			Format: "openai", AuthHeader: "Authorization", AuthPrefix: "Bearer ",
			Models: []ModelInfo{
				{ID: "command-r-plus", Name: "Command R+", ContextWindow: 128000, MaxTokens: 4096, InputPrice: 2.50, OutputPrice: 10.00, Capabilities: []string{"chat", "tools", "streaming"}},
				{ID: "command-r", Name: "Command R", ContextWindow: 128000, MaxTokens: 4096, InputPrice: 0.50, OutputPrice: 1.50, Capabilities: []string{"chat", "tools", "streaming"}},
				{ID: "command", Name: "Command", ContextWindow: 4096, MaxTokens: 4096, InputPrice: 0.50, OutputPrice: 1.50, Capabilities: []string{"chat", "streaming"}},
			},
		},
		{
			ID: "ai21", Name: "AI21 Labs", BaseURL: "https://api.ai21.com",
			Format: "openai", AuthHeader: "Authorization", AuthPrefix: "Bearer ",
			Models: []ModelInfo{
				{ID: "jamba-1.5-large", Name: "Jamba 1.5 Large", ContextWindow: 256000, MaxTokens: 4096, InputPrice: 2.00, OutputPrice: 8.00, Capabilities: []string{"chat", "tools", "streaming", "json_mode"}},
				{ID: "jamba-1.5-mini", Name: "Jamba 1.5 Mini", ContextWindow: 256000, MaxTokens: 4096, InputPrice: 0.20, OutputPrice: 0.40, Capabilities: []string{"chat", "tools", "streaming", "json_mode"}},
			},
		},
		{
			ID: "reka", Name: "Reka AI", BaseURL: "https://api.reka.ai",
			Format: "openai", AuthHeader: "Authorization", AuthPrefix: "Bearer ",
			Models: []ModelInfo{
				{ID: "reka-core", Name: "Reka Core", ContextWindow: 128000, MaxTokens: 4096, InputPrice: 3.00, OutputPrice: 15.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "reka-flash", Name: "Reka Flash", ContextWindow: 128000, MaxTokens: 4096, InputPrice: 0.20, OutputPrice: 0.80, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "reka-edge", Name: "Reka Edge", ContextWindow: 128000, MaxTokens: 4096, InputPrice: 0.10, OutputPrice: 0.40, Capabilities: []string{"chat", "streaming"}},
			},
		},
		{
			ID: "perplexity", Name: "Perplexity", BaseURL: "https://api.perplexity.ai",
			Format: "openai", AuthHeader: "Authorization", AuthPrefix: "Bearer ",
			Models: []ModelInfo{
				{ID: "sonar-pro", Name: "Sonar Pro", ContextWindow: 200000, MaxTokens: 4096, InputPrice: 3.00, OutputPrice: 15.00, Capabilities: []string{"chat", "tools", "streaming"}},
				{ID: "sonar", Name: "Sonar", ContextWindow: 128000, MaxTokens: 4096, InputPrice: 1.00, OutputPrice: 5.00, Capabilities: []string{"chat", "streaming"}},
			},
		},
		{
			ID: "commandcode", Name: "CommandCode Alpha", BaseURL: "https://api.commandcode.ai",
			Format: "commandcode", AuthHeader: "Authorization", AuthPrefix: "Bearer ",
			Models: []ModelInfo{
				{ID: "cc-claude-sonnet-4", Name: "Claude Sonnet 4 (CC)", ContextWindow: 200000, MaxTokens: 8192, InputPrice: 3.00, OutputPrice: 15.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "cc-claude-opus-4", Name: "Claude Opus 4 (CC)", ContextWindow: 200000, MaxTokens: 8192, InputPrice: 15.00, OutputPrice: 75.00, Capabilities: []string{"chat", "vision", "tools", "streaming"}},
				{ID: "cc-deepseek-v4-pro", Name: "DeepSeek V4 Pro (CC)", ContextWindow: 200000, MaxTokens: 16384, InputPrice: 0.55, OutputPrice: 2.20, Capabilities: []string{"chat", "vision", "tools", "streaming", "json_mode"}},
				{ID: "cc-deepseek-v3", Name: "DeepSeek V3 (CC)", ContextWindow: 128000, MaxTokens: 8192, InputPrice: 0.27, OutputPrice: 1.10, Capabilities: []string{"chat", "tools", "streaming", "json_mode"}},
				{ID: "cc-gpt-4o", Name: "GPT-4o (CC)", ContextWindow: 128000, MaxTokens: 16384, InputPrice: 2.50, OutputPrice: 10.00, Capabilities: []string{"chat", "vision", "tools", "streaming", "json_mode"}},
				{ID: "cc-gemini-2.5-pro", Name: "Gemini 2.5 Pro (CC)", ContextWindow: 1048576, MaxTokens: 8192, InputPrice: 1.25, OutputPrice: 10.00, Capabilities: []string{"chat", "vision", "tools", "streaming", "json_mode"}},
				{ID: "cc-qwen-2.5-72b", Name: "Qwen 2.5 72B (CC)", ContextWindow: 131072, MaxTokens: 8192, InputPrice: 0.90, OutputPrice: 2.70, Capabilities: []string{"chat", "tools", "streaming", "json_mode"}},
				{ID: "cc-llama-3.3-70b", Name: "Llama 3.3 70B (CC)", ContextWindow: 128000, MaxTokens: 8192, InputPrice: 0.59, OutputPrice: 0.79, Capabilities: []string{"chat", "tools", "streaming", "json_mode"}},
			},
		},
	}
}

// AllModels returns a flat list of all models from the catalog.
func AllModels() []ModelInfo {
	var all []ModelInfo
	for _, p := range Catalog() {
		all = append(all, p.Models...)
	}
	return all
}

// TotalModelCount returns the total number of models in the catalog.
func TotalModelCount() int {
	count := 0
	for _, p := range Catalog() {
		count += len(p.Models)
	}
	return count
}

// AllProviders returns all provider info entries from the catalog.
func AllProviders() []ProviderInfo {
	return Catalog()
}

// FindProvider returns the ProviderInfo for a given provider ID, or nil if not found.
func FindProvider(id string) *ProviderInfo {
	for _, p := range Catalog() {
		if p.ID == id {
			return &p
		}
	}
	return nil
}

// FindModel returns the ModelInfo for a given model ID, or nil if not found.
// It searches across all providers.
func FindModel(id string) *ModelInfo {
	for _, p := range Catalog() {
		for _, m := range p.Models {
			if m.ID == id {
				return &m
			}
		}
	}
	return nil
}
