import { validateDashboardSession } from "@/lib/auth";
import { getCacheStats, clearExpiredCache, clearAllCache } from "@/lib/cache";
import { getSetting, setSetting } from "@/lib/db";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const stats = getCacheStats();
  const enabled = getSetting("cache_enabled", "true") === "true";
  const ttl = parseInt(getSetting("cache_ttl", "3600"));

  return Response.json({ data: { ...stats, enabled, ttlSeconds: ttl } });
}

export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();

  if (body.action === "clear_expired") {
    const cleared = clearExpiredCache();
    return Response.json({ success: true, cleared });
  }

  if (body.action === "clear_all") {
    const cleared = clearAllCache();
    return Response.json({ success: true, cleared });
  }

  if (body.enabled !== undefined) {
    setSetting("cache_enabled", body.enabled ? "true" : "false");
  }

  if (body.ttlSeconds !== undefined) {
    setSetting("cache_ttl", String(body.ttlSeconds));
  }

  return Response.json({ success: true });
}
