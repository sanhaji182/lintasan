// Team/Organization system for Lintasan LLM proxy
import { getDb } from "./db/index.js";
import { randomBytes } from "crypto";

function generateId() {
  return randomBytes(16).toString("hex");
}

// --- Table initialization ---

export function initTeamsTable() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS teams (
      id TEXT PRIMARY KEY,
      name TEXT UNIQUE NOT NULL,
      description TEXT DEFAULT '',
      is_active INTEGER DEFAULT 1,
      created_at TEXT DEFAULT (datetime('now'))
    );

    CREATE TABLE IF NOT EXISTS team_members (
      team_id TEXT NOT NULL,
      user_id TEXT NOT NULL,
      role TEXT NOT NULL DEFAULT 'member' CHECK(role IN ('owner', 'member')),
      joined_at TEXT DEFAULT (datetime('now')),
      PRIMARY KEY (team_id, user_id),
      FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
    );

    CREATE INDEX IF NOT EXISTS idx_team_members_user ON team_members(user_id);
    CREATE INDEX IF NOT EXISTS idx_team_members_team ON team_members(team_id);
  `);

  // Add team_id column to api_keys if not present
  try {
    db.exec(`ALTER TABLE api_keys ADD COLUMN team_id TEXT DEFAULT NULL REFERENCES teams(id) ON DELETE SET NULL`);
  } catch (e) {
    // Column already exists — ignore
  }

  // Index for team key lookups
  try {
    db.exec(`CREATE INDEX IF NOT EXISTS idx_api_keys_team ON api_keys(team_id)`);
  } catch (e) {
    // Ignore if already exists or table not ready
  }
}

// --- Team CRUD ---

export function createTeam({ name, description = "", ownerId }) {
  const db = getDb();

  if (!name || name.length < 2 || name.length > 100) {
    throw new Error("Team name must be between 2 and 100 characters");
  }
  if (!ownerId) {
    throw new Error("Owner user ID is required");
  }

  const existing = db.prepare("SELECT id FROM teams WHERE name = ?").get(name);
  if (existing) {
    throw new Error("Team name already exists");
  }

  const id = generateId();
  db.prepare(
    "INSERT INTO teams (id, name, description) VALUES (?, ?, ?)"
  ).run(id, name, description);

  // Add creator as owner
  db.prepare(
    "INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, 'owner')"
  ).run(id, ownerId);

  return { id, name, description, is_active: 1 };
}

export function listTeams(userId = null) {
  const db = getDb();

  if (userId) {
    // Return teams the user belongs to
    return db.prepare(`
      SELECT t.*, tm.role as member_role
      FROM teams t
      JOIN team_members tm ON t.id = tm.team_id
      WHERE tm.user_id = ?
      ORDER BY t.created_at DESC
    `).all(userId);
  }

  // Admin: return all teams
  return db.prepare("SELECT * FROM teams ORDER BY created_at DESC").all();
}

export function getTeam(id) {
  const db = getDb();
  return db.prepare("SELECT * FROM teams WHERE id = ?").get(id);
}

export function updateTeam(id, updates) {
  const db = getDb();

  const team = db.prepare("SELECT id FROM teams WHERE id = ?").get(id);
  if (!team) {
    throw new Error("Team not found");
  }

  const allowedFields = ["name", "description", "is_active"];
  const fields = [];
  const values = [];

  for (const [key, val] of Object.entries(updates)) {
    if (!allowedFields.includes(key)) continue;
    if (key === "name") {
      if (!val || val.length < 2 || val.length > 100) {
        throw new Error("Team name must be between 2 and 100 characters");
      }
      const existing = db.prepare("SELECT id FROM teams WHERE name = ? AND id != ?").get(val, id);
      if (existing) {
        throw new Error("Team name already exists");
      }
    }
    fields.push(`${key} = ?`);
    values.push(val);
  }

  if (fields.length === 0) return getTeam(id);

  values.push(id);
  db.prepare(`UPDATE teams SET ${fields.join(", ")} WHERE id = ?`).run(...values);

  return getTeam(id);
}

export function deleteTeam(id) {
  const db = getDb();

  const team = db.prepare("SELECT id FROM teams WHERE id = ?").get(id);
  if (!team) {
    throw new Error("Team not found");
  }

  // Unlink team API keys (set team_id to null)
  db.prepare("UPDATE api_keys SET team_id = NULL WHERE team_id = ?").run(id);
  // Delete members and team (cascade handles members)
  db.prepare("DELETE FROM teams WHERE id = ?").run(id);

  return { deleted: true };
}

// --- Member management ---

export function addMember(teamId, userId, role = "member") {
  const db = getDb();

  if (!["owner", "member"].includes(role)) {
    throw new Error("Invalid role. Must be owner or member");
  }

  const team = db.prepare("SELECT id FROM teams WHERE id = ?").get(teamId);
  if (!team) {
    throw new Error("Team not found");
  }

  const user = db.prepare("SELECT id FROM users WHERE id = ?").get(userId);
  if (!user) {
    throw new Error("User not found");
  }

  const existing = db.prepare("SELECT team_id FROM team_members WHERE team_id = ? AND user_id = ?").get(teamId, userId);
  if (existing) {
    throw new Error("User is already a member of this team");
  }

  db.prepare(
    "INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)"
  ).run(teamId, userId, role);

  return { team_id: teamId, user_id: userId, role };
}

export function removeMember(teamId, userId) {
  const db = getDb();

  const member = db.prepare("SELECT role FROM team_members WHERE team_id = ? AND user_id = ?").get(teamId, userId);
  if (!member) {
    throw new Error("User is not a member of this team");
  }

  // Prevent removing the last owner
  if (member.role === "owner") {
    const ownerCount = db.prepare(
      "SELECT COUNT(*) as cnt FROM team_members WHERE team_id = ? AND role = 'owner'"
    ).get(teamId);
    if (ownerCount.cnt <= 1) {
      throw new Error("Cannot remove the last owner. Transfer ownership first");
    }
  }

  db.prepare("DELETE FROM team_members WHERE team_id = ? AND user_id = ?").run(teamId, userId);
  return { removed: true };
}

export function listMembers(teamId) {
  const db = getDb();
  return db.prepare(`
    SELECT tm.role, tm.joined_at, u.id, u.username, u.email, u.is_active
    FROM team_members tm
    JOIN users u ON tm.user_id = u.id
    WHERE tm.team_id = ?
    ORDER BY tm.role DESC, tm.joined_at ASC
  `).all(teamId);
}

export function isTeamMember(teamId, userId) {
  const db = getDb();
  const row = db.prepare("SELECT role FROM team_members WHERE team_id = ? AND user_id = ?").get(teamId, userId);
  return row || null;
}

export function isTeamOwner(teamId, userId) {
  const db = getDb();
  const row = db.prepare("SELECT role FROM team_members WHERE team_id = ? AND user_id = ? AND role = 'owner'").get(teamId, userId);
  return !!row;
}

// --- Team API keys ---

export function getTeamKeys(teamId) {
  const db = getDb();
  return db.prepare(
    "SELECT id, key, name, is_active, quota_rpm, quota_daily, used_today, created_at FROM api_keys WHERE team_id = ? ORDER BY created_at DESC"
  ).all(teamId);
}

// --- Team usage aggregation ---

export function getTeamUsage(teamId, period = "24h") {
  const db = getDb();

  // Determine time filter
  let timeFilter;
  switch (period) {
    case "1h":
      timeFilter = "datetime('now', '-1 hour')";
      break;
    case "24h":
      timeFilter = "datetime('now', '-1 day')";
      break;
    case "7d":
      timeFilter = "datetime('now', '-7 days')";
      break;
    case "30d":
      timeFilter = "datetime('now', '-30 days')";
      break;
    default:
      timeFilter = "datetime('now', '-1 day')";
  }

  // Get all team key IDs
  const teamKeys = db.prepare("SELECT id FROM api_keys WHERE team_id = ?").all(teamId);
  if (teamKeys.length === 0) {
    return { total_requests: 0, total_input_tokens: 0, total_output_tokens: 0, total_errors: 0, keys_count: 0 };
  }

  const keyIds = teamKeys.map((k) => k.id);
  const placeholders = keyIds.map(() => "?").join(",");

  // Aggregate usage from request_logs where the api_key_id matches team keys
  // request_logs may not have api_key_id column — fall back to counting team keys' used_today
  try {
    const usage = db.prepare(`
      SELECT
        COUNT(*) as total_requests,
        COALESCE(SUM(input_tokens), 0) as total_input_tokens,
        COALESCE(SUM(output_tokens), 0) as total_output_tokens,
        COALESCE(SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END), 0) as total_errors
      FROM request_logs
      WHERE api_key_id IN (${placeholders})
        AND created_at >= ${timeFilter}
    `).get(...keyIds);

    return { ...usage, keys_count: keyIds.length };
  } catch (e) {
    // If api_key_id column doesn't exist in request_logs, use daily counters
    const dailyUsage = db.prepare(`
      SELECT
        COALESCE(SUM(used_today), 0) as total_requests_today
      FROM api_keys
      WHERE team_id = ?
    `).get(teamId);

    return {
      total_requests: dailyUsage.total_requests_today,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_errors: 0,
      keys_count: keyIds.length,
      note: "Detailed usage tracking requires api_key_id in request_logs"
    };
  }
}

// Initialize on import
try {
  initTeamsTable();
} catch (e) {
  // Table init may fail if DB not ready yet (e.g., during build)
}
