package cost

import "math"

// SavingsCategory represents the source of cost savings.
type SavingsCategory string

const (
	SavingsCompression SavingsCategory = "compression" // Token compression reduced cost
	SavingsRouting     SavingsCategory = "routing"      // Smart routing chose cheaper model/provider
	SavingsCache       SavingsCategory = "cache"        // Cache hit avoided API call
	SavingsFree        SavingsCategory = "free"         // Free provider used
)

// Calculator computes costs and savings for LLM requests.
type Calculator struct {
	pricing *PricingLookup
}

// NewCalculator creates a calculator with default pricing.
func NewCalculator() *Calculator {
	return &Calculator{pricing: NewPricingLookup()}
}

// NewCalculatorWithPricing creates a calculator with custom pricing.
func NewCalculatorWithPricing(table []ModelPricing) *Calculator {
	return &Calculator{pricing: NewPricingLookupCustom(table)}
}

// RequestCost represents the cost breakdown for a single request.
type RequestCost struct {
	Model         string  `json:"model"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	InputCostUSD  float64 `json:"input_cost_usd"`
	OutputCostUSD float64 `json:"output_cost_usd"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
}

// CalculateCost computes the cost for a request with given token counts.
func (c *Calculator) CalculateCost(model string, inputTokens, outputTokens int) RequestCost {
	price, ok := c.pricing.Get(model)
	if !ok {
		return RequestCost{
			Model:        model,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		}
	}
	inCost := float64(inputTokens) / 1_000_000 * price.InputPrice
	outCost := float64(outputTokens) / 1_000_000 * price.OutputPrice
	return RequestCost{
		Model:         model,
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		InputCostUSD:  roundMoney(inCost),
		OutputCostUSD: roundMoney(outCost),
		TotalCostUSD:  roundMoney(inCost + outCost),
	}
}

// SavingsResult represents savings achieved on a request.
type SavingsResult struct {
	Category       SavingsCategory `json:"category"`
	OriginalCost   float64         `json:"original_cost_usd"`
	ActualCost     float64         `json:"actual_cost_usd"`
	SavingsUSD     float64         `json:"savings_usd"`
	SavingsPercent float64         `json:"savings_percent"`
	OriginalModel  string          `json:"original_model"`
	ActualModel    string          `json:"actual_model"`
}

// CalculateCompressionSavings calculates savings from token compression.
// originalTokens = original input, compressedTokens = compressed input, model = actual model used.
func (c *Calculator) CalculateCompressionSavings(model string, originalTokens, compressedTokens, outputTokens int) SavingsResult {
	origCost := c.CalculateCost(model, originalTokens, outputTokens)
	actualCost := c.CalculateCost(model, compressedTokens, outputTokens)
	savings := origCost.TotalCostUSD - actualCost.TotalCostUSD
	pct := 0.0
	if origCost.TotalCostUSD > 0 {
		pct = (savings / origCost.TotalCostUSD) * 100
	}
	return SavingsResult{
		Category:       SavingsCompression,
		OriginalCost:   origCost.TotalCostUSD,
		ActualCost:     actualCost.TotalCostUSD,
		SavingsUSD:     roundMoney(savings),
		SavingsPercent: roundPct(pct),
		OriginalModel:  model,
		ActualModel:    model,
	}
}

// CalculateRoutingSavings calculates savings from routing to a cheaper model.
func (c *Calculator) CalculateRoutingSavings(originalModel, actualModel string, inputTokens, outputTokens int) SavingsResult {
	origCost := c.CalculateCost(originalModel, inputTokens, outputTokens)
	actualCost := c.CalculateCost(actualModel, inputTokens, outputTokens)
	savings := origCost.TotalCostUSD - actualCost.TotalCostUSD
	pct := 0.0
	if origCost.TotalCostUSD > 0 {
		pct = (savings / origCost.TotalCostUSD) * 100
	}
	return SavingsResult{
		Category:       SavingsRouting,
		OriginalCost:   origCost.TotalCostUSD,
		ActualCost:     actualCost.TotalCostUSD,
		SavingsUSD:     roundMoney(savings),
		SavingsPercent: roundPct(pct),
		OriginalModel:  originalModel,
		ActualModel:    actualModel,
	}
}

// CalculateCacheSavings calculates savings from a cache hit (full cost avoided).
func (c *Calculator) CalculateCacheSavings(model string, inputTokens, outputTokens int) SavingsResult {
	fullCost := c.CalculateCost(model, inputTokens, outputTokens)
	return SavingsResult{
		Category:       SavingsCache,
		OriginalCost:   fullCost.TotalCostUSD,
		ActualCost:     0,
		SavingsUSD:     fullCost.TotalCostUSD,
		SavingsPercent: 100.0,
		OriginalModel:  model,
		ActualModel:    model,
	}
}

// CalculateFreeProviderSavings calculates savings from using a free provider.
func (c *Calculator) CalculateFreeProviderSavings(originalModel string, inputTokens, outputTokens int) SavingsResult {
	fullCost := c.CalculateCost(originalModel, inputTokens, outputTokens)
	return SavingsResult{
		Category:       SavingsFree,
		OriginalCost:   fullCost.TotalCostUSD,
		ActualCost:     0,
		SavingsUSD:     fullCost.TotalCostUSD,
		SavingsPercent: 100.0,
		OriginalModel:  originalModel,
		ActualModel:    "free",
	}
}

func roundMoney(v float64) float64 {
	return math.Round(v*1_000_000) / 1_000_000
}

func roundPct(v float64) float64 {
	return math.Round(v*100) / 100
}
