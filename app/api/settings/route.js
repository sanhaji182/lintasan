import { getSetting, setSetting } from "@/lib/db/index.js";
import { getDb } from "@/lib/db/index.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// GET — return all settings
export async function GET() {
  try {
    const db = getDb();
    const rows = db.prepare("SELECT key, value FROM settings").all();
    const data = {};
    for (const row of rows) {
      data[row.key] = row.value;
    }
    return Response.json({ data });
  } catch (error) {
    return Response.json({ error: error.message }, { status: 500 });
  }
}

// POST — save a single setting
export async function POST(request) {
  try {
    const { key, value } = await request.json();
    if (!key || value === undefined) {
      return Response.json({ error: "key and value are required" }, { status: 400 });
    }
    setSetting(key, String(value));
    return Response.json({ ok: true, key, value: String(value) });
  } catch (error) {
    return Response.json({ error: error.message }, { status: 500 });
  }
}
