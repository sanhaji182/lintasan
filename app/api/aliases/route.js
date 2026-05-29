import { validateDashboardSession } from "@/lib/auth";
import { getAllModelAliases, setModelAlias } from "@/lib/router";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  return Response.json({ data: getAllModelAliases() });
}

export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { alias, model, provider } = await request.json();
  if (!alias) return Response.json({ error: "alias required" }, { status: 400 });
  setModelAlias(alias, model, provider);
  return Response.json({ success: true });
}

export async function DELETE(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { searchParams } = new URL(request.url);
  const alias = searchParams.get("alias");
  if (!alias) return Response.json({ error: "alias required" }, { status: 400 });
  setModelAlias(alias, null, null);
  return Response.json({ success: true });
}
