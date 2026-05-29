import { getRealtimeStats, getTimeSeries, getTopModels, getTopProviders } from "@/lib/streaming-analytics.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  try {
    const url = new URL(request.url);
    const period = url.searchParams.get("period") || "1h";
    const topLimit = parseInt(url.searchParams.get("limit") || "10", 10);

    const stats = getRealtimeStats();
    const timeSeries = getTimeSeries(period === "24h" ? "24h" : "1h");
    const topModels = getTopModels(topLimit);
    const topProviders = getTopProviders(topLimit);

    return Response.json({
      stats,
      timeSeries,
      topModels,
      topProviders,
      period,
      generatedAt: new Date().toISOString(),
    });
  } catch (error) {
    return Response.json({ error: error.message }, { status: 500 });
  }
}
