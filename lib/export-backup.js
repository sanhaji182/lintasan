// Export & Backup system for Lintasan LLM proxy
import fs from "fs";
import path from "path";
import { getDb, getSetting, setSetting, listConnections } from "./db/index.js";
import { listApiKeys } from "./api-keys.js";
import { listCombos } from "./combo.js";
import { getAllModelAliases } from "./router.js";
import { getAllFallbackChains } from "./fallback-chain.js";

const DATA_DIR = path.join(process.cwd(), "data");
const DB_PATH = path.join(DATA_DIR, "lintasan.db");
const BACKUP_DIR = path.join(DATA_DIR, "backups");

function ensureBackupDir() {
  if (!fs.existsSync(BACKUP_DIR)) {
    fs.mkdirSync(BACKUP_DIR, { recursive: true });
  }
}

/**
 * Export all config: settings, connections (no key masking), combos, aliases, fallback chains
 */
export function exportConfig() {
  const db = getDb();

  // All settings
  const settingsRows = db.prepare("SELECT key, value FROM settings").all();
  const settings = {};
  for (const row of settingsRows) {
    settings[row.key] = row.value;
  }

  // Connections with full details (without masking API keys)
  const connections = db.prepare("SELECT * FROM connections").all();

  // Combos
  const combos = listCombos();

  // Aliases
  const aliases = getAllModelAliases();

  // Fallback chains
  const fallbackChains = getAllFallbackChains();

  return {
    version: 1,
    exported_at: new Date().toISOString(),
    settings,
    connections,
    combos,
    aliases,
    fallback_chains: fallbackChains,
  };
}

/**
 * Export analytics/request logs as JSON or CSV with optional date range
 * @param {Object} options - { format: 'json'|'csv', from: 'YYYY-MM-DD', to: 'YYYY-MM-DD', limit: number }
 */
export function exportAnalytics(options = {}) {
  const db = getDb();
  const { format = "json", from, to, limit } = options;

  let query = "SELECT * FROM request_logs";
  const conditions = [];
  const params = [];

  if (from) {
    conditions.push("created_at >= ?");
    params.push(from);
  }
  if (to) {
    conditions.push("created_at <= ?");
    params.push(to + " 23:59:59");
  }

  if (conditions.length > 0) {
    query += " WHERE " + conditions.join(" AND ");
  }
  query += " ORDER BY created_at DESC";

  if (limit) {
    query += " LIMIT ?";
    params.push(Number(limit));
  }

  const rows = db.prepare(query).all(...params);

  if (format === "csv") {
    if (rows.length === 0) return "id,connection_id,provider,model,status,input_tokens,output_tokens,latency_ms,error,created_at\n";
    const headers = Object.keys(rows[0]);
    const csvLines = [headers.join(",")];
    for (const row of rows) {
      const line = headers.map((h) => {
        const val = row[h];
        if (val === null || val === undefined) return "";
        const str = String(val);
        // Escape CSV values containing commas or quotes
        if (str.includes(",") || str.includes('"') || str.includes("\n")) {
          return '"' + str.replace(/"/g, '""') + '"';
        }
        return str;
      });
      csvLines.push(line.join(","));
    }
    return csvLines.join("\n");
  }

  return {
    exported_at: new Date().toISOString(),
    count: rows.length,
    filters: { from: from || null, to: to || null },
    logs: rows,
  };
}

/**
 * Export API keys list with keys partially masked
 */
export function exportApiKeys() {
  const keys = listApiKeys();
  return {
    exported_at: new Date().toISOString(),
    keys: keys.map((k) => ({
      ...k,
      key: k.key ? k.key.slice(0, 6) + "..." + k.key.slice(-4) : "***",
    })),
  };
}

/**
 * Import config from exported JSON (settings, connections)
 * @param {Object} json - The exported config object
 */
export function importConfig(json) {
  const db = getDb();

  const imported = { settings: 0, connections: 0, combos: 0, aliases: false, fallback_chains: false };

  // Import settings
  if (json.settings && typeof json.settings === "object") {
    const stmt = db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)");
    const importSettings = db.transaction((settings) => {
      for (const [key, value] of Object.entries(settings)) {
        stmt.run(key, value);
        imported.settings++;
      }
    });
    importSettings(json.settings);
  }

  // Import connections
  if (Array.isArray(json.connections)) {
    const stmt = db.prepare(
      `INSERT OR REPLACE INTO connections (id, name, base_url, api_key, format, chat_path, models_path, auth_header, auth_prefix, extra_headers, is_active, priority, last_sync, models_count, created_at, updated_at)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
    );
    const importConns = db.transaction((connections) => {
      for (const c of connections) {
        stmt.run(
          c.id, c.name, c.base_url, c.api_key, c.format || "openai",
          c.chat_path || "/v1/chat/completions", c.models_path || "/v1/models",
          c.auth_header || "Authorization", c.auth_prefix || "Bearer ",
          c.extra_headers || "{}", c.is_active ?? 1, c.priority || 0,
          c.last_sync || null, c.models_count || 0,
          c.created_at || new Date().toISOString(), c.updated_at || new Date().toISOString()
        );
        imported.connections++;
      }
    });
    importConns(json.connections);
  }

  // Import combos (stored in settings as combos_v2)
  if (Array.isArray(json.combos) && json.combos.length > 0) {
    db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("combos_v2", JSON.stringify(json.combos));
    imported.combos = json.combos.length;
  }

  // Import aliases
  if (json.aliases && typeof json.aliases === "object") {
    db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("model_aliases", JSON.stringify(json.aliases));
    imported.aliases = true;
  }

  // Import fallback chains
  if (json.fallback_chains) {
    if (json.fallback_chains.model_chains) {
      db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("fallback_model_chains", JSON.stringify(json.fallback_chains.model_chains));
    }
    if (json.fallback_chains.connection_chains) {
      db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("fallback_connection_chains", JSON.stringify(json.fallback_chains.connection_chains));
    }
    imported.fallback_chains = true;
  }

  return imported;
}

/**
 * Create a backup of the SQLite database
 */
export function createBackup() {
  ensureBackupDir();
  const now = new Date();
  const timestamp = now.toISOString().replace(/[-:T]/g, (m) => {
    if (m === "T") return "-";
    if (m === ":") return "";
    return m;
  }).slice(0, 17); // YYYY-MM-DD-HHmmss
  const filename = `lintasan-${timestamp}.db`;
  const destPath = path.join(BACKUP_DIR, filename);

  // Checkpoint WAL to ensure DB file is up to date, then copy
  const db = getDb();
  try {
    db.pragma("wal_checkpoint(TRUNCATE)");
  } catch {
    // Non-fatal if checkpoint fails
  }
  fs.copyFileSync(DB_PATH, destPath);

  const stat = fs.statSync(destPath);
  return {
    filename,
    path: destPath,
    size: stat.size,
    created_at: now.toISOString(),
  };
}

/**
 * List available backups with size and date
 */
export function listBackups() {
  ensureBackupDir();
  const files = fs.readdirSync(BACKUP_DIR).filter((f) => f.endsWith(".db"));
  return files.map((filename) => {
    const filePath = path.join(BACKUP_DIR, filename);
    const stat = fs.statSync(filePath);
    return {
      filename,
      size: stat.size,
      created_at: stat.mtime.toISOString(),
    };
  }).sort((a, b) => b.created_at.localeCompare(a.created_at));
}

/**
 * Restore from a backup file
 * @param {string} filename - The backup filename to restore from
 */
export function restoreBackup(filename) {
  ensureBackupDir();
  // Sanitize filename to prevent path traversal
  const sanitized = path.basename(filename);
  const backupPath = path.join(BACKUP_DIR, sanitized);

  if (!fs.existsSync(backupPath)) {
    throw new Error(`Backup file not found: ${sanitized}`);
  }

  // Close current DB connection before restoring
  const db = getDb();
  db.close();

  // Copy backup over current DB
  fs.copyFileSync(backupPath, DB_PATH);

  return { restored: true, filename: sanitized, restored_at: new Date().toISOString() };
}

/**
 * Delete backups older than N days
 * @param {number} keepDays - Number of days to keep
 */
export function cleanOldBackups(keepDays = 7) {
  ensureBackupDir();
  const cutoff = Date.now() - keepDays * 24 * 60 * 60 * 1000;
  const files = fs.readdirSync(BACKUP_DIR).filter((f) => f.endsWith(".db"));
  let deleted = 0;

  for (const filename of files) {
    const filePath = path.join(BACKUP_DIR, filename);
    const stat = fs.statSync(filePath);
    if (stat.mtime.getTime() < cutoff) {
      fs.unlinkSync(filePath);
      deleted++;
    }
  }

  return { deleted, kept: files.length - deleted };
}

// Auto-backup interval reference
let backupInterval = null;

/**
 * Setup interval-based auto backup
 * Reads backup_interval_hours and backup_keep_days from settings
 */
export function scheduleBackup() {
  // Clear existing interval
  if (backupInterval) {
    clearInterval(backupInterval);
    backupInterval = null;
  }

  const intervalHours = parseInt(getSetting("backup_interval_hours", "24"), 10);
  const keepDays = parseInt(getSetting("backup_keep_days", "7"), 10);

  if (intervalHours <= 0) return { scheduled: false, reason: "backup_interval_hours is 0 or negative" };

  const intervalMs = intervalHours * 60 * 60 * 1000;

  backupInterval = setInterval(() => {
    try {
      createBackup();
      cleanOldBackups(keepDays);
    } catch (err) {
      console.error("[backup] Auto-backup failed:", err.message);
    }
  }, intervalMs);

  // Prevent interval from keeping process alive
  if (backupInterval.unref) {
    backupInterval.unref();
  }

  return {
    scheduled: true,
    interval_hours: intervalHours,
    keep_days: keepDays,
    next_backup_at: new Date(Date.now() + intervalMs).toISOString(),
  };
}
