import { getDb, getSetting, setSetting, listConnections, createConnection } from "./db/index.js";
import crypto from "crypto";

// Export config as portable JSON (no secrets by default)
export function exportConfig({ includeKeys = false } = {}) {
  const db = getDb();

  // Settings (exclude sensitive)
  const allSettings = db.prepare("SELECT key, value FROM settings").all();
  const sensitiveKeys = ["master_key", "dashboard_password", "embedding_api_key", "oauth_github_token", "oauth_github_session_token"];
  const settings = allSettings.filter((s) => includeKeys || !sensitiveKeys.includes(s.key));

  // Connections
  const connections = listConnections().map((c) => ({
    name: c.name,
    base_url: c.base_url,
    format: c.format,
    chat_path: c.chat_path,
    models_path: c.models_path,
    auth_header: c.auth_header,
    auth_prefix: c.auth_prefix,
    extra_headers: c.extra_headers,
    priority: c.priority,
    is_active: c.is_active,
    // Only include API key if explicitly requested
    ...(includeKeys ? { api_key: c.api_key } : {}),
  }));

  // Combos
  const combosRaw = getSetting("combos_v2", "[]");
  let combos = [];
  try { combos = JSON.parse(combosRaw); } catch {}

  // Plugins (names only, not code)
  const plugins = db.prepare("SELECT name, description, enabled, priority FROM plugins").all().catch?.(() => []) || [];

  const config = {
    version: "1.0.0",
    exported_at: new Date().toISOString(),
    instance_id: getSetting("instance_id", generateInstanceId()),
    settings,
    connections,
    combos,
    plugins,
  };

  return config;
}

// Import config from JSON
export function importConfig(config, { overwrite = false, mergeConnections = true } = {}) {
  const db = getDb();
  const results = { settings: 0, connections: 0, combos: 0 };

  // Import settings
  if (config.settings) {
    const stmt = overwrite
      ? db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)")
      : db.prepare("INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)");

    for (const { key, value } of config.settings) {
      stmt.run(key, value);
      results.settings++;
    }
  }

  // Import connections
  if (config.connections) {
    const existing = listConnections();

    for (const conn of config.connections) {
      // Skip if already exists (by name or base_url)
      if (mergeConnections) {
        const exists = existing.some(
          (e) => e.name === conn.name || e.base_url === conn.base_url
        );
        if (exists) continue;
      }

      try {
        createConnection({
          name: conn.name,
          baseUrl: conn.base_url,
          apiKey: conn.api_key || "",
          format: conn.format || "openai",
          chatPath: conn.chat_path || "/v1/chat/completions",
          modelsPath: conn.models_path || "/v1/models",
          authHeader: conn.auth_header || "Authorization",
          authPrefix: conn.auth_prefix || "Bearer ",
          extraHeaders: conn.extra_headers || "{}",
          priority: conn.priority || 0,
        });
        results.connections++;
      } catch {}
    }
  }

  // Import combos
  if (config.combos && config.combos.length > 0) {
    if (overwrite) {
      setSetting("combos_v2", JSON.stringify(config.combos));
    } else {
      // Merge — add combos that don't exist by name
      const existingRaw = getSetting("combos_v2", "[]");
      let existing = [];
      try { existing = JSON.parse(existingRaw); } catch {}

      const existingNames = new Set(existing.map((c) => c.name));
      const newCombos = config.combos.filter((c) => !existingNames.has(c.name));

      if (newCombos.length > 0) {
        setSetting("combos_v2", JSON.stringify([...existing, ...newCombos]));
        results.combos = newCombos.length;
      }
    }
  }

  return results;
}

// Generate a unique instance ID for this installation
function generateInstanceId() {
  const id = crypto.randomUUID();
  setSetting("instance_id", id);
  return id;
}

// Generate a share code (short URL-safe string)
export function generateShareCode(config) {
  const json = JSON.stringify(config);
  const compressed = Buffer.from(json).toString("base64url");
  return compressed;
}

// Decode a share code back to config
export function decodeShareCode(code) {
  try {
    const json = Buffer.from(code, "base64url").toString("utf8");
    return JSON.parse(json);
  } catch {
    throw new Error("Invalid share code");
  }
}
