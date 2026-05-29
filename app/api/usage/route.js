import { validateDashboardSession } from "@/lib/auth";
import { getDb } from "@/lib/db";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const days = parseInt(searchParams.get("days") || "7");

  const db = getDb();

  // Daily stats
  const dailyStats = db.prepare(`
    SELECT 
      DATE(created_at) as date,
      COUNT(*) as requests,
      SUM(input_tokens) as input_tokens,
      SUM(output_tokens) as output_tokens,
      AVG(latency_ms) as avg_latency,
      SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END) as errors
    FROM request_logs
    WHERE created_at >= datetime('now', '-' || ? || ' days')
    GROUP BY DATE(created_at)
    ORDER BY date DESC
  `).all(days);

  // Per-provider stats
  const providerStats = db.prepare(`
    SELECT 
      provider,
      COUNT(*) as requests,
      SUM(input_tokens) as input_tokens,
      SUM(output_tokens) as output_tokens,
      AVG(latency_ms) as avg_latency,
      SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END) as errors
    FROM request_logs
    WHERE created_at >= datetime('now', '-' || ? || ' days')
    GROUP BY provider
    ORDER BY requests DESC
  `).all(days);

  // Per-model stats
  const modelStats = db.prepare(`
    SELECT 
      model,
      provider,
      COUNT(*) as requests,
      SUM(input_tokens) as input_tokens,
      SUM(output_tokens) as output_tokens,
      AVG(latency_ms) as avg_latency
    FROM request_logs
    WHERE created_at >= datetime('now', '-' || ? || ' days')
    GROUP BY model
    ORDER BY requests DESC
    LIMIT 20
  `).all(days);

  // Totals
  const totals = db.prepare(`
    SELECT 
      COUNT(*) as total_requests,
      SUM(input_tokens) as total_input_tokens,
      SUM(output_tokens) as total_output_tokens,
      AVG(latency_ms) as avg_latency,
      SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END) as total_errors,
      SUM(CASE WHEN provider = 'cache' THEN 1 ELSE 0 END) as cache_hits
    FROM request_logs
    WHERE created_at >= datetime('now', '-' || ? || ' days')
  `).get(days);

  // Cost estimation (rough pricing per 1M tokens)
  const PRICING = {
    commandcode: { input: 0, output: 0 }, // Free (Go plan)
    openai: { input: 2.5, output: 10 },
    anthropic: { input: 3, output: 15 },
    deepseek: { input: 0.14, output: 0.28 },
    groq: { input: 0.05, output: 0.08 },
    openrouter: { input: 1, output: 3 },
    cache: { input: 0, output: 0 },
  };

  let estimatedCost = 0;
  for (const ps of providerStats) {
    const pricing = PRICING[ps.provider] || { input: 1, output: 3 };
    estimatedCost += ((ps.input_tokens || 0) / 1_000_000) * pricing.input;
    estimatedCost += ((ps.output_tokens || 0) / 1_000_000) * pricing.output;
  }

  return Response.json({
    data: {
      days,
      totals: { ...totals, estimated_cost_usd: estimatedCost.toFixed(4) },
      daily: dailyStats,
      providers: providerStats,
      models: modelStats,
    },
  });
}
