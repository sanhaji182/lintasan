// Load Balancing Strategies
import { getDb, listConnections, getSetting } from "./db/index.js";

// In-memory latency tracker
const latencyTracker = new Map(); // connectionId -> { avgLatency, requestCount, lastUpdated }

export function recordLatency(connectionId, latencyMs) {
  const entry = latencyTracker.get(connectionId) || { avgLatency: 0, requestCount: 0, lastUpdated: 0 };
  // Exponential moving average
  const alpha = 0.3;
  entry.avgLatency = entry.requestCount === 0 ? latencyMs : (alpha * latencyMs + (1 - alpha) * entry.avgLatency);
  entry.requestCount++;
  entry.lastUpdated = Date.now();
  latencyTracker.set(connectionId, entry);
}

export function getLatencyStats() {
  const stats = {};
  for (const [id, entry] of latencyTracker) {
    stats[id] = { avgLatency: Math.round(entry.avgLatency), requests: entry.requestCount };
  }
  return stats;
}

// Get load balancing strategy for a provider
export function getLoadBalanceStrategy(providerId) {
  try {
    const configJson = getSetting("lb_strategies", "{}");
    const config = JSON.parse(configJson);
    return config[providerId] || "round-robin"; // default
  } catch {
    return "round-robin";
  }
}

export function setLoadBalanceStrategy(providerId, strategy) {
  try {
    const configJson = getSetting("lb_strategies", "{}");
    const config = JSON.parse(configJson);
    config[providerId] = strategy;
    const db = getDb();
    db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("lb_strategies", JSON.stringify(config));
  } catch {}
}

export function getAllStrategies() {
  try {
    const configJson = getSetting("lb_strategies", "{}");
    return JSON.parse(configJson);
  } catch {
    return {};
  }
}

// Weighted selection
const weightedIndex = new Map();

export function selectConnectionByStrategy(providerId) {
  const connections = listConnections(providerId);
  const active = connections.filter(c => c.is_active);
  if (active.length === 0) return null;
  if (active.length === 1) return active[0];

  const strategy = getLoadBalanceStrategy(providerId);

  switch (strategy) {
    case "priority":
      // Always use highest priority
      return active[0]; // Already sorted by priority DESC

    case "round-robin": {
      const idx = weightedIndex.get(providerId) || 0;
      const next = (idx + 1) % active.length;
      weightedIndex.set(providerId, next);
      return active[idx];
    }

    case "weighted": {
      // Weight by priority value (higher priority = more traffic)
      const totalWeight = active.reduce((sum, c) => sum + Math.max(c.priority, 1), 0);
      let random = Math.random() * totalWeight;
      for (const conn of active) {
        random -= Math.max(conn.priority, 1);
        if (random <= 0) return conn;
      }
      return active[0];
    }

    case "least-latency": {
      // Pick connection with lowest average latency
      let bestConn = active[0];
      let bestLatency = Infinity;
      for (const conn of active) {
        const entry = latencyTracker.get(conn.id);
        const lat = entry ? entry.avgLatency : 999999;
        if (lat < bestLatency) {
          bestLatency = lat;
          bestConn = conn;
        }
      }
      return bestConn;
    }

    case "random":
      return active[Math.floor(Math.random() * active.length)];

    default:
      return active[0];
  }
}

// Available strategies
export const STRATEGIES = ["priority", "round-robin", "weighted", "least-latency", "random"];
