// Multi-Account Pooling — multiple credentials per connection with round-robin
// Allows users to add multiple API keys per provider for load distribution
// Each key gets its own cooldown tracking

import { getDb, getSetting } from "./db/index.js";
import { v4 as uuidv4 } from "uuid";

// In-memory state
const rotationIndex = new Map(); // connection_id -> current key index
const keyCooldowns = new Map(); // key_id -> { until: timestamp, reason: string }

// DB operations
export function addAccountKey(connectionId, apiKey, label = "") {
  const db = getDb();
  // Ensure table exists
  db.exec(`CREATE TABLE IF NOT EXISTS connection_keys (
    id TEXT PRIMARY KEY,
    connection_id TEXT NOT NULL,
    api_key TEXT NOT NULL,
    label TEXT DEFAULT '',
    is_active INTEGER DEFAULT 1,
    total_requests INTEGER DEFAULT 0,
    total_tokens INTEGER DEFAULT 0,
    last_used TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE
  )`);

  const id = uuidv4();
  db.prepare("INSERT INTO connection_keys (id, connection_id, api_key, label) VALUES (?, ?, ?, ?)").run(id, connectionId, apiKey, label);
  return { id, connection_id: connectionId, label };
}

export function listAccountKeys(connectionId) {
  const db = getDb();
  try {
    return db.prepare("SELECT id, connection_id, label, is_active, total_requests, total_tokens, last_used, created_at FROM connection_keys WHERE connection_id = ? AND is_active = 1 ORDER BY created_at").all(connectionId);
  } catch {
    return [];
  }
}

export function removeAccountKey(keyId) {
  const db = getDb();
  try {
    db.prepare("DELETE FROM connection_keys WHERE id = ?").run(keyId);
  } catch {}
}

// Get the next available API key for a connection (round-robin, skip cooldowns)
export function getNextKey(connectionId, primaryKey) {
  const keys = listAccountKeys(connectionId);

  // If no extra keys, use the primary connection key
  if (keys.length === 0) return { key: primaryKey, keyId: "primary" };

  // All available keys = primary + extra keys
  const allKeys = [{ id: "primary", api_key: primaryKey }, ...keys.map(k => ({ id: k.id, api_key: k.api_key || primaryKey }))];

  // Filter out cooled-down keys
  const now = Date.now();
  const available = allKeys.filter(k => {
    const cooldown = keyCooldowns.get(k.id);
    return !cooldown || cooldown.until < now;
  });

  if (available.length === 0) {
    // All keys in cooldown — use the one with earliest recovery
    let earliest = allKeys[0];
    let earliestTime = Infinity;
    for (const k of allKeys) {
      const cd = keyCooldowns.get(k.id);
      if (!cd || cd.until < earliestTime) {
        earliestTime = cd ? cd.until : 0;
        earliest = k;
      }
    }
    return { key: earliest.api_key, keyId: earliest.id };
  }

  // Round-robin among available keys
  const idx = rotationIndex.get(connectionId) || 0;
  const nextIdx = (idx + 1) % available.length;
  rotationIndex.set(connectionId, nextIdx);

  const selected = available[idx % available.length];
  return { key: selected.api_key, keyId: selected.id };
}

// Record key usage
export function recordKeyUsage(connectionId, keyId, tokens = 0) {
  if (keyId === "primary") return;
  const db = getDb();
  try {
    db.prepare("UPDATE connection_keys SET total_requests = total_requests + 1, total_tokens = total_tokens + ?, last_used = datetime('now') WHERE id = ?").run(tokens, keyId);
  } catch {}
}

// Put a key in cooldown (e.g., rate limited)
export function cooldownKey(keyId, durationMs = 60000, reason = "rate_limited") {
  keyCooldowns.set(keyId, { until: Date.now() + durationMs, reason });
}

// Clear cooldown
export function clearCooldown(keyId) {
  keyCooldowns.delete(keyId);
}

// Get cooldown status for all keys of a connection
export function getCooldownStatus(connectionId) {
  const keys = listAccountKeys(connectionId);
  const now = Date.now();
  return keys.map(k => ({
    id: k.id,
    label: k.label,
    cooldown: keyCooldowns.get(k.id),
    available: !keyCooldowns.has(k.id) || keyCooldowns.get(k.id).until < now,
  }));
}
