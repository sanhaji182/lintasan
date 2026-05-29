import { listDiscoveredModels } from "@/lib/db";
import { listCombos } from "@/lib/combo";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// GET /api/v1/models — OpenAI-compatible models list from all connected providers
export async function GET() {
  try {
    const discovered = listDiscoveredModels();

    // Deduplicate by model_id (same model from multiple providers → show once)
    const seen = new Map();
    for (const m of discovered) {
      if (!seen.has(m.model_id)) {
        seen.set(m.model_id, {
          id: m.model_id,
          object: "model",
          created: Math.floor(new Date(m.discovered_at).getTime() / 1000) || 0,
          owned_by: m.owned_by || m.connection_name || "unknown",
          connection: m.connection_name,
        });
      }
    }

    const models = Array.from(seen.values());

    // Add combos as virtual models
    const combos = listCombos();
    for (const combo of combos) {
      models.push({
        id: combo.name,
        object: "model",
        created: Math.floor(new Date(combo.created_at).getTime() / 1000) || 0,
        owned_by: "combo",
        connection: "Combo: " + (combo.models || []).map(m => m.label || m.model).join(" → "),
      });
    }

    return Response.json({
      object: "list",
      data: models,
    });
  } catch (error) {
    return Response.json({ object: "list", data: [] });
  }
}
