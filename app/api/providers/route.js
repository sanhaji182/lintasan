import { validateDashboardSession } from "@/lib/auth";
import { listConnections } from "@/lib/db";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// GET /api/providers — returns active connections (replaces old hardcoded registry)
export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const connections = listConnections();
  const providers = connections.map(c => ({
    id: c.id,
    name: c.name,
    format: c.format,
    baseUrl: c.base_url,
    modelsCount: c.models_count || 0,
    isActive: !!c.is_active,
    lastSync: c.last_sync,
  }));
  return Response.json({ data: providers });
}
