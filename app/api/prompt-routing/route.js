// Prompt Routing API - CRUD for routing rules
import { getPromptRoutingRules, setPromptRoutingRules, isPromptRoutingEnabled, estimateComplexity } from "@/lib/prompt-router.js";
import { getSetting, getDb } from "@/lib/db/index.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const enabled = isPromptRoutingEnabled();
    const rules = getPromptRoutingRules();
    return Response.json({ enabled, rules });
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
      db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("prompt_routing_enabled", body.enabled ? "true" : "false");
      return Response.json({ success: true, enabled: body.enabled });
    }

    // Update rules
    if (body.rules) {
      setPromptRoutingRules(body.rules);
      return Response.json({ success: true, rules: body.rules });
    }

    // Test complexity estimation
    if (body.messages) {
      const complexity = estimateComplexity(body.messages);
      const rules = getPromptRoutingRules();
      const rule = rules[complexity];
      return Response.json({
        complexity,
        wouldRoute: rule ? { model: rule.model, provider: rule.provider } : null,
      });
    }

    return Response.json({ error: "Provide 'enabled', 'rules', or 'messages'" }, { status: 400 });
  } catch (error) {
    return Response.json({ error: error.message }, { status: 500 });
  }
}
