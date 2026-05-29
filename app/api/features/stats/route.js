import { validateDashboardSession } from "@/lib/auth";
import { getDb } from "@/lib/db";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// GET — return stats for all features
export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const db = getDb();

  // Semantic cache stats
  let semanticCacheEntries = 0;
  let semanticCacheHits = 0;
  try {
    const sc = db.prepare("SELECT COUNT(*) as cnt, COALESCE(SUM(hits), 0) as hits FROM semantic_cache").get();
    semanticCacheEntries = sc?.cnt || 0;
    semanticCacheHits = sc?.hits || 0;
  } catch {}

  // Batch queue stats
  let batchQueued = 0;
  let batchProcessed = 0;
  try {
    const bq = db.prepare("SELECT status, COUNT(*) as cnt FROM batch_queue GROUP BY status").all();
    for (const row of bq) {
      if (row.status === "queued") batchQueued = row.cnt;
      if (row.status === "completed") batchProcessed = row.cnt;
    }
  } catch {}

  // Token usage stats
  let todayTokens = 0;
  let monthTokens = 0;
  let totalUsageLogs = 0;
  try {
    const today = db.prepare("SELECT COALESCE(SUM(input_tokens + output_tokens), 0) as total FROM token_usage_log WHERE created_at >= date('now')").get();
    todayTokens = today?.total || 0;

    const month = db.prepare("SELECT COALESCE(SUM(input_tokens + output_tokens), 0) as total FROM token_usage_log WHERE created_at >= date('now', 'start of month')").get();
    monthTokens = month?.total || 0;

    const logs = db.prepare("SELECT COUNT(*) as cnt FROM token_usage_log").get();
    totalUsageLogs = logs?.cnt || 0;
  } catch {}

  return Response.json({
    data: {
      semanticCacheEntries,
      semanticCacheHits,
      batchQueued,
      batchProcessed,
      todayTokens,
      monthTokens,
      totalUsageLogs,
    },
  });
}
