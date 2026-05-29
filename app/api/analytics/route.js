// Token Analytics API - provides data for the analytics dashboard
import { getDb } from "@/lib/db/index.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  try {
    const db = getDb();
    const url = new URL(request.url);
    const period = url.searchParams.get("period") || "7d";

    let days = 7;
    if (period === "30d") days = 30;
    if (period === "1d") days = 1;

    // Total stats
    const totalStats = db.prepare(`
      SELECT 
        COUNT(*) as total_requests,
        SUM(input_tokens) as total_input_tokens,
        SUM(output_tokens) as total_output_tokens,
        AVG(latency_ms) as avg_latency
      FROM request_logs 
      WHERE created_at >= datetime('now', '-${days} days')
    `).get();

    // Cache hits
    const cacheHits = db.prepare(`
      SELECT COUNT(*) as count, SUM(input_tokens) as input_tokens, SUM(output_tokens) as output_tokens
      FROM request_logs 
      WHERE provider IN ('cache', 'semantic-cache', 'stream-cache')
      AND created_at >= datetime('now', '-${days} days')
    `).get();

    // Today's stats
    const todayStats = db.prepare(`
      SELECT 
        COUNT(*) as total_requests,
        SUM(input_tokens) as total_input_tokens,
        SUM(output_tokens) as total_output_tokens
      FROM request_logs 
      WHERE created_at >= datetime('now', 'start of day')
    `).get();

    const todayCacheHits = db.prepare(`
      SELECT COUNT(*) as count, SUM(input_tokens + output_tokens) as tokens_saved
      FROM request_logs 
      WHERE provider IN ('cache', 'semantic-cache', 'stream-cache')
      AND created_at >= datetime('now', 'start of day')
    `).get();

    // Daily breakdown (last N days)
    const dailyUsage = db.prepare(`
      SELECT 
        date(created_at) as day,
        COUNT(*) as requests,
        SUM(input_tokens) as input_tokens,
        SUM(output_tokens) as output_tokens,
        SUM(CASE WHEN provider IN ('cache', 'semantic-cache', 'stream-cache') THEN 1 ELSE 0 END) as cache_hits
      FROM request_logs 
      WHERE created_at >= datetime('now', '-${days} days')
      GROUP BY date(created_at)
      ORDER BY day ASC
    `).all();

    // Provider breakdown
    const providerBreakdown = db.prepare(`
      SELECT 
        provider,
        COUNT(*) as count,
        SUM(input_tokens) as input_tokens,
        SUM(output_tokens) as output_tokens
      FROM request_logs 
      WHERE created_at >= datetime('now', '-${days} days')
      GROUP BY provider
      ORDER BY count DESC
    `).all();

    // Calculate savings
    const totalTokens = (totalStats.total_input_tokens || 0) + (totalStats.total_output_tokens || 0);
    const cacheTokensSaved = (cacheHits.input_tokens || 0) + (cacheHits.output_tokens || 0);
    const cacheHitRate = totalStats.total_requests > 0
      ? Math.round((cacheHits.count / totalStats.total_requests) * 100)
      : 0;

    // Estimate cost savings (rough: $0.002 per 1K tokens average)
    const costPerToken = 0.000002;
    const costSaved = cacheTokensSaved * costPerToken;

    return Response.json({
      period,
      summary: {
        totalRequests: totalStats.total_requests || 0,
        totalTokens,
        totalInputTokens: totalStats.total_input_tokens || 0,
        totalOutputTokens: totalStats.total_output_tokens || 0,
        avgLatency: Math.round(totalStats.avg_latency || 0),
        cacheHits: cacheHits.count || 0,
        cacheHitRate,
        tokensSaved: cacheTokensSaved,
        costSaved: costSaved.toFixed(4),
      },
      today: {
        totalRequests: todayStats.total_requests || 0,
        totalTokens: (todayStats.total_input_tokens || 0) + (todayStats.total_output_tokens || 0),
        tokensSaved: todayCacheHits.tokens_saved || 0,
        cacheHits: todayCacheHits.count || 0,
      },
      dailyUsage,
      providerBreakdown,
    });
  } catch (error) {
    return Response.json({ error: error.message }, { status: 500 });
  }
}
