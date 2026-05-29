// Per-provider runtime config (timeout, retries)
import { getSetting, getDb } from "./db/index.js";

const DEFAULT_TIMEOUT_MS = 30000;
const DEFAULT_MAX_RETRIES = 2;
const DEFAULT_RETRY_DELAY_MS = 1000;

export function getProviderTimeout(providerId) {
  try {
    const configJson = getSetting("provider_timeouts", "{}");
    const config = JSON.parse(configJson);
    return config[providerId] || DEFAULT_TIMEOUT_MS;
  } catch {
    return DEFAULT_TIMEOUT_MS;
  }
}

export function setProviderTimeout(providerId, timeoutMs) {
  try {
    const configJson = getSetting("provider_timeouts", "{}");
    const config = JSON.parse(configJson);
    config[providerId] = timeoutMs;
    const db = getDb();
    db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("provider_timeouts", JSON.stringify(config));
  } catch {}
}

export function getAllProviderTimeouts() {
  try {
    const configJson = getSetting("provider_timeouts", "{}");
    return JSON.parse(configJson);
  } catch {
    return {};
  }
}

export function getRetryConfig() {
  return {
    maxRetries: parseInt(getSetting("max_retries", String(DEFAULT_MAX_RETRIES))),
    retryDelayMs: parseInt(getSetting("retry_delay_ms", String(DEFAULT_RETRY_DELAY_MS))),
    retryOnStatus: [429, 500, 502, 503, 504],
  };
}

// Exponential backoff delay
export function getBackoffDelay(attempt, baseDelay) {
  return baseDelay * Math.pow(2, attempt) + Math.random() * 500;
}

// Sleep helper
export function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}
