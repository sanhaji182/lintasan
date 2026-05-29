import { validateDashboardSession } from "@/lib/auth";
import { getRecentLogs } from "@/lib/db";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  try {
    const { searchParams } = new URL(request.url);
    const limit = parseInt(searchParams.get("limit") || "50");
    const logs = getRecentLogs(limit);
    return Response.json({ data: logs });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}
