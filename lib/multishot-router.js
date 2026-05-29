// Multi-shot Routing — retry with alternative model if response quality is low
// Integrates with quality-filter to decide when to retry
import { getDb, getSetting } from "./db/index.js";

// Get alternative models for retry (different provider preferred)
export function getRetryModels(currentModel) {
  const enabled = getSetting("multishot_enabled", "true") === "true";
  if (!enabled) return [];

  try {
    const db = getDb();

    // Get all active models except current
    const models = db.prepare(`
      SELECT m.model_id, m.provider, c.name as connection_name, c.id as connection_id
      FROM models m
      JOIN connections c ON m.connection_id = c.id
      WHERE m.model_id != ? AND m.enabled = 1 AND c.enabled = 1
      ORDER BY c.priority ASC
    `).all(currentModel);

    if (models.length === 0) return [];

    // Prefer different provider for diversity
    const currentProvider = currentModel.split("/")[0];
    const differentProvider = models.filter((m) => !m.provider?.includes(currentProvider));
    const sameProvider = models.filter((m) => m.provider?.includes(currentProvider));

    // Return up to 2 alternatives: prefer different provider first
    const alternatives = [...differentProvider, ...sameProvider].slice(0, 2);

    return alternatives.map((m) => ({
      model: m.model_id,
      provider: m.provider,
      connectionId: m.connection_id,
      connectionName: m.connection_name,
    }));
  } catch (e) {
    return [];
  }
}

// Decide whether to retry based on quality score
export function shouldRetry(qualityResult, attempt, maxRetries) {
  const enabled = getSetting("multishot_enabled", "true") === "true";
  if (!enabled) return false;

  const max = maxRetries || parseInt(getSetting("multishot_max_retries", "1"));
  if (attempt >= max) return false;

  // Retry if quality is below threshold
  if (!qualityResult.pass) return true;

  // Also retry if score is very low even if technically "passing"
  if (qualityResult.score < 0.3) return true;

  return false;
}

// Record retry outcome for learning
export function recordRetryOutcome(originalModel, retryModel, originalScore, retryScore, taskCategory) {
  try {
    const db = getDb();
    db.exec(`
      CREATE TABLE IF NOT EXISTS multishot_outcomes (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        original_model TEXT,
        retry_model TEXT,
        original_score REAL,
        retry_score REAL,
        task_category TEXT,
        retry_helped INTEGER,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
      );
    `);

    db.prepare(`
      INSERT INTO multishot_outcomes (original_model, retry_model, original_score, retry_score, task_category, retry_helped)
      VALUES (?, ?, ?, ?, ?, ?)
    `).run(originalModel, retryModel, originalScore, retryScore, taskCategory, retryScore > originalScore ? 1 : 0);
  } catch (e) {
    // Silent fail
  }
}

// Get retry stats (for dashboard)
export function getMultishotStats() {
  try {
    const db = getDb();
    db.exec(`
      CREATE TABLE IF NOT EXISTS multishot_outcomes (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        original_model TEXT,
        retry_model TEXT,
        original_score REAL,
        retry_score REAL,
        task_category TEXT,
        retry_helped INTEGER,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
      );
    `);

    const stats = db.prepare(`
      SELECT 
        COUNT(*) as total_retries,
        SUM(retry_helped) as successful_retries,
        ROUND(AVG(retry_score - original_score), 3) as avg_improvement,
        ROUND(AVG(CASE WHEN retry_helped = 1 THEN retry_score ELSE NULL END), 2) as avg_retry_score
      FROM multishot_outcomes
      WHERE created_at > datetime('now', '-7 days')
    `).get();

    return {
      enabled: getSetting("multishot_enabled", "true") === "true",
      maxRetries: parseInt(getSetting("multishot_max_retries", "1")),
      ...stats,
      successRate: stats.total_retries > 0
        ? Math.round((stats.successful_retries / stats.total_retries) * 100)
        : 0,
    };
  } catch (e) {
    return { enabled: false, error: e.message };
  }
}
