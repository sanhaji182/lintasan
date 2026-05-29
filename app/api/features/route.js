import { getDb, getSetting } from "@/lib/db";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// GET — return all feature settings
export async function GET() {
  const db = getDb();
  const rows = db.prepare("SELECT key, value FROM settings").all();
  const data = {};
  for (const row of rows) {
    data[row.key] = row.value;
  }
  return Response.json({ data });
}

// POST — update a single feature setting
export async function POST(request) {
  const { key, value } = await request.json();
  if (!key) {
    return Response.json({ error: "key is required" }, { status: 400 });
  }

  const db = getDb();
  db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run(key, value);

  return Response.json({ ok: true, key, value });
}
