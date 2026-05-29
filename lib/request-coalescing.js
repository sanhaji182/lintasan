// Request Coalescing — dedup concurrent identical requests
// If multiple identical requests arrive within a short window,
// only send one to the LLM and fan out the response to all waiters
import crypto from "crypto";

const inflight = new Map(); // hash -> { promise, waiters }
const COALESCE_WINDOW_MS = 2000; // 2 second window

// Generate hash from request body (model + messages + key params)
function requestHash(model, messages, params = {}) {
  const key = JSON.stringify({
    model,
    messages,
    temperature: params.temperature,
    max_tokens: params.max_tokens,
    top_p: params.top_p,
  });
  return crypto.createHash("sha256").update(key).digest("hex").slice(0, 16);
}

// Try to coalesce a request. Returns cached promise if duplicate is in-flight.
// Otherwise returns null and caller should execute the request.
export function tryCoalesce(model, messages, params = {}) {
  const hash = requestHash(model, messages, params);

  if (inflight.has(hash)) {
    const entry = inflight.get(hash);
    entry.waiters++;
    return { coalesced: true, promise: entry.promise, hash };
  }

  return { coalesced: false, hash };
}

// Register an in-flight request so others can coalesce onto it
export function registerInflight(hash, promise) {
  inflight.set(hash, { promise, waiters: 1, createdAt: Date.now() });

  // Auto-cleanup after resolution
  promise
    .then(() => {
      setTimeout(() => inflight.delete(hash), 100);
    })
    .catch(() => {
      inflight.delete(hash);
    });

  // Safety cleanup after timeout
  setTimeout(() => {
    inflight.delete(hash);
  }, 60000);
}

// Get coalescing stats
export function getCoalesceStats() {
  return {
    inflightRequests: inflight.size,
    entries: Array.from(inflight.entries()).map(([hash, entry]) => ({
      hash,
      waiters: entry.waiters,
      age: Date.now() - entry.createdAt,
    })),
  };
}
