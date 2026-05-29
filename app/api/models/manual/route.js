import { validateDashboardSession } from "@/lib/auth";
import { addModelToConnection, removeModelFromConnection, listDiscoveredModels, toggleModelActive } from "@/lib/db/index.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// GET /api/models/manual?connectionId=xxx — list models for a connection
export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { searchParams } = new URL(request.url);
  const connectionId = searchParams.get("connectionId");
  const models = listDiscoveredModels(connectionId || null);
  return Response.json({ models });
}

// POST /api/models/manual — add model(s) to a connection or toggle active state
export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const { action, connectionId, modelId, active, models } = body;

  // Handle toggle action
  if (action === "toggle") {
    if (!connectionId || !modelId) {
      return Response.json({ error: "connectionId and modelId are required for toggle" }, { status: 400 });
    }
    toggleModelActive(connectionId, modelId, active ? true : false);
    return Response.json({ success: true, model_id: modelId, is_active: active ? 1 : 0 });
  }

  if (!connectionId) {
    return Response.json({ error: "connectionId is required" }, { status: 400 });
  }

  // Support single model or array
  const modelList = Array.isArray(models) ? models : [models];
  
  if (modelList.length === 0) {
    return Response.json({ error: "models array is required" }, { status: 400 });
  }

  const results = [];
  for (const m of modelList) {
    const modelId = typeof m === "string" ? m : m.id || m.model_id;
    const modelName = typeof m === "string" ? m : (m.name || m.model_name || modelId);
    const ownedBy = typeof m === "string" ? "" : (m.owned_by || "");
    
    if (!modelId) continue;
    const result = addModelToConnection(connectionId, modelId, modelName, ownedBy);
    results.push({ model_id: modelId, ...result });
  }

  const added = results.filter(r => r.added).length;
  const skipped = results.filter(r => !r.added).length;

  return Response.json({
    success: true,
    added,
    skipped,
    results,
  });
}

// DELETE /api/models/manual — remove model from a connection
export async function DELETE(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const connectionId = searchParams.get("connectionId");
  const modelId = searchParams.get("modelId");

  if (!connectionId || !modelId) {
    return Response.json({ error: "connectionId and modelId are required" }, { status: 400 });
  }

  removeModelFromConnection(connectionId, modelId);
  return Response.json({ success: true, removed: modelId });
}
