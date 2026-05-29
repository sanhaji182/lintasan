/**
 * AI Plugin Generator API
 * POST /api/plugins/generate — generate plugin code from natural language description
 * Uses Lintasan's own LLM connections to generate the code
 */

import { getMasterKey } from "@/lib/auth";
import { getSetting } from "@/lib/db/index.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const SYSTEM_PROMPT = `You are a plugin code generator for Lintasan LLM Proxy. Generate a complete, working ES module plugin.

The plugin system has these hooks:
- beforeRequest(ctx) — ctx has: { model, messages, stream, auth, headers, metadata }. Can modify ctx or return { shortCircuit: true, response: Response } to skip upstream.
- afterRequest(ctx, response) — response is parsed JSON. Can modify or log.
- onError(ctx, error) — called on upstream error.
- onStream(ctx, chunk) — called per SSE chunk during streaming.

Rules:
1. Export default object with: name, version, description, priority (number, lower=first), enabled (true), hooks object
2. Use only Node.js built-ins (no external imports except 'fs', 'path', 'crypto')
3. Code must be a valid ES module (export default {})
4. Include helpful comments
5. Return ONLY the JavaScript code, no markdown fences, no explanation

Example structure:
export default {
  name: "my-plugin",
  version: "1.0.0",
  description: "What it does",
  priority: 10,
  enabled: true,
  hooks: {
    beforeRequest(ctx) { },
    afterRequest(ctx, response) { }
  }
};`;

export async function POST(request) {
  const body = await request.json();
  const { prompt, name } = body;

  if (!prompt) {
    return Response.json(
      { error: "Required: { prompt: string, name?: string }" },
      { status: 400 }
    );
  }

  // Use Lintasan itself to generate (localhost)
  const masterKey = getMasterKey();
  const port = getSetting("port", "20180");
  const baseUrl = `http://127.0.0.1:${port}/api/v1`;

  // Pick a model — prefer a smart one
  const preferredModel = getSetting("ai_agent_model", "") || getSetting("plugin_generator_model", "");

  let model = preferredModel;
  if (!model) {
    // Try to find an available model from /v1/models
    try {
      const modelsRes = await fetch(`${baseUrl}/models`, {
        headers: { "Authorization": `Bearer ${masterKey}` }
      });
      const modelsData = await modelsRes.json();
      const models = modelsData.data || [];
      // Prefer large models for code generation
      const preferred = ["deepseek/deepseek-v4-pro", "deepseek-v4-pro", "gpt-4o", "claude-3-opus", "claude-3-sonnet", "gpt-4", "deepseek-chat", "deepseek-coder"];
      for (const p of preferred) {
        if (models.find(m => m.id === p || m.id.includes(p))) {
          model = models.find(m => m.id === p || m.id.includes(p)).id;
          break;
        }
      }
      // Fallback to first available
      if (!model && models.length > 0) {
        model = models[0].id;
      }
    } catch (e) {
      // ignore
    }
  }

  if (!model) {
    return Response.json(
      { error: "No LLM model available. Add a connection and sync models first, or set ai_agent_model in settings." },
      { status: 503 }
    );
  }

  const pluginName = name || prompt.toLowerCase().replace(/[^a-z0-9]+/g, "-").slice(0, 30);

  try {
    const llmRes = await fetch(`${baseUrl}/chat/completions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${masterKey}`
      },
      body: JSON.stringify({
        model,
        messages: [
          { role: "system", content: SYSTEM_PROMPT },
          { role: "user", content: `Generate a Lintasan plugin named "${pluginName}" that does the following:\n\n${prompt}` }
        ],
        temperature: 0.3,
        max_tokens: 16384
      })
    });

    if (!llmRes.ok) {
      const err = await llmRes.text();
      return Response.json(
        { error: "LLM request failed: " + err },
        { status: 502 }
      );
    }

    const llmData = await llmRes.json();
    let code = llmData.choices?.[0]?.message?.content || "";

    // Clean up — remove markdown fences if present
    code = code.replace(/^```(?:javascript|js)?\n?/gm, "").replace(/```$/gm, "").trim();

    if (!code.includes("export default")) {
      return Response.json(
        { error: "Generated code doesn't look like a valid plugin. Try a more specific description.", generated: code },
        { status: 422 }
      );
    }

    return Response.json({
      ok: true,
      name: pluginName,
      code,
      model_used: model,
      description: prompt
    });

  } catch (err) {
    return Response.json(
      { error: "Failed to generate: " + err.message },
      { status: 500 }
    );
  }
}
