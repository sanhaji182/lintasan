// Auth middleware for Lintasan
import { getSetting } from "./db/index.js";
import { validateUserApiKey } from "./api-keys.js";

const DEFAULT_MASTER_KEY = "sr-admin-key-change-me";

export function getMasterKey() {
  return getSetting("master_key", DEFAULT_MASTER_KEY);
}

export function getDashboardPassword() {
  return getSetting("dashboard_password", "admin");
}

// Validate API key - supports master key + user keys
export function validateApiKey(request) {
  const authHeader = request.headers.get("authorization");
  if (!authHeader) return { valid: false };
  const token = authHeader.replace("Bearer ", "").trim();

  // Check master key first
  const masterKey = getMasterKey();
  if (token === masterKey) {
    return { valid: true, token, isMaster: true, name: "master" };
  }

  // Check user API keys
  const userCheck = validateUserApiKey(token);
  if (userCheck.valid) {
    return { valid: true, token, isMaster: false, name: userCheck.name, keyId: userCheck.keyId, quotaRpm: userCheck.quotaRpm, remaining: userCheck.remaining };
  }

  return { valid: false, reason: userCheck.reason };
}

export function validateDashboardSession(request) {
  const cookie = request.headers.get("cookie") || "";
  const match = cookie.match(/sr_session=([^;]+)/);
  if (!match) return false;
  try {
    const decoded = Buffer.from(match[1], "base64").toString();
    const [password] = decoded.split(":");
    return password === getDashboardPassword();
  } catch {
    return false;
  }
}
