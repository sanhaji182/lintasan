import http from "http";
import path from "path";
import { execSync } from "child_process";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.join(__dirname, "..");
const LINTASAN_PORT = process.env.PORT || 20180;

// Resolve master key once at startup
let _masterKey = process.env.LINTASAN_MASTER_KEY || "";
if (!_masterKey) {
  try {
    const dbPath = path.join(ROOT, "data", "lintasan.db");
    // Try different quoting for cross-platform compat
    try {
      _masterKey = execSync(`sqlite3 '${dbPath}' 'SELECT value FROM settings WHERE key="master_key"'`, { encoding: "utf8" }).trim();
    } catch {
      _masterKey = execSync(`sqlite3 "${dbPath}" "SELECT value FROM settings WHERE key='master_key'"`, { encoding: "utf8" }).trim();
    }
  } catch {}
}
if (_masterKey) {
  console.log(`  🔑 MITM using key: ${_masterKey.substring(0, 8)}...`);
} else {
  console.log("  ⚠  No master key found. Set LINTASAN_MASTER_KEY env var.");
}

// Tool detection based on host + URL pattern
const TOOL_PATTERNS = {
  copilot: {
    hosts: ["api.individual.githubcopilot.com"],
    paths: ["/chat/completions", "/v1/messages", "/responses"],
  },
  kiro: {
    hosts: ["q.us-east-1.amazonaws.com"],
    paths: ["/generateAssistantResponse"],
  },
  cursor: {
    hosts: ["api2.cursor.sh"],
    paths: ["/BidiAppend", "/RunSSE", "/RunPoll", "/Run"],
  },
  gemini: {
    hosts: ["cloudcode-pa.googleapis.com", "daily-cloudcode-pa.googleapis.com"],
    paths: [":generateContent", ":streamGenerateContent"],
  },
};

function detectTool(host, url) {
  for (const [tool, config] of Object.entries(TOOL_PATTERNS)) {
    if (!config.hosts.includes(host)) continue;
    for (const p of config.paths) {
      if (url.includes(p)) return tool;
    }
  }
  return null;
}

// Collect request body
function collectBody(req) {
  return new Promise((resolve) => {
    const chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => resolve(Buffer.concat(chunks)));
  });
}

// Forward to Lintasan's /api/v1/chat/completions
function forwardToLintasan(model, messages, stream, originalHeaders) {
  return new Promise((resolve, reject) => {
    const body = JSON.stringify({ model, messages, stream });

    const options = {
      hostname: "127.0.0.1",
      port: LINTASAN_PORT,
      path: "/api/v1/chat/completions",
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Content-Length": Buffer.byteLength(body),
        Authorization: `Bearer ${_masterKey}`,
        "x-lintasan-mitm": "true",
        "x-mitm-source": originalHeaders["x-mitm-tool"] || "unknown",
      },
    };

    const req = http.request(options, (res) => {
      const chunks = [];
      res.on("data", (c) => chunks.push(c));
      res.on("end", () => {
        resolve({
          statusCode: res.statusCode,
          headers: res.headers,
          body: Buffer.concat(chunks),
        });
      });
    });

    req.on("error", reject);
    req.write(body);
    req.end();
  });
}

// Main interceptor — returns true if handled, false to passthrough
export async function interceptRequest(req, res, host) {
  const tool = detectTool(host, req.url);

  if (!tool) return false;

  const body = await collectBody(req);

  // For now, handle Copilot (OpenAI-compatible) directly
  // Other tools need protocol translation
  switch (tool) {
    case "copilot":
      return handleCopilot(req, res, body);
    case "cursor":
      return handleCursor(req, res, body);
    case "kiro":
      // Kiro uses AWS EventStream — complex binary protocol
      // Pass through for now, intercept in future version
      return false;
    case "gemini":
      // Gemini uses different format — pass through for now
      return false;
    default:
      return false;
  }
}

// Copilot uses OpenAI-compatible format — easiest to intercept
async function handleCopilot(req, res, body) {
  try {
    const parsed = JSON.parse(body.toString());
    const model = parsed.model || "gpt-4o";
    const messages = parsed.messages || [];
    const stream = parsed.stream !== false;

    console.log(`  🔀 [copilot] Intercepted: ${model} (${messages.length} msgs, stream=${stream})`);

    // Forward to Lintasan
    const result = await forwardToLintasan(model, messages, stream, {
      ...req.headers,
      "x-mitm-tool": "copilot",
    });

    res.writeHead(result.statusCode, {
      "content-type": result.headers["content-type"] || "application/json",
      "x-lintasan-intercepted": "true",
    });
    res.end(result.body);
    return true;
  } catch (err) {
    console.error("  ⚠  Copilot intercept error:", err.message);
    return false;
  }
}

// Cursor uses a custom protocol but some endpoints are OpenAI-compatible
async function handleCursor(req, res, body) {
  // /chat/completions is OpenAI-compatible on Cursor too
  if (req.url.includes("/chat/completions") || req.url.includes("/v1/chat/completions")) {
    try {
      const parsed = JSON.parse(body.toString());
      const model = parsed.model || "cursor-default";
      const messages = parsed.messages || [];
      const stream = parsed.stream !== false;

      console.log(`  🔀 [cursor] Intercepted: ${model} (${messages.length} msgs)`);

      const result = await forwardToLintasan(model, messages, stream, {
        ...req.headers,
        "x-mitm-tool": "cursor",
      });

      res.writeHead(result.statusCode, {
        "content-type": result.headers["content-type"] || "application/json",
        "x-lintasan-intercepted": "true",
      });
      res.end(result.body);
      return true;
    } catch {
      return false;
    }
  }

  // Other Cursor endpoints (BidiAppend, RunSSE) use custom protocol — pass through
  return false;
}
