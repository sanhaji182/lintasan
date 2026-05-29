import https from "https";
import http from "http";
import { createConnection, listConnections } from "./db/index.js";

// Known free-tier providers that don't require API keys or have generous free tiers
export const FREE_PROVIDERS = [
  {
    id: "ollama-local",
    name: "Ollama (Local)",
    baseUrl: "http://127.0.0.1:11434",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/api/tags",
    authRequired: false,
    description: "Local LLM inference via Ollama",
    detectPath: "/api/tags",
    parseModels: (data) => (data.models || []).map((m) => m.name),
  },
  {
    id: "lmstudio-local",
    name: "LM Studio (Local)",
    baseUrl: "http://127.0.0.1:1234",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: false,
    description: "Local LLM inference via LM Studio",
    detectPath: "/v1/models",
    parseModels: (data) => (data.data || []).map((m) => m.id),
  },
  {
    id: "jan-local",
    name: "Jan (Local)",
    baseUrl: "http://127.0.0.1:1337",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: false,
    description: "Local LLM inference via Jan",
    detectPath: "/v1/models",
    parseModels: (data) => (data.data || []).map((m) => m.id),
  },
  {
    id: "llamacpp-local",
    name: "llama.cpp (Local)",
    baseUrl: "http://127.0.0.1:8080",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: false,
    description: "Local LLM inference via llama.cpp server",
    detectPath: "/v1/models",
    parseModels: (data) => (data.data || []).map((m) => m.id),
  },
];

// Probe a provider endpoint to check if it's running
function probeEndpoint(url, timeout = 3000) {
  return new Promise((resolve) => {
    const parsed = new URL(url);
    const client = parsed.protocol === "https:" ? https : http;

    const req = client.get(
      {
        hostname: parsed.hostname,
        port: parsed.port,
        path: parsed.pathname,
        timeout,
      },
      (res) => {
        let data = "";
        res.on("data", (c) => (data += c));
        res.on("end", () => {
          try {
            resolve({ alive: true, status: res.statusCode, data: JSON.parse(data) });
          } catch {
            resolve({ alive: true, status: res.statusCode, data: null });
          }
        });
      }
    );

    req.on("error", () => resolve({ alive: false }));
    req.on("timeout", () => {
      req.destroy();
      resolve({ alive: false });
    });
  });
}

// Scan for available free providers
export async function scanFreeProviders() {
  const results = [];

  for (const provider of FREE_PROVIDERS) {
    const detectUrl = `${provider.baseUrl}${provider.detectPath}`;
    const probe = await probeEndpoint(detectUrl);

    if (probe.alive && probe.status === 200) {
      let models = [];
      if (probe.data && provider.parseModels) {
        try {
          models = provider.parseModels(probe.data);
        } catch {}
      }

      results.push({
        ...provider,
        available: true,
        models,
        modelCount: models.length,
      });
    } else {
      results.push({
        ...provider,
        available: false,
        models: [],
        modelCount: 0,
      });
    }
  }

  return results;
}

// Auto-add discovered free providers as connections
export async function autoAddFreeProviders() {
  const discovered = await scanFreeProviders();
  const existing = listConnections();
  const added = [];

  for (const provider of discovered) {
    if (!provider.available) continue;

    // Check if already connected (by base_url)
    const alreadyExists = existing.some(
      (c) => c.base_url === provider.baseUrl || c.name === provider.name
    );
    if (alreadyExists) continue;

    // Add as connection
    try {
      const id = createConnection({
        name: provider.name,
        baseUrl: provider.baseUrl,
        apiKey: "",
        format: provider.format,
        chatPath: provider.chatPath,
        modelsPath: provider.modelsPath,
        authHeader: "",
        authPrefix: "",
        priority: 0,
      });

      added.push({
        id,
        name: provider.name,
        models: provider.models,
        modelCount: provider.modelCount,
      });
    } catch (err) {
      // Skip on error
    }
  }

  return { discovered, added };
}
