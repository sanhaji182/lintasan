// Enhanced Rate Limiter for Lintasan LLM Proxy
// Implements: sliding window, per-key limits, burst allowance, IP-based limiting
import { getSetting } from "./db/index.js";

// === SLIDING WINDOW STORAGE ===
// Each entry stores an array of timestamps for requests within the window
const keyWindows = new Map();   // apiKey -> { timestamps: number[] }
const ipWindows = new Map();    // ip -> { timestamps: number[] }

// Burst tracking: tracks if a key is in burst mode
const burstState = new Map();   // apiKey -> { burstStart: number }

// Cleanup interval - prune stale entries every 5 minutes
const CLEANUP_INTERVAL_MS = 5 * 60_000;
let lastCleanup = Date.now();

/**
 * Remove timestamps older than the window from an array (in-place).
 * Returns the pruned array.
 */
function pruneTimestamps(timestamps, windowMs, now) {
  const cutoff = now - windowMs;
  // Find first index that's within the window
  let i = 0;
  while (i < timestamps.length && timestamps[i] <= cutoff) {
    i++;
  }
  if (i > 0) {
    timestamps.splice(0, i);
  }
  return timestamps;
}

/**
 * Periodic cleanup of stale entries from Maps to prevent memory leaks.
 */
function maybeCleanup(now) {
  if (now - lastCleanup < CLEANUP_INTERVAL_MS) return;
  lastCleanup = now;

  const windowMs = 60_000;
  const cutoff = now - windowMs;

  for (const [key, entry] of keyWindows) {
    if (entry.timestamps.length === 0 || entry.timestamps[entry.timestamps.length - 1] <= cutoff) {
      keyWindows.delete(key);
    }
  }

  for (const [ip, entry] of ipWindows) {
    if (entry.timestamps.length === 0 || entry.timestamps[entry.timestamps.length - 1] <= cutoff) {
      ipWindows.delete(ip);
    }
  }

  for (const [key, state] of burstState) {
    if (now - state.burstStart > 5000) {
      burstState.delete(key);
    }
  }
}

/**
 * Get the effective rate limit for a key, considering burst allowance.
 * Burst allows 2x the normal limit for a 5-second window.
 */
function getEffectiveLimit(apiKey, baseRpm, now) {
  const burstMultiplier = parseFloat(getSetting("rate_limit_burst_multiplier", "2"));
  const burstWindowMs = parseInt(getSetting("rate_limit_burst_window_ms", "5000"));

  const state = burstState.get(apiKey);

  if (state && (now - state.burstStart) <= burstWindowMs) {
    // Currently in burst mode
    return Math.floor(baseRpm * burstMultiplier);
  }

  return baseRpm;
}

/**
 * Count requests in the sliding window and determine if a new request is allowed.
 * Uses a sliding window: counts all requests in the last 60 seconds.
 */
function slidingWindowCheck(windowMap, key, limit, now) {
  const windowMs = 60_000;

  let entry = windowMap.get(key);
  if (!entry) {
    entry = { timestamps: [] };
    windowMap.set(key, entry);
  }

  // Prune old timestamps outside the window
  pruneTimestamps(entry.timestamps, windowMs, now);

  const currentCount = entry.timestamps.length;

  if (currentCount >= limit) {
    // Calculate when the oldest request in the window will expire
    const oldestInWindow = entry.timestamps[0];
    const resetIn = Math.ceil((oldestInWindow + windowMs - now) / 1000);
    const retryAfter = Math.max(1, resetIn);

    return {
      allowed: false,
      remaining: 0,
      resetIn: Math.max(1, resetIn),
      retryAfter,
    };
  }

  // Request is allowed - record the timestamp
  entry.timestamps.push(now);

  return {
    allowed: true,
    remaining: limit - currentCount - 1,
    resetIn: 60,
    retryAfter: null,
  };
}

/**
 * Activate burst mode for a key. Called when the key is approaching its limit
 * (over 80% usage) to allow a temporary burst.
 */
function maybeActivateBurst(apiKey, currentCount, baseRpm, now) {
  const threshold = Math.floor(baseRpm * 0.8);

  if (currentCount >= threshold && !burstState.has(apiKey)) {
    burstState.set(apiKey, { burstStart: now });
  }
}

/**
 * Main rate limit check. Combines per-key sliding window and IP-based limiting.
 *
 * @param {string} apiKey - The API key making the request
 * @param {string} ip - The client IP address
 * @param {number|null} keyQuotaRpm - Per-key RPM limit from api_keys table (null = use global default)
 * @returns {{allowed: boolean, remaining: number, resetIn: number, retryAfter: number|null}}
 */
export function checkRateLimitAdvanced(apiKey, ip, keyQuotaRpm) {
  const now = Date.now();

  // Run periodic cleanup
  maybeCleanup(now);

  // Determine the base RPM for this key
  const globalRpm = parseInt(getSetting("rate_limit_rpm", "60"));
  const baseRpm = (keyQuotaRpm != null && keyQuotaRpm > 0) ? keyQuotaRpm : globalRpm;

  // If rate limiting is disabled (limit <= 0), allow everything
  if (baseRpm <= 0) {
    return { allowed: true, remaining: Infinity, resetIn: 0, retryAfter: null };
  }

  // === IP-based rate limiting (secondary protection) ===
  const ipLimitPerMin = parseInt(getSetting("rate_limit_ip_rpm", "100"));

  if (ip && ipLimitPerMin > 0) {
    const ipResult = slidingWindowCheck(ipWindows, ip, ipLimitPerMin, now);
    if (!ipResult.allowed) {
      return {
        allowed: false,
        remaining: 0,
        resetIn: ipResult.resetIn,
        retryAfter: ipResult.retryAfter,
      };
    }
  }

  // === Per-key sliding window check ===
  const keyId = apiKey.slice(0, 16); // Use prefix as key identifier

  // Check current usage to potentially activate burst
  let entry = keyWindows.get(keyId);
  if (entry) {
    pruneTimestamps(entry.timestamps, 60_000, now);
    maybeActivateBurst(keyId, entry.timestamps.length, baseRpm, now);
  }

  // Get effective limit (may be elevated during burst)
  const effectiveLimit = getEffectiveLimit(keyId, baseRpm, now);

  // Perform the sliding window check for the API key
  const keyResult = slidingWindowCheck(keyWindows, keyId, effectiveLimit, now);

  return {
    allowed: keyResult.allowed,
    remaining: keyResult.remaining,
    resetIn: keyResult.resetIn,
    retryAfter: keyResult.retryAfter,
  };
}

/**
 * Reset rate limit state for a specific key (useful for testing or admin actions).
 */
export function resetRateLimit(apiKey) {
  const keyId = apiKey.slice(0, 16);
  keyWindows.delete(keyId);
  burstState.delete(keyId);
}

/**
 * Reset rate limit state for a specific IP.
 */
export function resetIpRateLimit(ip) {
  ipWindows.delete(ip);
}

/**
 * Get current usage stats for a key (for monitoring/admin).
 */
export function getRateLimitStats(apiKey) {
  const now = Date.now();
  const keyId = apiKey.slice(0, 16);
  const entry = keyWindows.get(keyId);

  if (!entry) {
    return { requestsInWindow: 0, windowMs: 60_000, burstActive: false };
  }

  pruneTimestamps(entry.timestamps, 60_000, now);
  const burst = burstState.get(keyId);
  const burstActive = burst ? (now - burst.burstStart <= 5000) : false;

  return {
    requestsInWindow: entry.timestamps.length,
    windowMs: 60_000,
    burstActive,
  };
}
