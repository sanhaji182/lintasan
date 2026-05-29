// Lintasan - Routing Engine
// Handles: rate limiting, load balancing, fallback chains, model aliasing
import { getDb, listConnections, getConnection, getSetting, findModelConnections, getActiveConnections } from "./db/index.js";

// === RATE LIMITING ===
const rateLimitWindows = new Map();

export function checkRateLimit(apiKey) {
  const limitPerMin = parseInt(getSetting("rate_limit_rpm", "60"));
  if (limitPerMin <= 0) return { allowed: true };

  const now = Date.now();
  const windowMs = 60_000;
  const key = apiKey.slice(0, 16);

  let window = rateLimitWindows.get(key);
  if (!window || now - window.windowStart > windowMs) {
    window = { count: 0, windowStart: now };
    rateLimitWindows.set(key, window);
  }

  window.count++;
  if (window.count > limitPerMin) {
    return { allowed: false, remaining: 0, resetIn: Math.ceil((window.windowStart + windowMs - now) / 1000) };
  }

  return { allowed: true, remaining: limitPerMin - window.count };
}

// === LOAD BALANCING ===
// Strategy: round-robin, least-connections, random, priority (default)
const rotationIndex = new Map(); // model -> index
const activeRequests = new Map(); // connection_id -> count

export function getLoadBalanceStrategy() {
  return getSetting("load_balance_strategy", "priority");
}

export function selectConnection(connections, model) {
  if (connections.length === 0) return null;
  if (connections.length === 1) return connections[0];

  const strategy = getLoadBalanceStrategy();

  switch (strategy) {
    case "round-robin": {
      const key = model || "default";
      const currentIdx = rotationIndex.get(key) || 0;
      const nextIdx = (currentIdx + 1) % connections.length;
      rotationIndex.set(key, nextIdx);
      return connections[currentIdx];
    }
    case "least-connections": {
      let minConn = connections[0];
      let minCount = activeRequests.get(connections[0].id) || 0;
      for (let i = 1; i < connections.length; i++) {
        const count = activeRequests.get(connections[i].id) || 0;
        if (count < minCount) { minConn = connections[i]; minCount = count; }
      }
      return minConn;
    }
    case "random": {
      return connections[Math.floor(Math.random() * connections.length)];
    }
    case "priority":
    default: {
      // Already sorted by priority DESC from DB query
      return connections[0];
    }
  }
}

export function trackRequestStart(connectionId) {
  activeRequests.set(connectionId, (activeRequests.get(connectionId) || 0) + 1);
}

export function trackRequestEnd(connectionId) {
  const count = activeRequests.get(connectionId) || 0;
  activeRequests.set(connectionId, Math.max(0, count - 1));
}

// === FALLBACK CHAIN ===
// Enhanced fallback system - delegates to lib/fallback-chain.js
// These exports are kept for backward compatibility
import {
  getFallbackConnections as _getFallbackConnections,
  setFallbackConnections as _setFallbackConnections,
  getAllFallbackChains as _getAllFallbackChains,
} from "./fallback-chain.js";

export function getFallbackChain(primaryConnectionId) {
  return _getFallbackConnections(primaryConnectionId);
}

export function setFallbackChain(connectionId, fallbackConnectionIds) {
  _setFallbackConnections(connectionId, fallbackConnectionIds || []);
}

export function getAllFallbackChains() {
  return _getAllFallbackChains();
}

// === MODEL ALIASING ===
export function resolveModelAlias(model) {
  try {
    const aliasesJson = getSetting("model_aliases", "{}");
    const aliases = JSON.parse(aliasesJson);
    return aliases[model] || null; // Returns { model, connection_id } or null
  } catch {
    return null;
  }
}

export function getAllModelAliases() {
  try {
    const aliasesJson = getSetting("model_aliases", "{}");
    return JSON.parse(aliasesJson);
  } catch {
    return {};
  }
}

export function setModelAlias(alias, model, connectionId) {
  try {
    const aliasesJson = getSetting("model_aliases", "{}");
    const aliases = JSON.parse(aliasesJson);
    if (model && connectionId) {
      aliases[alias] = { model, connection_id: connectionId };
    } else if (model) {
      aliases[alias] = { model };
    } else {
      delete aliases[alias];
    }
    const db = getDb();
    db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("model_aliases", JSON.stringify(aliases));
  } catch {}
}

// Legacy compat
export function selectConnectionRotated(providerId) {
  const connections = listConnections();
  const active = connections.filter(c => c.is_active);
  if (active.length === 0) return null;
  if (active.length === 1) return active[0];
  const currentIdx = rotationIndex.get(providerId) || 0;
  const nextIdx = (currentIdx + 1) % active.length;
  rotationIndex.set(providerId, nextIdx);
  return active[currentIdx];
}
