/**
 * Plugin Store — curated catalog of ready-to-install plugin templates.
 * Each entry is a complete, working ES module plugin.
 */

const STORE_PLUGINS = [
  {
    id: "rate-limit-logger",
    name: "Rate Limit Logger",
    description: "Log all 429 rate limit events to a file with timestamps",
    category: "monitoring",
    author: "Lintasan",
    version: "1.0.0",
    tags: ["rate-limit", "logging", "429"],
    code: `import { appendFileSync } from "fs";
import { resolve } from "path";

const LOG_FILE = resolve(process.cwd(), "logs/rate-limits.log");

export default {
  name: "rate-limit-logger",
  version: "1.0.0",
  description: "Log all 429 rate limit events to a file with timestamps",
  priority: 5,
  enabled: true,
  hooks: {
    onError(ctx, error) {
      if (error?.status === 429 || error?.statusCode === 429) {
        const entry = \`[\${new Date().toISOString()}] 429 Rate Limited | model=\${ctx.model} | key=\${ctx.auth?.slice(0, 8)}... | provider=\${ctx.provider || "unknown"}\\n\`;
        try {
          appendFileSync(LOG_FILE, entry);
        } catch (e) {
          console.error("[rate-limit-logger] Failed to write log:", e.message);
        }
        console.warn("[rate-limit-logger]", entry.trim());
      }
    }
  }
};
`,
  },
  {
    id: "response-censor",
    name: "Response Censor",
    description: "Mask sensitive data (emails, API keys, phone numbers) in responses",
    category: "security",
    author: "Lintasan",
    version: "1.0.0",
    tags: ["security", "pii", "masking", "privacy"],
    code: `const PATTERNS = [
  { regex: /[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}/g, mask: "[EMAIL_REDACTED]" },
  { regex: /\\b(sk-[a-zA-Z0-9]{20,})\\b/g, mask: "[API_KEY_REDACTED]" },
  { regex: /\\b(key-[a-zA-Z0-9]{20,})\\b/g, mask: "[API_KEY_REDACTED]" },
  { regex: /\\b\\d{3}[-.]?\\d{3}[-.]?\\d{4}\\b/g, mask: "[PHONE_REDACTED]" },
];

function censorText(text) {
  if (typeof text !== "string") return text;
  let result = text;
  for (const { regex, mask } of PATTERNS) {
    result = result.replace(regex, mask);
  }
  return result;
}

function censorObject(obj) {
  if (!obj || typeof obj !== "object") return obj;
  if (Array.isArray(obj)) return obj.map(censorObject);
  const result = {};
  for (const [key, value] of Object.entries(obj)) {
    if (typeof value === "string") {
      result[key] = censorText(value);
    } else if (typeof value === "object") {
      result[key] = censorObject(value);
    } else {
      result[key] = value;
    }
  }
  return result;
}

export default {
  name: "response-censor",
  version: "1.0.0",
  description: "Mask sensitive data (emails, API keys, phone numbers) in responses",
  priority: 90,
  enabled: true,
  hooks: {
    afterRequest(ctx, response) {
      return censorObject(response);
    },
    onStream(ctx, chunk) {
      if (typeof chunk === "string") return censorText(chunk);
      return censorObject(chunk);
    }
  }
};
`,
  },
  {
    id: "latency-alert",
    name: "Latency Alert",
    description: "Trigger console warning when latency exceeds configurable threshold (default 5000ms)",
    category: "monitoring",
    author: "Lintasan",
    version: "1.0.0",
    tags: ["latency", "alerting", "performance"],
    code: `const THRESHOLD_MS = parseInt(process.env.LATENCY_ALERT_MS || "5000", 10);

const requestTimers = new Map();

export default {
  name: "latency-alert",
  version: "1.0.0",
  description: "Trigger console warning when latency exceeds threshold",
  priority: 1,
  enabled: true,
  hooks: {
    beforeRequest(ctx) {
      const id = ctx.requestId || Math.random().toString(36).slice(2);
      ctx._latencyAlertId = id;
      requestTimers.set(id, Date.now());
    },
    afterRequest(ctx, response) {
      const id = ctx._latencyAlertId;
      if (id && requestTimers.has(id)) {
        const elapsed = Date.now() - requestTimers.get(id);
        requestTimers.delete(id);
        if (elapsed > THRESHOLD_MS) {
          console.warn(\`[latency-alert] ⚠️  High latency: \${elapsed}ms (threshold: \${THRESHOLD_MS}ms) | model=\${ctx.model}\`);
        }
      }
      return response;
    }
  }
};
`,
  },
  {
    id: "model-override",
    name: "Model Override",
    description: "Force specific model for specific API keys (configurable mapping)",
    category: "utility",
    author: "Lintasan",
    version: "1.0.0",
    tags: ["model", "routing", "override", "keys"],
    code: `/**
 * Configure model overrides per API key prefix.
 * Edit this mapping to force specific models for specific keys.
 */
const MODEL_MAP = {
  // "sk-test": "gpt-4o-mini",
  // "sk-prod": "gpt-4o",
};

export default {
  name: "model-override",
  version: "1.0.0",
  description: "Force specific model for specific API keys",
  priority: 10,
  enabled: true,
  hooks: {
    beforeRequest(ctx) {
      const key = ctx.auth || "";
      for (const [prefix, model] of Object.entries(MODEL_MAP)) {
        if (key.startsWith(prefix)) {
          console.log(\`[model-override] Overriding model from \${ctx.model} to \${model} for key \${prefix}...\`);
          ctx.model = model;
          break;
        }
      }
    }
  }
};
`,
  },
  {
    id: "cost-cap",
    name: "Cost Cap",
    description: "Hard stop requests when daily cost exceeds configurable limit",
    category: "security",
    author: "Lintasan",
    version: "1.0.0",
    tags: ["cost", "budget", "limit", "billing"],
    code: `const DAILY_LIMIT_USD = parseFloat(process.env.COST_CAP_DAILY || "50");

let dailyCost = 0;
let lastReset = new Date().toDateString();

// Rough cost estimates per 1K tokens (input/output averaged)
const COST_PER_1K = {
  "gpt-4o": 0.005,
  "gpt-4o-mini": 0.00015,
  "gpt-4-turbo": 0.01,
  "gpt-3.5-turbo": 0.0005,
  "claude-3-opus": 0.015,
  "claude-3-sonnet": 0.003,
  "claude-3-haiku": 0.00025,
  default: 0.002,
};

function estimateCost(model, tokens) {
  const rate = COST_PER_1K[model] || COST_PER_1K.default;
  return (tokens / 1000) * rate;
}

export default {
  name: "cost-cap",
  version: "1.0.0",
  description: "Hard stop requests when daily cost exceeds limit",
  priority: 2,
  enabled: true,
  hooks: {
    beforeRequest(ctx) {
      const today = new Date().toDateString();
      if (today !== lastReset) {
        dailyCost = 0;
        lastReset = today;
      }
      if (dailyCost >= DAILY_LIMIT_USD) {
        console.error(\`[cost-cap] Daily limit of $\${DAILY_LIMIT_USD} exceeded ($\${dailyCost.toFixed(4)}). Blocking request.\`);
        return {
          __shortCircuit: true,
          response: new Response(
            JSON.stringify({ error: { message: "Daily cost limit exceeded. Try again tomorrow.", type: "cost_cap_exceeded" } }),
            { status: 429, headers: { "Content-Type": "application/json" } }
          )
        };
      }
    },
    afterRequest(ctx, response) {
      const usage = response?.usage;
      if (usage) {
        const totalTokens = (usage.prompt_tokens || 0) + (usage.completion_tokens || 0);
        dailyCost += estimateCost(ctx.model, totalTokens);
      }
      return response;
    }
  }
};
`,
  },
  {
    id: "request-validator",
    name: "Request Validator",
    description: "Validate request schema (require messages array, valid model name)",
    category: "security",
    author: "Lintasan",
    version: "1.0.0",
    tags: ["validation", "schema", "security"],
    code: `const VALID_MODEL_PATTERNS = [
  /^gpt-/,
  /^claude-/,
  /^gemini-/,
  /^mistral-/,
  /^llama-/,
  /^command-/,
];

export default {
  name: "request-validator",
  version: "1.0.0",
  description: "Validate request schema before forwarding",
  priority: 3,
  enabled: true,
  hooks: {
    beforeRequest(ctx) {
      const errors = [];

      // Validate messages
      if (!ctx.messages || !Array.isArray(ctx.messages)) {
        errors.push("messages must be a non-empty array");
      } else if (ctx.messages.length === 0) {
        errors.push("messages array cannot be empty");
      } else {
        for (let i = 0; i < ctx.messages.length; i++) {
          const msg = ctx.messages[i];
          if (!msg.role || !["system", "user", "assistant", "tool", "function"].includes(msg.role)) {
            errors.push(\`messages[\${i}].role is invalid: \${msg.role}\`);
          }
          if (msg.content === undefined && !msg.tool_calls && !msg.function_call) {
            errors.push(\`messages[\${i}] must have content, tool_calls, or function_call\`);
          }
        }
      }

      // Validate model name
      if (!ctx.model || typeof ctx.model !== "string") {
        errors.push("model is required and must be a string");
      } else {
        const isValid = VALID_MODEL_PATTERNS.some(p => p.test(ctx.model));
        if (!isValid) {
          errors.push(\`model "\${ctx.model}" does not match any known provider pattern\`);
        }
      }

      if (errors.length > 0) {
        return {
          __shortCircuit: true,
          response: new Response(
            JSON.stringify({ error: { message: "Request validation failed", details: errors, type: "validation_error" } }),
            { status: 400, headers: { "Content-Type": "application/json" } }
          )
        };
      }
    }
  }
};
`,
  },
  {
    id: "token-counter",
    name: "Token Counter",
    description: "Add X-Token-Count header to responses with input/output/total tokens",
    category: "monitoring",
    author: "Lintasan",
    version: "1.0.0",
    tags: ["tokens", "usage", "headers", "metrics"],
    code: `export default {
  name: "token-counter",
  version: "1.0.0",
  description: "Add X-Token-Count header to responses with token usage",
  priority: 95,
  enabled: true,
  hooks: {
    afterRequest(ctx, response) {
      const usage = response?.usage;
      if (usage) {
        const input = usage.prompt_tokens || 0;
        const output = usage.completion_tokens || 0;
        const total = usage.total_tokens || (input + output);
        // Store token info on context for header injection
        ctx._tokenCount = { input, output, total };
        ctx._responseHeaders = {
          ...(ctx._responseHeaders || {}),
          "X-Token-Count": \`input=\${input}, output=\${output}, total=\${total}\`,
          "X-Token-Input": String(input),
          "X-Token-Output": String(output),
          "X-Token-Total": String(total),
        };
      }
      return response;
    }
  }
};
`,
  },
  {
    id: "retry-enhancer",
    name: "Retry Enhancer",
    description: "Custom retry logic with per-model retry counts and delays",
    category: "optimization",
    author: "Lintasan",
    version: "1.0.0",
    tags: ["retry", "resilience", "reliability"],
    code: `/**
 * Per-model retry configuration.
 * Edit retries and delay (ms) per model pattern.
 */
const RETRY_CONFIG = {
  "gpt-4o": { retries: 3, delay: 1000 },
  "gpt-4-turbo": { retries: 2, delay: 2000 },
  "gpt-3.5-turbo": { retries: 4, delay: 500 },
  "claude-3-opus": { retries: 2, delay: 3000 },
  "claude-3-sonnet": { retries: 3, delay: 1500 },
  default: { retries: 2, delay: 1000 },
};

function getConfig(model) {
  for (const [key, config] of Object.entries(RETRY_CONFIG)) {
    if (key !== "default" && model?.includes(key)) return config;
  }
  return RETRY_CONFIG.default;
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

export default {
  name: "retry-enhancer",
  version: "1.0.0",
  description: "Custom retry logic with per-model retry counts and delays",
  priority: 5,
  enabled: true,
  hooks: {
    async onError(ctx, error) {
      const status = error?.status || error?.statusCode || 500;
      // Only retry on 429, 500, 502, 503, 504
      if (![429, 500, 502, 503, 504].includes(status)) return;

      const config = getConfig(ctx.model);
      const attempt = (ctx._retryAttempt || 0) + 1;

      if (attempt > config.retries) {
        console.warn(\`[retry-enhancer] Max retries (\${config.retries}) reached for \${ctx.model}\`);
        return;
      }

      ctx._retryAttempt = attempt;
      const delay = config.delay * attempt; // exponential-ish backoff
      console.log(\`[retry-enhancer] Retry \${attempt}/\${config.retries} for \${ctx.model} in \${delay}ms\`);
      await sleep(delay);

      // Signal retry by setting flag on context
      ctx._shouldRetry = true;
    }
  }
};
`,
  },
  {
    id: "auto-summarize",
    name: "Auto Summarize",
    description: "Automatically summarize long conversations (>10 messages) before forwarding",
    category: "optimization",
    author: "Lintasan",
    version: "1.0.0",
    tags: ["summarize", "context", "optimization", "tokens"],
    code: `const MAX_MESSAGES = parseInt(process.env.AUTO_SUMMARIZE_THRESHOLD || "10", 10);

function summarizeMessages(messages) {
  // Keep system message if present
  const system = messages.find(m => m.role === "system");
  const nonSystem = messages.filter(m => m.role !== "system");

  // Keep last 4 messages as-is for context
  const recent = nonSystem.slice(-4);
  const older = nonSystem.slice(0, -4);

  if (older.length === 0) return messages;

  // Create a summary of older messages
  const summaryParts = older.map(m => \`\${m.role}: \${typeof m.content === "string" ? m.content.slice(0, 100) : "[complex content]"}\`);
  const summaryText = \`[Conversation summary - \${older.length} earlier messages]:\\n\${summaryParts.join("\\n")}\`;

  const result = [];
  if (system) result.push(system);
  result.push({ role: "system", content: summaryText });
  result.push(...recent);

  console.log(\`[auto-summarize] Condensed \${messages.length} messages to \${result.length} (summarized \${older.length} older messages)\`);
  return result;
}

export default {
  name: "auto-summarize",
  version: "1.0.0",
  description: "Summarize long conversations before forwarding",
  priority: 20,
  enabled: true,
  hooks: {
    beforeRequest(ctx) {
      if (ctx.messages && ctx.messages.length > MAX_MESSAGES) {
        ctx.messages = summarizeMessages(ctx.messages);
      }
    }
  }
};
`,
  },
  {
    id: "usage-reporter",
    name: "Usage Reporter",
    description: "Collect hourly usage stats and log summary to console every hour",
    category: "monitoring",
    author: "Lintasan",
    version: "1.0.0",
    tags: ["usage", "stats", "reporting", "metrics"],
    code: `let stats = {
  requests: 0,
  totalTokens: 0,
  totalInputTokens: 0,
  totalOutputTokens: 0,
  models: {},
  errors: 0,
  startTime: Date.now(),
};

function resetStats() {
  const report = { ...stats, duration: Date.now() - stats.startTime };
  stats = {
    requests: 0,
    totalTokens: 0,
    totalInputTokens: 0,
    totalOutputTokens: 0,
    models: {},
    errors: 0,
    startTime: Date.now(),
  };
  return report;
}

// Log summary every hour
setInterval(() => {
  const report = resetStats();
  if (report.requests > 0) {
    console.log("[usage-reporter] ═══════════════════════════════════════");
    console.log(\`[usage-reporter] Hourly Summary:\`);
    console.log(\`[usage-reporter]   Requests: \${report.requests}\`);
    console.log(\`[usage-reporter]   Tokens: \${report.totalTokens} (in: \${report.totalInputTokens}, out: \${report.totalOutputTokens})\`);
    console.log(\`[usage-reporter]   Errors: \${report.errors}\`);
    console.log(\`[usage-reporter]   Models: \${JSON.stringify(report.models)}\`);
    console.log("[usage-reporter] ═══════════════════════════════════════");
  }
}, 60 * 60 * 1000);

export default {
  name: "usage-reporter",
  version: "1.0.0",
  description: "Collect hourly usage stats and log summary",
  priority: 99,
  enabled: true,
  hooks: {
    beforeRequest(ctx) {
      stats.requests++;
      stats.models[ctx.model] = (stats.models[ctx.model] || 0) + 1;
    },
    afterRequest(ctx, response) {
      const usage = response?.usage;
      if (usage) {
        stats.totalInputTokens += usage.prompt_tokens || 0;
        stats.totalOutputTokens += usage.completion_tokens || 0;
        stats.totalTokens += usage.total_tokens || ((usage.prompt_tokens || 0) + (usage.completion_tokens || 0));
      }
      return response;
    },
    onError(ctx, error) {
      stats.errors++;
    }
  }
};
`,
  },
];

/**
 * Get all store plugins, optionally filtered by category.
 */
export function getStorePlugins(category) {
  if (category) {
    return STORE_PLUGINS.filter((p) => p.category === category);
  }
  return STORE_PLUGINS;
}

/**
 * Get a single store plugin by ID.
 */
export function getStorePlugin(id) {
  return STORE_PLUGINS.find((p) => p.id === id) || null;
}

/**
 * Get all available categories.
 */
export function getStoreCategories() {
  return ["security", "monitoring", "optimization", "utility", "integration"];
}
