import { getDb, getSetting, setSetting } from "./db/index.js";

// === FALLBACK TRIGGERS ===
export const FALLBACK_REASONS = {
  TIMEOUT: "timeout",
  SERVER_ERROR: "5xx",
  RATE_LIMIT: "429",
  CIRCUIT_BREAKER: "circuit_breaker_open",
};

// === SETTINGS KEYS ===
const MODEL_CHAINS_KEY = "fallback_model_chains";
const CONNECTION_CHAINS_KEY = "fallback_connection_chains";
const FALLBACK_METRICS_KEY = "fallback_metrics";

// === MODEL FALLBACK ===

/**
 * Get fallback models for a given model.
 * Returns an ordered array of model IDs to try if the primary model fails.
 * @param {string} model - The primary model ID
 * @returns {string[]} Ordered list of fallback model IDs
 */
export function getFallbackModels(model) {
  try {
    const chainsJson = getSetting(MODEL_CHAINS_KEY, "{}");
    const chains = JSON.parse(chainsJson);
    return chains[model] || [];
  } catch {
    return [];
  }
}

/**
 * Set fallback models for a given model.
 * @param {string} model - The primary model ID
 * @param {string[]} fallbackModels - Ordered list of fallback model IDs
 */
export function setFallbackModels(model, fallbackModels) {
  try {
    const chainsJson = getSetting(MODEL_CHAINS_KEY, "{}");
    const chains = JSON.parse(chainsJson);
    if (!fallbackModels || fallbackModels.length === 0) {
      delete chains[model];
    } else {
      chains[model] = fallbackModels;
    }
    setSetting(MODEL_CHAINS_KEY, JSON.stringify(chains));
  } catch {
    // silent fail
  }
}

/**
 * Get all model fallback chains.
 * @returns {Object} Map of model -> fallback model array
 */
export function getAllModelChains() {
  try {
    const chainsJson = getSetting(MODEL_CHAINS_KEY, "{}");
    return JSON.parse(chainsJson);
  } catch {
    return {};
  }
}

// === CONNECTION FALLBACK ===

/**
 * Get fallback connections for a given connection.
 * Returns an ordered array of connection IDs to try if the primary connection fails.
 * @param {string} connectionId - The primary connection ID
 * @returns {string[]} Ordered list of fallback connection IDs
 */
export function getFallbackConnections(connectionId) {
  try {
    const chainsJson = getSetting(CONNECTION_CHAINS_KEY, "{}");
    const chains = JSON.parse(chainsJson);
    return chains[connectionId] || [];
  } catch {
    return [];
  }
}

/**
 * Set fallback connections for a given connection.
 * @param {string} connectionId - The primary connection ID
 * @param {string[]} fallbackConnectionIds - Ordered list of fallback connection IDs
 */
export function setFallbackConnections(connectionId, fallbackConnectionIds) {
  try {
    const chainsJson = getSetting(CONNECTION_CHAINS_KEY, "{}");
    const chains = JSON.parse(chainsJson);
    if (!fallbackConnectionIds || fallbackConnectionIds.length === 0) {
      delete chains[connectionId];
    } else {
      chains[connectionId] = fallbackConnectionIds;
    }
    setSetting(CONNECTION_CHAINS_KEY, JSON.stringify(chains));
  } catch {
    // silent fail
  }
}

/**
 * Get all connection fallback chains.
 * @returns {Object} Map of connectionId -> fallback connection array
 */
export function getAllConnectionChains() {
  try {
    const chainsJson = getSetting(CONNECTION_CHAINS_KEY, "{}");
    return JSON.parse(chainsJson);
  } catch {
    return {};
  }
}

// === FALLBACK METRICS ===

/**
 * Ensure the fallback_metrics table exists.
 */
function ensureMetricsTable() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS fallback_metrics (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      from_id TEXT NOT NULL,
      to_id TEXT NOT NULL,
      chain_type TEXT NOT NULL DEFAULT 'connection',
      reason TEXT NOT NULL,
      created_at TEXT DEFAULT (datetime('now'))
    );
    CREATE INDEX IF NOT EXISTS idx_fallback_metrics_from ON fallback_metrics(from_id);
    CREATE INDEX IF NOT EXISTS idx_fallback_metrics_reason ON fallback_metrics(reason);
    CREATE INDEX IF NOT EXISTS idx_fallback_metrics_created ON fallback_metrics(created_at);
  `);
}

/**
 * Record a fallback usage event.
 * @param {string} from - The source model or connection ID that failed
 * @param {string} to - The fallback model or connection ID that was used
 * @param {string} reason - One of FALLBACK_REASONS values
 * @param {string} [chainType='connection'] - 'model' or 'connection'
 */
export function recordFallbackUsage(from, to, reason, chainType = "connection") {
  try {
    ensureMetricsTable();
    const db = getDb();
    db.prepare(
      "INSERT INTO fallback_metrics (from_id, to_id, chain_type, reason) VALUES (?, ?, ?, ?)"
    ).run(from, to, chainType, reason);
  } catch {
    // silent fail - metrics should not break request flow
  }
}

/**
 * Get fallback usage statistics.
 * @param {Object} [options] - Filter options
 * @param {string} [options.from] - Filter by source ID
 * @param {string} [options.chainType] - Filter by chain type ('model' or 'connection')
 * @param {string} [options.reason] - Filter by reason
 * @param {number} [options.hours=24] - Time window in hours
 * @returns {Object} Statistics object with totals and breakdowns
 */
export function getFallbackStats(options = {}) {
  try {
    ensureMetricsTable();
    const db = getDb();
    const hours = options.hours || 24;
    const since = new Date(Date.now() - hours * 60 * 60 * 1000).toISOString();

    let whereClause = "WHERE created_at >= ?";
    const params = [since];

    if (options.from) {
      whereClause += " AND from_id = ?";
      params.push(options.from);
    }
    if (options.chainType) {
      whereClause += " AND chain_type = ?";
      params.push(options.chainType);
    }
    if (options.reason) {
      whereClause += " AND reason = ?";
      params.push(options.reason);
    }

    // Total count
    const total = db.prepare(
      `SELECT COUNT(*) as count FROM fallback_metrics ${whereClause}`
    ).get(...params);

    // Breakdown by reason
    const byReason = db.prepare(
      `SELECT reason, COUNT(*) as count FROM fallback_metrics ${whereClause} GROUP BY reason ORDER BY count DESC`
    ).all(...params);

    // Breakdown by from -> to pair
    const byPair = db.prepare(
      `SELECT from_id, to_id, chain_type, COUNT(*) as count FROM fallback_metrics ${whereClause} GROUP BY from_id, to_id, chain_type ORDER BY count DESC LIMIT 50`
    ).all(...params);

    // Breakdown by chain type
    const byChainType = db.prepare(
      `SELECT chain_type, COUNT(*) as count FROM fallback_metrics ${whereClause} GROUP BY chain_type ORDER BY count DESC`
    ).all(...params);

    return {
      total: total.count,
      hours,
      by_reason: byReason,
      by_pair: byPair,
      by_chain_type: byChainType,
    };
  } catch {
    return { total: 0, hours: 24, by_reason: [], by_pair: [], by_chain_type: [] };
  }
}

// === UTILITY: Determine if an error should trigger fallback ===

/**
 * Check if a given error/status should trigger a fallback.
 * @param {number|null} status - HTTP status code (null for timeout/network errors)
 * @param {Error|null} error - The error object
 * @param {boolean} circuitBreakerOpen - Whether the circuit breaker is open for this target
 * @returns {{ shouldFallback: boolean, reason: string|null }}
 */
export function shouldTriggerFallback(status, error, circuitBreakerOpen = false) {
  if (circuitBreakerOpen) {
    return { shouldFallback: true, reason: FALLBACK_REASONS.CIRCUIT_BREAKER };
  }
  if (error && (error.code === "ETIMEDOUT" || error.code === "ECONNABORTED" || error.name === "AbortError" || error.message?.includes("timeout"))) {
    return { shouldFallback: true, reason: FALLBACK_REASONS.TIMEOUT };
  }
  if (status === 429) {
    return { shouldFallback: true, reason: FALLBACK_REASONS.RATE_LIMIT };
  }
  if (status && status >= 500 && status < 600) {
    return { shouldFallback: true, reason: FALLBACK_REASONS.SERVER_ERROR };
  }
  return { shouldFallback: false, reason: null };
}

// === COMBINED GETTER (backward compat with router.js) ===

/**
 * Get all fallback chains (both model and connection).
 * @returns {Object} Combined chains object
 */
export function getAllFallbackChains() {
  return {
    model_chains: getAllModelChains(),
    connection_chains: getAllConnectionChains(),
  };
}

/**
 * Delete a specific fallback chain.
 * @param {string} type - 'model' or 'connection'
 * @param {string} id - The model or connection ID
 */
export function deleteFallbackChain(type, id) {
  if (type === "model") {
    setFallbackModels(id, []);
  } else {
    setFallbackConnections(id, []);
  }
}
