import { validateDashboardSession } from "@/lib/auth";
import {
  exportConfig,
  exportAnalytics,
  exportApiKeys,
  importConfig,
} from "@/lib/export-backup";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const type = searchParams.get("type") || "full";
  const format = searchParams.get("format") || "json";
  const from = searchParams.get("from") || undefined;
  const to = searchParams.get("to") || undefined;
  const limit = searchParams.get("limit") || undefined;

  try {
    switch (type) {
      case "config": {
        const data = exportConfig();
        return Response.json({ data });
      }

      case "analytics": {
        const result = exportAnalytics({ format, from, to, limit });
        if (format === "csv") {
          return new Response(result, {
            headers: {
              "Content-Type": "text/csv",
              "Content-Disposition": `attachment; filename="lintasan-analytics-${new Date().toISOString().slice(0, 10)}.csv"`,
            },
          });
        }
        return Response.json({ data: result });
      }

      case "keys": {
        const data = exportApiKeys();
        return Response.json({ data });
      }

      case "full": {
        const config = exportConfig();
        const analytics = exportAnalytics({ format: "json", from, to, limit: limit || "1000" });
        const keys = exportApiKeys();
        return Response.json({
          data: {
            ...config,
            analytics: analytics.logs,
            api_keys: keys.keys,
          },
        });
      }

      default:
        return Response.json({ error: "Invalid type. Use: config, analytics, keys, full" }, { status: 400 });
    }
  } catch (err) {
    return Response.json({ error: err.message }, { status: 500 });
  }
}

export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  try {
    const body = await request.json();

    if (!body || typeof body !== "object") {
      return Response.json({ error: "Invalid JSON body" }, { status: 400 });
    }

    const result = importConfig(body);
    return Response.json({ success: true, imported: result });
  } catch (err) {
    return Response.json({ error: err.message }, { status: 500 });
  }
}
