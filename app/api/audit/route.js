import { validateDashboardSession } from "@/lib/auth";
import { getAuditLog, clearAuditLog } from "@/lib/audit";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { searchParams } = new URL(request.url);
  const limit = parseInt(searchParams.get("limit") || "100");
  return Response.json({ data: getAuditLog(limit) });
}

export async function DELETE(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const result = clearAuditLog(30);
  return Response.json({ success: true, cleared: result.changes });
}
