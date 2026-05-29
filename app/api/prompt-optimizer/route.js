// Prompt Optimizer API - settings and test endpoint
import { isPromptOptimizerEnabled, optimizePrompt } from "@/lib/prompt-optimizer.js";
import { getDb } from "@/lib/db/index.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const enabled = isPromptOptimizerEnabled();
    return Response.json({ enabled });
  } catch (error) {
    return Response.json({ error: error.message }, { status: 500 });
  }
}

export async function POST(request) {
  try {
    const body = await request.json();

    // Toggle enable/disable
    if (body.enabled !== undefined) {
      const db = getDb();
      db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("prompt_optimizer_enabled", body.enabled ? "true" : "false");
      return Response.json({ success: true, enabled: body.enabled });
    }

    // Test optimization
    if (body.messages) {
      const result = optimizePrompt(body.messages);
      return Response.json({
        original: body.messages,
        optimized: result.messages,
        savings: result.savings + "%",
        originalTokens: result.originalTokens,
        optimizedTokens: result.optimizedTokens,
        tokensSaved: result.originalTokens - result.optimizedTokens,
      });
    }

    return Response.json({ error: "Provide 'enabled' or 'messages'" }, { status: 400 });
  } catch (error) {
    return Response.json({ error: error.message }, { status: 500 });
  }
}
