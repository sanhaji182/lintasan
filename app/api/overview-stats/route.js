import { validateDashboardSession } from "@/lib/auth";
import { getDb, getSetting } from "@/lib/db";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const db = getDb();

  let totalRequests = 0;
  let errorCount = 0;
  let cacheHits = 0;
  let exactCacheHits = 0;
  let semanticCacheHits = 0;
  let streamCacheHits = 0;
  let coalescedRequests = 0;
  let tokensSaved = 0;
  let tokensCompressed = 0;
  let avgLatency = 0;
  let tokensToday = 0;
  let tokensMonth = 0;
  let cacheHitRate = 0;
  let providers = [];
  let features = [];

  try {
    // Total requests (last 24h)
    const total = db.prepare("SELECT COUNT(*) as cnt FROM request_logs WHERE created_at >= datetime('now', '-24 hours')").get();
    totalRequests = total?.cnt || 0;

    // Errors (last 24h)
    const errors = db.prepare("SELECT COUNT(*) as cnt FROM request_logs WHERE status >= 400 AND created_at >= datetime('now', '-24 hours')").get();
    errorCount = errors?.cnt || 0;

    // Cache hits by type (last 24h)
    const cacheTypes = db.prepare("SELECT provider, COUNT(*) as cnt FROM request_logs WHERE created_at >= datetime('now', '-24 hours') AND provider IN ('cache', 'semantic-cache', 'stream-cache', 'coalesced') GROUP BY provider").all();
    for (const row of cacheTypes) {
      if (row.provider === "cache") exactCacheHits = row.cnt;
      if (row.provider === "semantic-cache") semanticCacheHits = row.cnt;
      if (row.provider === "stream-cache") streamCacheHits = row.cnt;
      if (row.provider === "coalesced") coalescedRequests = row.cnt;
    }
    cacheHits = exactCacheHits + semanticCacheHits + streamCacheHits + coalescedRequests;

    // Avg latency (last 24h)
    const latency = db.prepare("SELECT AVG(latency_ms) as avg_lat FROM request_logs WHERE latency_ms > 0 AND created_at >= datetime('now', '-24 hours')").get();
    avgLatency = Math.round(latency?.avg_lat || 0);

    // Tokens today
    const tokenSum = db.prepare("SELECT COALESCE(SUM(input_tokens), 0) + COALESCE(SUM(output_tokens), 0) as total FROM request_logs WHERE created_at >= datetime('now', '-24 hours')").get();
    tokensToday = tokenSum?.total || 0;

    // Cache hit rate
    if (totalRequests > 0) cacheHitRate = Math.round((cacheHits / totalRequests) * 100);

    // Tokens saved estimate (cache hits * avg tokens per request)
    const avgTokens = db.prepare("SELECT AVG(input_tokens) as avg_in FROM request_logs WHERE input_tokens > 0 AND created_at >= datetime('now', '-24 hours')").get();
    const avgIn = avgTokens?.avg_in || 7500;
    tokensSaved = Math.round(cacheHits * avgIn);

    // Tokens compressed estimate
    const compressedCount = db.prepare("SELECT COUNT(*) as cnt FROM request_logs WHERE input_tokens > 0 AND created_at >= datetime('now', '-24 hours') AND provider != 'cache' AND provider != 'semantic-cache'").get();
    tokensCompressed = Math.round((compressedCount?.cnt || 0) * 200);

    // Tokens this month
    const monthTokens = db.prepare("SELECT COALESCE(SUM(input_tokens), 0) + COALESCE(SUM(output_tokens), 0) as total FROM request_logs WHERE created_at >= date('now', 'start of month')").get();
    tokensMonth = monthTokens?.total || 0;

    // Providers list
    const providerRows = db.prepare("SELECT DISTINCT provider FROM request_logs WHERE created_at >= datetime('now', '-24 hours') AND provider NOT IN ('cache', 'semantic-cache', 'stream-cache', 'coalesced')").all();
    providers = providerRows.map(r => ({ name: r.provider, healthy: true, latency: null }));

  } catch (e) {
    // Tables might not exist yet
  }

  // Feature enabled status
  const cacheEnabled = getSetting("cache_enabled", "true") === "true";
  const semanticCacheEnabled = getSetting("semantic_cache_enabled", "true") === "true";
  const compressionEnabled = getSetting("compression_enabled", "true") === "true";
  const smartTokensEnabled = getSetting("smart_tokens_enabled", "true") === "true";
  const circuitBreakerEnabled = getSetting("circuit_breaker_enabled", "true") === "true";
  const streamCacheEnabled = getSetting("stream_cache_enabled", "true") === "true";
  const batchRoutingEnabled = getSetting("batch_routing_enabled", "true") === "true";
  const qualityRoutingEnabled = getSetting("quality_routing_enabled", "false") === "true";
  const mlRouterEnabled = getSetting("ml_router_enabled", "false") === "true";

  features = [
    { name: "Cache", enabled: cacheEnabled },
    { name: "Semantic Cache", enabled: semanticCacheEnabled },
    { name: "Compression", enabled: compressionEnabled },
    { name: "Smart Tokens", enabled: smartTokensEnabled },
    { name: "Circuit Breaker", enabled: circuitBreakerEnabled },
    { name: "Stream Cache", enabled: streamCacheEnabled },
    { name: "Batch Routing", enabled: batchRoutingEnabled },
    { name: "Quality Routing", enabled: qualityRoutingEnabled },
    { name: "ML Router", enabled: mlRouterEnabled },
  ];

  return Response.json({
    data: {
      totalRequests,
      errorCount,
      cacheHits,
      exactCacheHits,
      semanticCacheHits,
      streamCacheHits,
      coalescedRequests,
      tokensSaved,
      tokensCompressed,
      avgLatency,
      tokensToday,
      tokensMonth,
      cacheHitRate,
      providers,
      features,
      cacheEnabled,
      semanticCacheEnabled,
      compressionEnabled,
      smartTokensEnabled,
      circuitBreakerEnabled,
      streamCacheEnabled,
      batchRoutingEnabled,
      qualityRoutingEnabled,
      mlRouterEnabled,
    },
  });
}
