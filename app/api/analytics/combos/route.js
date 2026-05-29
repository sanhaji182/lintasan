import { NextResponse } from "next/server";
import { validateDashboardSession } from "@/lib/auth";
import { getDb } from "@/lib/db";

// GET /api/analytics/combos — combo usage stats
export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const period = searchParams.get("period") || "24h";

  const db = getDb();

  // Time filter
  let timeFilter = "datetime('now', '-24 hours')";
  if (period === "7d") timeFilter = "datetime('now', '-7 days')";
  if (period === "30d") timeFilter = "datetime('now', '-30 days')";
  if (period === "all") timeFilter = "datetime('2000-01-01')";

  // Combo stats — aggregate by combo_name
  const comboStats = db.prepare(`
    SELECT 
      combo_name,
      COUNT(*) as total_requests,
      SUM(CASE WHEN status >= 200 AND status < 400 THEN 1 ELSE 0 END) as success_count,
      SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END) as error_count,
      SUM(CASE WHEN cached = 1 THEN 1 ELSE 0 END) as cache_hits,
      AVG(latency_ms) as avg_latency,
      MIN(latency_ms) as min_latency,
      MAX(latency_ms) as max_latency,
      SUM(input_tokens) as total_input_tokens,
      SUM(output_tokens) as total_output_tokens,
      SUM(input_tokens + output_tokens) as total_tokens
    FROM request_logs
    WHERE combo_name IS NOT NULL 
      AND created_at >= ${timeFilter}
    GROUP BY combo_name
    ORDER BY total_requests DESC
  `).all();

  // Per-combo model breakdown
  const comboModels = db.prepare(`
    SELECT 
      combo_name,
      model,
      COUNT(*) as requests,
      AVG(latency_ms) as avg_latency,
      SUM(CASE WHEN status >= 200 AND status < 400 THEN 1 ELSE 0 END) as successes,
      SUM(input_tokens + output_tokens) as tokens
    FROM request_logs
    WHERE combo_name IS NOT NULL 
      AND created_at >= ${timeFilter}
    GROUP BY combo_name, model
    ORDER BY combo_name, requests DESC
  `).all();

  // Overall stats (including non-combo)
  const overall = db.prepare(`
    SELECT 
      COUNT(*) as total_requests,
      SUM(CASE WHEN combo_name IS NOT NULL THEN 1 ELSE 0 END) as combo_requests,
      SUM(CASE WHEN combo_name IS NULL THEN 1 ELSE 0 END) as direct_requests
    FROM request_logs
    WHERE created_at >= ${timeFilter}
  `).get();

  // Format combo stats with model breakdown
  const combos = comboStats.map((combo) => ({
    ...combo,
    avg_latency: Math.round(combo.avg_latency || 0),
    success_rate: combo.total_requests > 0
      ? Math.round((combo.success_count / combo.total_requests) * 100)
      : 0,
    models: comboModels
      .filter((m) => m.combo_name === combo.combo_name)
      .map((m) => ({
        model: m.model,
        requests: m.requests,
        avg_latency: Math.round(m.avg_latency || 0),
        successes: m.successes,
        tokens: m.tokens,
      })),
  }));

  return NextResponse.json({
    data: {
      combos,
      overall,
      period,
    },
  });
}
