// Token Budgeting — per-key spend limits with auto-downgrade
// Track token usage per API key, enforce daily/monthly budgets,
// auto-downgrade to cheaper model when approaching limit
import { getDb, getSetting } from "./db/index.js";

// Ensure budget tables exist
function initBudgetTables() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS token_budgets (
      api_key TEXT PRIMARY KEY,
      daily_limit INTEGER DEFAULT 0,
      monthly_limit INTEGER DEFAULT 0,
      downgrade_at_percent INTEGER DEFAULT 80,
      downgrade_model TEXT DEFAULT 'minimax/minimax-m2.7',
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    CREATE TABLE IF NOT EXISTS token_usage_log (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      api_key TEXT NOT NULL,
      model TEXT NOT NULL,
      input_tokens INTEGER DEFAULT 0,
      output_tokens INTEGER DEFAULT 0,
      cost_estimate REAL DEFAULT 0,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    CREATE INDEX IF NOT EXISTS idx_usage_key_date ON token_usage_log(api_key, created_at);
  `);
}

let budgetInitialized = false;
function ensureBudgetInit() {
  if (!budgetInitialized) {
    initBudgetTables();
    budgetInitialized = true;
  }
}

// Get budget config for a key
export function getBudget(apiKey) {
  ensureBudgetInit();
  const db = getDb();
  return db.prepare("SELECT * FROM token_budgets WHERE api_key = ?").get(apiKey) || null;
}

// Set budget for a key
export function setBudget(apiKey, { dailyLimit, monthlyLimit, downgradeAtPercent, downgradeModel }) {
  ensureBudgetInit();
  const db = getDb();
  db.prepare(`
    INSERT OR REPLACE INTO token_budgets (api_key, daily_limit, monthly_limit, downgrade_at_percent, downgrade_model)
    VALUES (?, ?, ?, ?, ?)
  `).run(apiKey, dailyLimit || 0, monthlyLimit || 0, downgradeAtPercent || 80, downgradeModel || "minimax/minimax-m2.7");
}

// Get usage for today
export function getDailyUsage(apiKey) {
  ensureBudgetInit();
  const db = getDb();
  const row = db.prepare(`
    SELECT COALESCE(SUM(input_tokens + output_tokens), 0) as total_tokens,
           COALESCE(SUM(cost_estimate), 0) as total_cost
    FROM token_usage_log
    WHERE api_key = ? AND created_at >= date('now')
  `).get(apiKey);
  return row || { total_tokens: 0, total_cost: 0 };
}

// Get usage for this month
export function getMonthlyUsage(apiKey) {
  ensureBudgetInit();
  const db = getDb();
  const row = db.prepare(`
    SELECT COALESCE(SUM(input_tokens + output_tokens), 0) as total_tokens,
           COALESCE(SUM(cost_estimate), 0) as total_cost
    FROM token_usage_log
    WHERE api_key = ? AND created_at >= date('now', 'start of month')
  `).get(apiKey);
  return row || { total_tokens: 0, total_cost: 0 };
}

// Record token usage
export function recordUsage(apiKey, model, inputTokens, outputTokens) {
  ensureBudgetInit();
  const db = getDb();
  // Rough cost estimate (per 1M tokens)
  const costPer1M = getCostPerMillion(model);
  const totalTokens = (inputTokens || 0) + (outputTokens || 0);
  const cost = (totalTokens / 1000000) * costPer1M;

  db.prepare(`
    INSERT INTO token_usage_log (api_key, model, input_tokens, output_tokens, cost_estimate)
    VALUES (?, ?, ?, ?, ?)
  `).run(apiKey, model, inputTokens || 0, outputTokens || 0, cost);
}

// Check if budget allows request, return downgrade info if needed
export function checkBudget(apiKey) {
  ensureBudgetInit();
  const budget = getBudget(apiKey);
  if (!budget) return { allowed: true, downgrade: false };

  const daily = getDailyUsage(apiKey);
  const monthly = getMonthlyUsage(apiKey);

  const downgradePercent = budget.downgrade_at_percent / 100;

  // Check hard limits
  if (budget.daily_limit > 0 && daily.total_tokens >= budget.daily_limit) {
    return { allowed: false, reason: "daily_limit_exceeded", usage: daily.total_tokens, limit: budget.daily_limit };
  }
  if (budget.monthly_limit > 0 && monthly.total_tokens >= budget.monthly_limit) {
    return { allowed: false, reason: "monthly_limit_exceeded", usage: monthly.total_tokens, limit: budget.monthly_limit };
  }

  // Check downgrade thresholds
  if (budget.daily_limit > 0 && daily.total_tokens >= budget.daily_limit * downgradePercent) {
    return { allowed: true, downgrade: true, model: budget.downgrade_model, reason: "approaching_daily_limit", percent: Math.round((daily.total_tokens / budget.daily_limit) * 100) };
  }
  if (budget.monthly_limit > 0 && monthly.total_tokens >= budget.monthly_limit * downgradePercent) {
    return { allowed: true, downgrade: true, model: budget.downgrade_model, reason: "approaching_monthly_limit", percent: Math.round((monthly.total_tokens / budget.monthly_limit) * 100) };
  }

  return { allowed: true, downgrade: false };
}

// Cost table (rough estimates per 1M tokens, input+output average)
function getCostPerMillion(model) {
  const costs = {
    "deepseek/deepseek-v4-pro": 2.0,
    "minimax/minimax-m2.7": 0.5,
    "kimi/kimi-k2.6": 1.0,
    "glm/glm-4.7": 1.0,
    "qwen/qwen3-coder": 1.5,
    "gpt-4o": 10.0,
    "gpt-4o-mini": 0.6,
    "gpt-4.1": 8.0,
    "claude-sonnet-4-20250514": 9.0,
    "claude-haiku-35-20241022": 1.0,
    "deepseek-chat": 0.5,
    "deepseek-reasoner": 2.0,
  };
  return costs[model] || 2.0;
}

// Cleanup old usage logs (keep 90 days)
export function cleanupUsageLogs() {
  ensureBudgetInit();
  const db = getDb();
  db.prepare("DELETE FROM token_usage_log WHERE created_at < date('now', '-90 days')").run();
}
