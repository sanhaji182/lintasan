import { validateDashboardSession } from "@/lib/auth";
import { createApiKey, listApiKeys, deleteApiKey, toggleApiKey } from "@/lib/api-keys";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  return Response.json({ data: listApiKeys() });
}

export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { name, quotaRpm, quotaDaily } = await request.json();
  if (!name) return Response.json({ error: "name required" }, { status: 400 });
  const key = createApiKey({ name, quotaRpm: quotaRpm || 60, quotaDaily: quotaDaily || 1000 });
  return Response.json({ data: key }, { status: 201 });
}

export async function DELETE(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { searchParams } = new URL(request.url);
  const id = searchParams.get("id");
  if (!id) return Response.json({ error: "id required" }, { status: 400 });
  deleteApiKey(id);
  return Response.json({ success: true });
}

export async function PATCH(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { id, active } = await request.json();
  if (!id) return Response.json({ error: "id required" }, { status: 400 });
  toggleApiKey(id, active);
  return Response.json({ success: true });
}
