// Circuit Breaker — auto-disable providers after consecutive failures
// States: CLOSED (normal) → OPEN (blocked) → HALF_OPEN (testing)
import { getSetting } from "./db/index.js";

const breakers = new Map();

function getCircuitConfig() {
  return {
    enabled: getSetting("circuit_breaker_enabled", "true") === "true",
    // Number of consecutive failures before opening circuit
    failureThreshold: parseInt(getSetting("circuit_breaker_threshold", "3")),
    // How long to wait before trying again (ms)
    cooldownMs: parseInt(getSetting("circuit_breaker_cooldown", "30000")),
    // How many test requests in half-open state
    halfOpenMax: parseInt(getSetting("circuit_breaker_half_open", "2")),
  };
}

function getBreaker(providerId) {
  if (!breakers.has(providerId)) {
    breakers.set(providerId, {
      state: "CLOSED", // CLOSED, OPEN, HALF_OPEN
      failures: 0,
      successes: 0,
      lastFailure: 0,
      openedAt: 0,
    });
  }
  return breakers.get(providerId);
}

// Check if provider is available
export function isProviderAvailable(providerId) {
  const config = getCircuitConfig();
  if (!config.enabled) return true;

  const breaker = getBreaker(providerId);

  switch (breaker.state) {
    case "CLOSED":
      return true;

    case "OPEN": {
      // Check if cooldown has passed
      const elapsed = Date.now() - breaker.openedAt;
      if (elapsed >= config.cooldownMs) {
        // Transition to HALF_OPEN
        breaker.state = "HALF_OPEN";
        breaker.successes = 0;
        return true;
      }
      return false;
    }

    case "HALF_OPEN":
      return true;

    default:
      return true;
  }
}

// Record a successful request
export function recordSuccess(providerId) {
  const config = getCircuitConfig();
  if (!config.enabled) return;

  const breaker = getBreaker(providerId);

  switch (breaker.state) {
    case "HALF_OPEN":
      breaker.successes++;
      if (breaker.successes >= config.halfOpenMax) {
        // Recovered — close circuit
        breaker.state = "CLOSED";
        breaker.failures = 0;
        breaker.successes = 0;
      }
      break;

    case "CLOSED":
      // Reset failure count on success
      breaker.failures = 0;
      break;
  }
}

// Record a failed request
export function recordFailure(providerId) {
  const config = getCircuitConfig();
  if (!config.enabled) return;

  const breaker = getBreaker(providerId);

  breaker.failures++;
  breaker.lastFailure = Date.now();

  switch (breaker.state) {
    case "CLOSED":
      if (breaker.failures >= config.failureThreshold) {
        breaker.state = "OPEN";
        breaker.openedAt = Date.now();
      }
      break;

    case "HALF_OPEN":
      // Failed during test — reopen
      breaker.state = "OPEN";
      breaker.openedAt = Date.now();
      breaker.successes = 0;
      break;
  }
}

// Get status of all breakers (for dashboard)
export function getCircuitStatus() {
  const status = {};
  for (const [id, breaker] of breakers) {
    status[id] = {
      state: breaker.state,
      failures: breaker.failures,
      lastFailure: breaker.lastFailure ? new Date(breaker.lastFailure).toISOString() : null,
    };
  }
  return status;
}

// Force reset a breaker (manual override)
export function resetBreaker(providerId) {
  breakers.set(providerId, {
    state: "CLOSED",
    failures: 0,
    successes: 0,
    lastFailure: 0,
    openedAt: 0,
  });
}
