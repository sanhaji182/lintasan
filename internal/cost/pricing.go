package cost

// ModelPrice holds per-token pricing for a model (USD per 1M tokens).
type ModelPricing struct {
	Model       string  `json:"model"`
	Provider    string  `json:"provider"`
	InputPrice  float64 `json:"input_price_per_1m"`
	OutputPrice float64 `json:"output_price_per_1m"`
	ContextSize int     `json:"context_window"`
}

// DefaultPricingTable contains pricing for 15+ popular LLM models.
var DefaultPricingTable = []ModelPricing{
	// OpenAI
	{Model: "gpt-4o", Provider: "openai", InputPrice: 2.50, OutputPrice: 10.00, ContextSize: 128000},
	{Model: "gpt-4o-mini", Provider: "openai", InputPrice: 0.15, OutputPrice: 0.60, ContextSize: 128000},
	{Model: "gpt-4-turbo", Provider: "openai", InputPrice: 10.00, OutputPrice: 30.00, ContextSize: 128000},
	{Model: "o1", Provider: "openai", InputPrice: 15.00, OutputPrice: 60.00, ContextSize: 200000},
	{Model: "o1-mini", Provider: "openai", InputPrice: 3.00, OutputPrice: 12.00, ContextSize: 128000},
	// Anthropic
	{Model: "claude-sonnet-4-20250514", Provider: "anthropic", InputPrice: 3.00, OutputPrice: 15.00, ContextSize: 200000},
	{Model: "claude-opus-4-20250514", Provider: "anthropic", InputPrice: 15.00, OutputPrice: 75.00, ContextSize: 200000},
	{Model: "claude-haiku-3-5", Provider: "anthropic", InputPrice: 0.80, OutputPrice: 4.00, ContextSize: 200000},
	// DeepSeek
	{Model: "deepseek-v4-pro", Provider: "deepseek", InputPrice: 0.55, OutputPrice: 2.20, ContextSize: 128000},
	{Model: "deepseek-v3", Provider: "deepseek", InputPrice: 0.27, OutputPrice: 1.10, ContextSize: 128000},
	{Model: "deepseek-r1", Provider: "deepseek", InputPrice: 0.55, OutputPrice: 2.19, ContextSize: 128000},
	// Google
	{Model: "gemini-2.5-pro", Provider: "google", InputPrice: 1.25, OutputPrice: 10.00, ContextSize: 1000000},
	{Model: "gemini-2.5-flash", Provider: "google", InputPrice: 0.15, OutputPrice: 0.60, ContextSize: 1000000},
	// Meta (via API providers)
	{Model: "llama-3.3-70b", Provider: "meta", InputPrice: 0.35, OutputPrice: 0.40, ContextSize: 128000},
	{Model: "llama-3.1-405b", Provider: "meta", InputPrice: 1.00, OutputPrice: 1.00, ContextSize: 128000},
	// Mistral
	{Model: "mistral-large-2", Provider: "mistral", InputPrice: 2.00, OutputPrice: 6.00, ContextSize: 128000},
	{Model: "mistral-small", Provider: "mistral", InputPrice: 0.10, OutputPrice: 0.30, ContextSize: 32000},
	// Qwen
	{Model: "qwen-2.5-72b", Provider: "qwen", InputPrice: 0.30, OutputPrice: 0.60, ContextSize: 128000},
}

// PricingLookup provides fast model pricing lookups.
type PricingLookup struct {
	byModel map[string]ModelPricing
}

// NewPricingLookup builds a lookup table from DefaultPricingTable.
func NewPricingLookup() *PricingLookup {
	pl := &PricingLookup{
		byModel: make(map[string]ModelPricing, len(DefaultPricingTable)),
	}
	for _, p := range DefaultPricingTable {
		pl.byModel[p.Model] = p
	}
	return pl
}

// NewPricingLookupCustom builds a lookup from a custom table.
func NewPricingLookupCustom(table []ModelPricing) *PricingLookup {
	pl := &PricingLookup{
		byModel: make(map[string]ModelPricing, len(table)),
	}
	for _, p := range table {
		pl.byModel[p.Model] = p
	}
	return pl
}

// Get returns pricing for a model, or zero pricing if not found.
func (pl *PricingLookup) Get(model string) (ModelPricing, bool) {
	p, ok := pl.byModel[model]
	return p, ok
}

// All returns all model pricing entries.
func (pl *PricingLookup) All() []ModelPricing {
	out := make([]ModelPricing, 0, len(pl.byModel))
	for _, v := range pl.byModel {
		out = append(out, v)
	}
	return out
}

// CheapestForProvider returns the cheapest model for a given provider.
func (pl *PricingLookup) CheapestForProvider(provider string) (ModelPricing, bool) {
	var best ModelPricing
	found := false
	for _, p := range pl.byModel {
		if p.Provider == provider {
			totalPrice := p.InputPrice + p.OutputPrice
			if !found || totalPrice < (best.InputPrice+best.OutputPrice) {
				best = p
				found = true
			}
		}
	}
	return best, found
}
