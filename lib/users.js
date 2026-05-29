// Multi-user auth system with roles for Lintasan LLM proxy
import { randomBytes, scryptSync, timingSafeEqual } from "crypto";
import { getDb, getSetting } from "./db/index.js";

const SALT_LENGTH = 32;
const KEY_LENGTH = 64;
const SESSION_TTL_HOURS = 24;

// --- Table initialization ---

export function initUsersTable() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS users (
      id TEXT PRIMARY KEY,
      username TEXT UNIQUE NOT NULL,
      password_hash TEXT NOT NULL,
      role TEXT NOT NULL DEFAULT 'viewer' CHECK(role IN ('admin', 'editor', 'viewer')),
      email TEXT,
      is_active INTEGER DEFAULT 1,
      created_at TEXT DEFAULT (datetime('now')),
      last_login TEXT
    );

    CREATE TABLE IF NOT EXISTS sessions (
      id TEXT PRIMARY KEY,
      user_id TEXT NOT NULL,
      token TEXT UNIQUE NOT NULL,
      expires_at TEXT NOT NULL,
      created_at TEXT DEFAULT (datetime('now')),
      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
    );

    CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
    CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
    CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
    CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
  `);

  // Create default admin user if no users exist
  const count = db.prepare("SELECT COUNT(*) as cnt FROM users").get();
  if (count.cnt === 0) {
    const defaultPassword = getSetting("dashboard_password", "admin");
    createUser({
      username: "admin",
      password: defaultPassword,
      role: "admin",
      email: "admin@localhost",
    });
  }
}

// --- Password hashing ---

function hashPassword(password) {
  const salt = randomBytes(SALT_LENGTH).toString("hex");
  const hash = scryptSync(password, salt, KEY_LENGTH).toString("hex");
  return `${salt}:${hash}`;
}

function verifyPassword(password, storedHash) {
  const [salt, hash] = storedHash.split(":");
  if (!salt || !hash) return false;
  const hashBuffer = Buffer.from(hash, "hex");
  const derivedKey = scryptSync(password, salt, KEY_LENGTH);
  return timingSafeEqual(hashBuffer, derivedKey);
}

// --- Session management ---

function generateToken() {
  return randomBytes(32).toString("hex");
}

function generateId() {
  return randomBytes(16).toString("hex");
}

export function createSession(userId) {
  const db = getDb();
  const id = generateId();
  const token = generateToken();
  const expiresAt = new Date(Date.now() + SESSION_TTL_HOURS * 60 * 60 * 1000).toISOString();

  db.prepare(
    "INSERT INTO sessions (id, user_id, token, expires_at) VALUES (?, ?, ?, ?)"
  ).run(id, userId, token, expiresAt);

  return { token, expiresAt };
}

export function validateSession(token) {
  if (!token) return null;
  const db = getDb();
  const session = db.prepare(`
    SELECT s.*, u.id as user_id, u.username, u.role, u.is_active, u.email
    FROM sessions s
    JOIN users u ON s.user_id = u.id
    WHERE s.token = ? AND s.expires_at > datetime('now')
  `).get(token);

  if (!session) return null;
  if (!session.is_active) return null;

  return {
    userId: session.user_id,
    username: session.username,
    role: session.role,
    email: session.email,
  };
}

export function invalidateSession(token) {
  const db = getDb();
  db.prepare("DELETE FROM sessions WHERE token = ?").run(token);
}

export function invalidateUserSessions(userId) {
  const db = getDb();
  db.prepare("DELETE FROM sessions WHERE user_id = ?").run(userId);
}

function cleanExpiredSessions() {
  const db = getDb();
  db.prepare("DELETE FROM sessions WHERE expires_at <= datetime('now')").run();
}

// --- User CRUD ---

export function createUser({ username, password, role = "viewer", email = null }) {
  const db = getDb();

  if (!username || !password) {
    throw new Error("Username and password are required");
  }
  if (!["admin", "editor", "viewer"].includes(role)) {
    throw new Error("Invalid role. Must be admin, editor, or viewer");
  }
  if (username.length < 3 || username.length > 50) {
    throw new Error("Username must be between 3 and 50 characters");
  }
  if (password.length < 4) {
    throw new Error("Password must be at least 4 characters");
  }

  // Check if username already exists
  const existing = db.prepare("SELECT id FROM users WHERE username = ?").get(username);
  if (existing) {
    throw new Error("Username already exists");
  }

  const id = generateId();
  const passwordHash = hashPassword(password);

  db.prepare(
    "INSERT INTO users (id, username, password_hash, role, email) VALUES (?, ?, ?, ?, ?)"
  ).run(id, username, passwordHash, role, email);

  return { id, username, role, email, is_active: 1 };
}

export function authenticateUser(username, password) {
  const db = getDb();

  const user = db.prepare(
    "SELECT id, username, password_hash, role, email, is_active FROM users WHERE username = ?"
  ).get(username);

  if (!user) {
    return { success: false, error: "Invalid username or password" };
  }

  if (!user.is_active) {
    return { success: false, error: "Account is disabled" };
  }

  if (!verifyPassword(password, user.password_hash)) {
    return { success: false, error: "Invalid username or password" };
  }

  // Update last_login
  db.prepare("UPDATE users SET last_login = datetime('now') WHERE id = ?").run(user.id);

  // Create session
  const session = createSession(user.id);

  // Clean expired sessions periodically
  cleanExpiredSessions();

  return {
    success: true,
    user: { id: user.id, username: user.username, role: user.role, email: user.email },
    token: session.token,
    expiresAt: session.expiresAt,
  };
}

export function listUsers() {
  const db = getDb();
  return db.prepare(
    "SELECT id, username, role, email, is_active, created_at, last_login FROM users ORDER BY created_at ASC"
  ).all();
}

export function getUser(id) {
  const db = getDb();
  return db.prepare(
    "SELECT id, username, role, email, is_active, created_at, last_login FROM users WHERE id = ?"
  ).get(id);
}

export function updateUserRole(id, role) {
  const db = getDb();

  if (!["admin", "editor", "viewer"].includes(role)) {
    throw new Error("Invalid role. Must be admin, editor, or viewer");
  }

  const user = db.prepare("SELECT id FROM users WHERE id = ?").get(id);
  if (!user) {
    throw new Error("User not found");
  }

  db.prepare("UPDATE users SET role = ? WHERE id = ?").run(role, id);
  return { id, role };
}

export function updateUser(id, updates) {
  const db = getDb();

  const user = db.prepare("SELECT id FROM users WHERE id = ?").get(id);
  if (!user) {
    throw new Error("User not found");
  }

  const allowedFields = ["role", "email", "is_active"];
  const fields = [];
  const values = [];

  for (const [key, val] of Object.entries(updates)) {
    if (!allowedFields.includes(key)) continue;
    if (key === "role" && !["admin", "editor", "viewer"].includes(val)) {
      throw new Error("Invalid role. Must be admin, editor, or viewer");
    }
    fields.push(`${key} = ?`);
    values.push(val);
  }

  if (fields.length === 0) return getUser(id);

  values.push(id);
  db.prepare(`UPDATE users SET ${fields.join(", ")} WHERE id = ?`).run(...values);

  // If user was deactivated, invalidate their sessions
  if (updates.is_active === 0) {
    invalidateUserSessions(id);
  }

  return getUser(id);
}

export function deleteUser(id) {
  const db = getDb();

  const user = db.prepare("SELECT id, username FROM users WHERE id = ?").get(id);
  if (!user) {
    throw new Error("User not found");
  }

  // Prevent deleting the last admin
  if (user.username === "admin") {
    const adminCount = db.prepare("SELECT COUNT(*) as cnt FROM users WHERE role = 'admin'").get();
    if (adminCount.cnt <= 1) {
      throw new Error("Cannot delete the last admin user");
    }
  }

  // Remove sessions first
  invalidateUserSessions(id);
  db.prepare("DELETE FROM users WHERE id = ?").run(id);
  return { deleted: true };
}

export function changePassword(id, newPassword) {
  const db = getDb();

  if (!newPassword || newPassword.length < 4) {
    throw new Error("Password must be at least 4 characters");
  }

  const user = db.prepare("SELECT id FROM users WHERE id = ?").get(id);
  if (!user) {
    throw new Error("User not found");
  }

  const passwordHash = hashPassword(newPassword);
  db.prepare("UPDATE users SET password_hash = ? WHERE id = ?").run(passwordHash, id);

  // Invalidate all existing sessions for this user
  invalidateUserSessions(id);

  return { success: true };
}

// --- Dashboard session validation (backward compatible) ---

export function validateDashboardSessionMultiUser(request) {
  const cookie = request.headers.get("cookie") || "";

  // Check new token-based session first
  const tokenMatch = cookie.match(/sr_token=([^;]+)/);
  if (tokenMatch) {
    const session = validateSession(tokenMatch[1]);
    if (session) return session;
  }

  // Fall back to legacy sr_session (base64 password) for backward compat
  const legacyMatch = cookie.match(/sr_session=([^;]+)/);
  if (legacyMatch) {
    try {
      const decoded = Buffer.from(legacyMatch[1], "base64").toString();
      const [password] = decoded.split(":");
      const dashPassword = getSetting("dashboard_password", "admin");
      if (password === dashPassword) {
        return { userId: "legacy", username: "admin", role: "admin", email: null };
      }
    } catch {
      // Invalid base64, ignore
    }
  }

  return null;
}

// --- Role checking helpers ---

export function requireRole(session, ...roles) {
  if (!session) return false;
  return roles.includes(session.role);
}

export function canManageUsers(session) {
  return requireRole(session, "admin");
}

export function canManageConnections(session) {
  return requireRole(session, "admin", "editor");
}

export function canViewDashboard(session) {
  return requireRole(session, "admin", "editor", "viewer");
}

// Initialize on import
try {
  initUsersTable();
} catch (e) {
  // Table init may fail if DB not ready yet (e.g., during build)
  // Will be retried on first access
}
