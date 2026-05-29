// Cost Tracking - per-request cost estimation
import { getSetting, getDb } from "./db/index.js";

// Default pricing per 1M tokens (USD)
const DEFAULT_PRICING = {
  commandcode: { input: 0, output: 0 },
  openai: { input: 2.5, output: 10 },
  anthropic: { input: 3, output: 15 },
  deepseek: { input: 0.14, output: 0.28 },
  groq: { input: 0.05, output: 0.08 },
  openrouter: { input: 1, output: 3 },
  together: { input: 0.2, output: 0.6 },
  cache: { input: 0, output: 0 },
};

// Model-specific overrides
const MODEL_PRICING = {
  "gpt-4o": { input: 2.5, output: 10 },
  "gpt-4o-mini": { input: 0.15, output: 0.6 },
  "gpt-4.1": { input: 2, output: 8 },
  "o3-mini": { input: 1.1, output: 4.4 },
  "claude-sonnet-4-20250514": { input: 3, output: 15 },
  "claude-haiku-35-20241022": { input: 0.8, output: 4 },
  "claude-opus-4-20250514": { input: 15, output: 75 },
  "deepseek-chat": { input: 0.14, output: 0.28 },
  "deepseek-reasoner": { input: 0.55, output: 2.19 },
};

export function getCustomPricing() {
  try {
    const json = getSetting("custom_pricing", "{}");
    return JSON.parse(json);
  } catch {
    return {};
  }
}

export function setCustomPricing(pricing) {
  const db = getDb();
  db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("custom_pricing", JSON.stringify(pricing));
}

export function calculateCost(provider, model, inputTokens, outputTokens) {
  const customPricing = getCustomPricing();

  // Check custom pricing first (model-level)
  let pricing = customPricing[model] || MODEL_PRICING[model] || customPricing[provider] || DEFAULT_PRICING[provider] || { input: 1, output: 3 };

  const inputCost = (inputTokens / 1_000_000) * pricing.input;
  const outputCost = (outputTokens / 1_000_000) * pricing.output;

  return {
    input_cost: inputCost,
    output_cost: outputCost,
    total_cost: inputCost + outputCost,
    pricing_per_1m: pricing,
  };
}

// Get cost summary from logs
export function getCostSummary(days = 7) {
  const db = getDb();

  const rows = db.prepare(`
    SELECT provider, model, SUM(input_tokens) as input_tokens, SUM(output_tokens) as output_tokens, COUNT(*) as requests
    FROM request_logs
    WHERE created_at >= datetime('now', '-' || ? || ' days') AND provider != 'cache'
    GROUP BY provider, model
  `).all(days);

  let totalCost = 0;
  const breakdown = rows.map(row => {
    const cost = calculateCost(row.provider, row.model, row.input_tokens || 0, row.output_tokens || 0);
    totalCost += cost.total_cost;
    return { ...row, ...cost };
  });

  return { totalCost, breakdown };
}
