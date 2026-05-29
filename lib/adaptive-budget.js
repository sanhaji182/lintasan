// Adaptive Token Budget — learns optimal max_tokens per task type from history
// Replaces hardcoded smartMaxTokens with data-driven approach
import { getDb, getSetting } from "./db/index.js";

// Task categories for learning
const TASK_CATEGORIES = ["simple", "medium", "complex", "coding", "analytical"];

// Classify task complexity (similar to smart-tokens but more granular)
function classifyTask(messages) {
  if (!messages || messages.length === 0) return "simple";

  const lastUser = [...messages].reverse().find((m) => m.role === "user");
  if (!lastUser) return "simple";

  const content =
    typeof lastUser.content === "string"
      ? lastUser.content
      : Array.isArray(lastUser.content)
        ? lastUser.content.filter((p) => p.type === "text").map((p) => p.text).join(" ")
        : "";

  const wordCount = content.split(/\s+/).length;
  const hasCode = /```/.test(content) || /\b(function|class|import|const|let|var)\b/.test(content);
  const hasAnalysis = /\b(analyze|compare|explain|trace|identify|diagnose|why|how)\b/i.test(content);
  const isMultiStep = /\b(step|phase|first|then|finally|1\.|2\.|3\.)\b/i.test(content);

  if (hasCode && (wordCount > 200 || isMultiStep)) return "coding";
  if (hasAnalysis && wordCount > 100) return "analytical";
  if (wordCount > 300 || isMultiStep) return "complex";
  if (wordCount > 50) return "medium";
  return "simple";
}

// Initialize the adaptive budget table
function ensureTable() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS adaptive_token_budget (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      task_category TEXT NOT NULL,
      model TEXT NOT NULL,
      requested_max INTEGER NOT NULL,
      actual_used INTEGER NOT NULL,
      finish_reason TEXT,
      quality_score REAL DEFAULT 1.0,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    CREATE INDEX IF NOT EXISTS idx_atb_category ON adaptive_token_budget(task_category, model);
  `);
}

// Record actual token usage for learning
export function recordTokenUsage(taskCategory, model, requestedMax, actualUsed, finishReason) {
  const enabled = getSetting("adaptive_budget_enabled", "true") === "true";
  if (!enabled) return;

  try {
    ensureTable();
    const db = getDb();

    // Quality score: 1.0 = good (stop), 0.5 = truncated (length), 0.0 = error
    let qualityScore = 1.0;
    if (finishReason === "length") qualityScore = 0.5;
    if (!finishReason || finishReason === "error") qualityScore = 0.0;

    db.prepare(`
      INSERT INTO adaptive_token_budget (task_category, model, requested_max, actual_used, finish_reason, quality_score)
      VALUES (?, ?, ?, ?, ?, ?)
    `).run(taskCategory, model, requestedMax, actualUsed, finishReason || "unknown", qualityScore);

    // Keep only last 500 entries per category to prevent bloat
    db.prepare(`
      DELETE FROM adaptive_token_budget 
      WHERE id NOT IN (
        SELECT id FROM adaptive_token_budget 
        WHERE task_category = ? 
        ORDER BY created_at DESC LIMIT 500
      ) AND task_category = ?
    `).run(taskCategory, taskCategory);
  } catch (e) {
    // Silent fail — don't break request flow
  }
}

// Get optimal max_tokens based on historical data
export function getAdaptiveBudget(messages, model, fallbackMax) {
  const enabled = getSetting("adaptive_budget_enabled", "true") === "true";
  if (!enabled) return { maxTokens: fallbackMax, category: "unknown", source: "fallback" };

  const category = classifyTask(messages);

  try {
    ensureTable();
    const db = getDb();

    // Get stats for this category + model
    const stats = db.prepare(`
      SELECT 
        COUNT(*) as samples,
        AVG(actual_used) as avg_used,
        MAX(actual_used) as max_used,
        AVG(CASE WHEN finish_reason = 'length' THEN 1.0 ELSE 0.0 END) as truncation_rate,
        AVG(quality_score) as avg_quality
      FROM adaptive_token_budget
      WHERE task_category = ? AND model = ?
      AND created_at > datetime('now', '-7 days')
    `).get(category, model);

    // Need at least 3 samples to make a decision
    if (!stats || stats.samples < 3) {
      return { maxTokens: fallbackMax, category, source: "insufficient_data", samples: stats?.samples || 0 };
    }

    // Calculate optimal budget:
    // - Base: P90 of actual usage (covers 90% of cases)
    // - If truncation rate > 20%, increase by 50%
    // - If truncation rate < 5%, decrease by 20% (save tokens)
    // - Never go below 256 or above 16384

    const p90 = db.prepare(`
      SELECT actual_used FROM adaptive_token_budget
      WHERE task_category = ? AND model = ?
      AND created_at > datetime('now', '-7 days')
      ORDER BY actual_used ASC
      LIMIT 1 OFFSET ?
    `).get(category, model, Math.floor(stats.samples * 0.9));

    let optimal = p90 ? p90.actual_used : stats.avg_used;

    // Add headroom based on truncation rate
    if (stats.truncation_rate > 0.2) {
      optimal = Math.ceil(optimal * 1.5); // High truncation → more room
    } else if (stats.truncation_rate > 0.1) {
      optimal = Math.ceil(optimal * 1.3); // Moderate truncation
    } else if (stats.truncation_rate < 0.05 && stats.avg_quality > 0.8) {
      optimal = Math.ceil(optimal * 0.9); // Low truncation + good quality → save tokens
    } else {
      optimal = Math.ceil(optimal * 1.2); // Default: 20% headroom
    }

    // Clamp
    const floor = parseInt(getSetting("adaptive_budget_floor", "256"));
    const ceiling = parseInt(getSetting("adaptive_budget_ceiling", "16384"));
    optimal = Math.max(floor, Math.min(ceiling, optimal));

    return {
      maxTokens: optimal,
      category,
      source: "adaptive",
      samples: stats.samples,
      avgUsed: Math.round(stats.avg_used),
      truncationRate: Math.round(stats.truncation_rate * 100),
      avgQuality: stats.avg_quality.toFixed(2),
    };
  } catch (e) {
    return { maxTokens: fallbackMax, category, source: "error", error: e.message };
  }
}

// Get adaptive budget stats (for dashboard)
export function getAdaptiveBudgetStats() {
  try {
    ensureTable();
    const db = getDb();

    const stats = db.prepare(`
      SELECT 
        task_category,
        COUNT(*) as samples,
        ROUND(AVG(actual_used)) as avg_tokens,
        ROUND(AVG(CASE WHEN finish_reason = 'length' THEN 1.0 ELSE 0.0 END) * 100) as truncation_pct,
        ROUND(AVG(quality_score), 2) as avg_quality
      FROM adaptive_token_budget
      WHERE created_at > datetime('now', '-7 days')
      GROUP BY task_category
      ORDER BY samples DESC
    `).all();

    return { enabled: getSetting("adaptive_budget_enabled", "true") === "true", stats };
  } catch (e) {
    return { enabled: false, stats: [], error: e.message };
  }
}

export { classifyTask };
