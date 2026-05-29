// Web Search API - settings and test endpoint
import { isWebSearchEnabled, needsWebSearch, searchDuckDuckGo, formatSearchContext } from "@/lib/web-search.js";
import { getDb } from "@/lib/db/index.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const enabled = isWebSearchEnabled();
    return Response.json({ enabled });
  } catch (error) {
    return Response.json({ error: error.message }, { status: 500 });
  }
}

export async function POST(request) {
  try {
    const body = await request.json();

    // Toggle enable/disable
    if (body.enabled !== undefined) {
      const db = getDb();
      db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("web_search_enabled", body.enabled ? "true" : "false");
      return Response.json({ success: true, enabled: body.enabled });
    }

    // Test search
    if (body.query) {
      const wouldSearch = needsWebSearch(body.query);
      const results = await searchDuckDuckGo(body.query);
      const context = formatSearchContext(results);
      return Response.json({
        query: body.query,
        wouldSearch,
        resultsCount: results.length,
        results,
        context,
      });
    }

    return Response.json({ error: "Provide 'enabled' or 'query'" }, { status: 400 });
  } catch (error) {
    return Response.json({ error: error.message }, { status: 500 });
  }
}
