// Combo System v2 — Hybrid model
// Combo = model + accounts (keys) + strategy, all in one place
// Like 9Router: create combo → use as model → done
import { getDb, getSetting, getConnection, getActiveConnections } from "./db/index.js";
import { getNextKey } from "./multi-account.js";

// In-memory sticky state (resets on restart)
const stickyState = new Map(); // comboName -> { lastIndex, successCount }

// Get all combos
export function listCombos() {
  try {
    const json = getSetting("combos_v2", null) || getSetting("combos", "[]");
    return JSON.parse(json);
  } catch { return []; }
}

// Get a specific combo by name
export function getCombo(name) {
  const combos = listCombos();
  return combos.find(c => c.name === name) || null;
}

// Create/update a combo
// Combo structure v2:
// {
//   name: "coding",
//   description: "Best for code tasks",
//   strategy: "priority" | "round-robin",
//   stickyLimit: 3,
//   entries: [
//     { model: "deepseek/deepseek-v4-pro", connection_ids: ["conn-1", "conn-2"], priority: 1 },
//     { model: "kimi/kimi-k2.6", connection_ids: ["conn-3"], priority: 2 },
//   ]
// }
export function saveCombo(combo) {
  const combos = listCombos();
  const idx = combos.findIndex(c => c.name === combo.name);
  if (idx >= 0) {
    combos[idx] = { ...combos[idx], ...combo, updated_at: new Date().toISOString() };
  } else {
    combos.push({ ...combo, created_at: new Date().toISOString() });
  }
  const db = getDb();
  db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("combos_v2", JSON.stringify(combos));
  return combo;
}

// Delete a combo
export function deleteCombo(name) {
  const combos = listCombos().filter(c => c.name !== name);
  const db = getDb();
  db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("combos_v2", JSON.stringify(combos));
}

// Check if a model name is a combo
export function isCombo(modelName) {
  return getCombo(modelName) !== null;
}

// Resolve combo to execution plan
// Returns ordered list of { model, connection, apiKey } ready to execute
export function resolveComboModel(comboName) {
  const combo = getCombo(comboName);
  if (!combo || !combo.entries || combo.entries.length === 0) {
    // Legacy format support (v1 combos with .models array)
    if (combo && combo.models && combo.models.length > 0) {
      return resolveLegacyCombo(combo);
    }
    return null;
  }

  const strategy = combo.strategy || "priority";
  const stickyLimit = combo.stickyLimit || 3;
  const state = stickyState.get(comboName) || { lastIndex: 0, successCount: 0 };

  // Build execution plan: expand entries into concrete connection+key pairs
  const plan = [];
  const startIdx = strategy === "round-robin" ? state.lastIndex : 0;

  for (let i = 0; i < combo.entries.length; i++) {
    const idx = (startIdx + i) % combo.entries.length;
    const entry = combo.entries[idx];

    // Get connections for this entry
    const connIds = entry.connection_ids || [];
    for (const connId of connIds) {
      const conn = getConnection(connId);
      if (!conn || !conn.is_active) continue;

      // Get next available key (multi-account rotation)
      const { key: activeKey, keyId } = getNextKey(conn.id, conn.api_key);

      plan.push({
        model: entry.model,
        connection: { ...conn, api_key: activeKey },
        keyId,
        entryIndex: idx,
        label: entry.label || entry.model,
      });
    }

    // If no specific connections, try to find any connection with this model
    if (connIds.length === 0) {
      const { findModelConnections } = require("./db/index.js");
      const matches = findModelConnections(entry.model);
      for (const m of matches) {
        const conn = getConnection(m.connection_id);
        if (!conn || !conn.is_active) continue;
        const { key: activeKey, keyId } = getNextKey(conn.id, conn.api_key);
        plan.push({
          model: entry.model,
          connection: { ...conn, api_key: activeKey },
          keyId,
          entryIndex: idx,
          label: entry.label || entry.model,
        });
      }
    }
  }

  return {
    models: plan,
    startIndex: 0,
    combo,
    strategy,
    stickyLimit,
  };
}

// Legacy v1 combo support
function resolveLegacyCombo(combo) {
  const stickyLimit = combo.stickyLimit || 3;
  const state = stickyState.get(combo.name) || { lastIndex: 0, successCount: 0 };

  return {
    models: combo.models,
    startIndex: state.lastIndex,
    combo,
    strategy: "priority",
    stickyLimit,
  };
}

// Record success — update sticky state
export function recordComboSuccess(comboName, modelIndex) {
  const combo = getCombo(comboName);
  if (!combo) return;

  const entries = combo.entries || combo.models || [];
  const state = stickyState.get(comboName) || { lastIndex: 0, successCount: 0 };
  const stickyLimit = combo.stickyLimit || 3;

  if (modelIndex === state.lastIndex) {
    state.successCount++;
    if (state.successCount >= stickyLimit) {
      state.lastIndex = (state.lastIndex + 1) % entries.length;
      state.successCount = 0;
    }
  } else {
    state.lastIndex = modelIndex;
    state.successCount = 1;
  }

  stickyState.set(comboName, state);
}

// Record failure
export function recordComboFailure(comboName, modelIndex) {
  const state = stickyState.get(comboName) || { lastIndex: 0, successCount: 0 };
  const combo = getCombo(comboName);
  if (!combo) return;

  const entries = combo.entries || combo.models || [];
  if (modelIndex === state.lastIndex) {
    state.lastIndex = (modelIndex + 1) % entries.length;
    state.successCount = 0;
    stickyState.set(comboName, state);
  }
}
