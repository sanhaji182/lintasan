/**
 * Plugin Store API
 * GET  /api/plugins/store — list all store plugins (supports ?category=xxx filter)
 * POST /api/plugins/store — install a store plugin by ID
 */

import { getStorePlugins, getStorePlugin, getStoreCategories } from "@/lib/plugin-store";
import { loadPlugins } from "@/lib/plugins";
import { writeFile, mkdir } from "fs/promises";
import path from "path";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const PLUGINS_DIR = path.resolve(process.cwd(), "plugins");

export async function GET(request) {
  const { searchParams } = new URL(request.url);
  const category = searchParams.get("category");

  const plugins = getStorePlugins(category || undefined);
  const categories = getStoreCategories();

  return new Response(
    JSON.stringify({ plugins, categories }),
    { status: 200, headers: { "Content-Type": "application/json" } }
  );
}

export async function POST(request) {
  const body = await request.json();
  const { action, id } = body;

  if (action !== "install") {
    return new Response(
      JSON.stringify({ error: "Required: { action: 'install', id: string }" }),
      { status: 400, headers: { "Content-Type": "application/json" } }
    );
  }

  if (!id) {
    return new Response(
      JSON.stringify({ error: "Plugin ID is required" }),
      { status: 400, headers: { "Content-Type": "application/json" } }
    );
  }

  const plugin = getStorePlugin(id);
  if (!plugin) {
    return new Response(
      JSON.stringify({ error: `Store plugin "${id}" not found` }),
      { status: 404, headers: { "Content-Type": "application/json" } }
    );
  }

  // Ensure plugins directory exists
  try {
    await mkdir(PLUGINS_DIR, { recursive: true });
  } catch (e) {
    // ignore if exists
  }

  const fileName = `${id}.js`;
  const filePath = path.join(PLUGINS_DIR, fileName);

  try {
    await writeFile(filePath, plugin.code, "utf-8");
    await loadPlugins();
    return new Response(
      JSON.stringify({ ok: true, id, name: plugin.name, action: "installed" }),
      { status: 201, headers: { "Content-Type": "application/json" } }
    );
  } catch (err) {
    return new Response(
      JSON.stringify({ error: "Failed to install plugin: " + err.message }),
      { status: 500, headers: { "Content-Type": "application/json" } }
    );
  }
}
