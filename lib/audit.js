// Admin Audit Log - track dashboard actions
import { getDb } from "./db/index.js";

function initAudit() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS audit_log (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      action TEXT NOT NULL,
      actor TEXT DEFAULT 'admin',
      details TEXT,
      ip TEXT,
      created_at TEXT DEFAULT (datetime('now'))
    );
    CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at);
  `);
}

export function logAudit(action, details = null, actor = "admin", ip = null) {
  try {
    initAudit();
    const db = getDb();
    db.prepare(
      "INSERT INTO audit_log (action, actor, details, ip) VALUES (?, ?, ?, ?)"
    ).run(action, actor, typeof details === "string" ? details : JSON.stringify(details), ip);
  } catch {}
}

export function getAuditLog(limit = 100) {
  try {
    initAudit();
    const db = getDb();
    return db.prepare("SELECT * FROM audit_log ORDER BY created_at DESC LIMIT ?").all(limit);
  } catch {
    return [];
  }
}

export function clearAuditLog(olderThanDays = 30) {
  try {
    initAudit();
    const db = getDb();
    return db.prepare("DELETE FROM audit_log WHERE created_at < datetime('now', '-' || ? || ' days')").run(olderThanDays);
  } catch {
    return { changes: 0 };
  }
}
