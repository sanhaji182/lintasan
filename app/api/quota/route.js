import { validateDashboardSession } from "@/lib/auth";
import { getAllQuotaStatus, getQuotaStatus, setQuotaLimits } from "@/lib/quota-tracking";
import { listConnections } from "@/lib/db";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// GET /api/quota — get quota status for all connections
export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { searchParams } = new URL(request.url);
  const connectionId = searchParams.get("connection_id");

  if (connectionId) {
    return Response.json({ data: getQuotaStatus(connectionId) });
  }

  // Get all connections and their quota
  const connections = listConnections();
  const statuses = connections.map(c => ({
    ...getQuotaStatus(c.id),
    connection_name: c.name,
  }));
  return Response.json({ data: statuses });
}

// POST /api/quota — set quota limits for a connection
export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  try {
    const body = await request.json();
    const { connection_id, daily_tokens, monthly_tokens } = body;
    if (!connection_id) {
      return Response.json({ error: { message: "connection_id is required" } }, { status: 400 });
    }
    setQuotaLimits(connection_id, daily_tokens || 0, monthly_tokens || 0);
    return Response.json({ success: true });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}
