import { validateDashboardSession } from "@/lib/auth";
import { getConnection, saveDiscoveredModels, listDiscoveredModels, getActiveConnections } from "@/lib/db";
import { PROVIDER_PRESETS } from "@/lib/provider-presets";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// GET /api/models/sync — list all discovered models (or per connection)
export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  try {
    const { searchParams } = new URL(request.url);
    const connectionId = searchParams.get("connection_id");
    const models = listDiscoveredModels(connectionId || null);
    return Response.json({ data: models });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}

// POST /api/models/sync — sync models from a connection (or all connections)
export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  try {
    const body = await request.json().catch(() => ({}));
    const { connection_id } = body;

    let connections = [];
    if (connection_id) {
      const conn = getConnection(connection_id);
      if (!conn) return Response.json({ error: { message: "Connection not found" } }, { status: 404 });
      connections = [conn];
    } else {
      connections = getActiveConnections();
    }

    const results = [];

    for (const conn of connections) {
      try {
        const models = await fetchModelsFromProvider(conn);
        saveDiscoveredModels(conn.id, models);
        results.push({ connection_id: conn.id, name: conn.name, status: "ok", models_count: models.length });
      } catch (err) {
        results.push({ connection_id: conn.id, name: conn.name, status: "error", error: err.message });
      }
    }

    return Response.json({ data: results });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}

// Known models for providers that don't have a /models endpoint
const KNOWN_MODELS = {
  commandcode: [
    { id: "deepseek/deepseek-v4-pro", name: "DeepSeek V4 Pro", owned_by: "deepseek" },
    { id: "kimi/kimi-k2.6", name: "Kimi K2.6", owned_by: "moonshot" },
    { id: "glm/glm-4.7", name: "GLM 4.7", owned_by: "zhipu" },
    { id: "minimax/minimax-m2.7", name: "MiniMax M2.7", owned_by: "minimax" },
    { id: "qwen/qwen3-coder", name: "Qwen3 Coder", owned_by: "alibaba" },
    { id: "deepseek/deepseek-r1", name: "DeepSeek R1", owned_by: "deepseek" },
    { id: "deepseek/deepseek-v3", name: "DeepSeek V3", owned_by: "deepseek" },
  ],
};

// Get known models from presets or hardcoded list
function getKnownModels(conn) {
  // Check presets by matching name or base_url
  const preset = PROVIDER_PRESETS.find(p =>
    p.name.toLowerCase() === conn.name.toLowerCase() ||
    (p.baseUrl && conn.base_url && conn.base_url.startsWith(p.baseUrl))
  );
  if (preset && preset.knownModels) return preset.knownModels;

  // Check hardcoded by format
  if (KNOWN_MODELS[conn.format]) return KNOWN_MODELS[conn.format];

  return null;
}

async function fetchModelsFromProvider(conn) {
  const modelsPath = conn.models_path || "/v1/models";
  const url = conn.base_url + modelsPath;

  const headers = {
    "Content-Type": "application/json",
    ...(conn.extra_headers ? JSON.parse(conn.extra_headers) : {}),
  };

  // Add auth
  if (conn.auth_header && conn.api_key) {
    headers[conn.auth_header] = (conn.auth_prefix || "") + conn.api_key;
  }

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 15000);

  try {
    const res = await fetch(url, { headers, signal: controller.signal });
    clearTimeout(timeout);

    if (!res.ok) {
      // If provider doesn't have models endpoint, use known models
      const known = getKnownModels(conn);
      if (known) return known;
      const text = await res.text().catch(() => "");
      throw new Error(`HTTP ${res.status}: ${text.slice(0, 200)}`);
    }

    const data = await res.json();

    // Handle OpenAI format: { data: [...] } or { models: [...] } or plain array
    let models = [];
    if (Array.isArray(data)) {
      models = data;
    } else if (Array.isArray(data.data)) {
      models = data.data;
    } else if (Array.isArray(data.models)) {
      models = data.models;
    }

    // Normalize model objects
    const normalized = models.map(m => ({
      id: m.id || m.model || m.name,
      name: m.name || m.id || m.model,
      owned_by: m.owned_by || m.owner || "",
    })).filter(m => m.id);

    // If API returned empty but we have known models, use those
    if (normalized.length === 0) {
      const known = getKnownModels(conn);
      if (known) return known;
    }

    return normalized;
  } catch (err) {
    clearTimeout(timeout);
    if (err.name === "AbortError") throw new Error("Timeout fetching models (15s)");
    // Fallback to known models if available
    const known = getKnownModels(conn);
    if (known) return known;
    throw err;
  }
}
