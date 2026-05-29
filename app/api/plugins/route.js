/**
 * Plugin management API
 * GET  /api/plugins — list all plugins with status
 * GET  /api/plugins?name=xxx — get plugin source code
 * POST /api/plugins — enable/disable or create/delete a plugin
 */

import { loadPlugins, listPlugins, enablePlugin, disablePlugin } from "@/lib/plugins";
import { writeFile, unlink, readFile } from "fs/promises";
import path from "path";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const PLUGINS_DIR = path.resolve(process.cwd(), "plugins");

export async function GET(request) {
  const { searchParams } = new URL(request.url);
  const name = searchParams.get("name");

  // Return source code of a specific plugin
  if (name) {
    const safeName = name.replace(/[^a-zA-Z0-9_-]/g, "");
    const filePath = path.join(PLUGINS_DIR, `${safeName}.js`);
    try {
      const code = await readFile(filePath, "utf-8");
      return new Response(JSON.stringify({ name: safeName, code }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    } catch (err) {
      return new Response(
        JSON.stringify({ error: `Plugin "${safeName}" not found` }),
        { status: 404, headers: { "Content-Type": "application/json" } }
      );
    }
  }

  // Hot-reload plugins from disk
  await loadPlugins();
  const plugins = listPlugins();

  return new Response(JSON.stringify({ plugins }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

export async function POST(request) {
  const body = await request.json();
  const { action, name, enabled, code } = body;

  // Create new plugin
  if (action === "create" || action === "update") {
    if (!name || !code) {
      return new Response(
        JSON.stringify({ error: `Required: { action: '${action}', name: string, code: string }` }),
        { status: 400, headers: { "Content-Type": "application/json" } }
      );
    }
    const safeName = name.replace(/[^a-zA-Z0-9_-]/g, "");
    if (!safeName) {
      return new Response(
        JSON.stringify({ error: "Invalid plugin name" }),
        { status: 400, headers: { "Content-Type": "application/json" } }
      );
    }
    const filePath = path.join(PLUGINS_DIR, `${safeName}.js`);
    try {
      await writeFile(filePath, code, "utf-8");
      await loadPlugins();
      return new Response(
        JSON.stringify({ ok: true, name: safeName, action: action === "update" ? "updated" : "created" }),
        { status: action === "update" ? 200 : 201, headers: { "Content-Type": "application/json" } }
      );
    } catch (err) {
      return new Response(
        JSON.stringify({ error: "Failed to write plugin: " + err.message }),
        { status: 500, headers: { "Content-Type": "application/json" } }
      );
    }
  }

  // Delete plugin
  if (action === "delete") {
    if (!name) {
      return new Response(
        JSON.stringify({ error: "Required: { action: 'delete', name: string }" }),
        { status: 400, headers: { "Content-Type": "application/json" } }
      );
    }
    const safeName = name.replace(/[^a-zA-Z0-9_-]/g, "");
    const filePath = path.join(PLUGINS_DIR, `${safeName}.js`);
    try {
      await unlink(filePath);
      await loadPlugins();
      return new Response(
        JSON.stringify({ ok: true, name: safeName, action: "deleted" }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      );
    } catch (err) {
      return new Response(
        JSON.stringify({ error: "Failed to delete plugin: " + err.message }),
        { status: 500, headers: { "Content-Type": "application/json" } }
      );
    }
  }

  // Enable/disable plugin (default action)
  if (!name || typeof enabled !== "boolean") {
    return new Response(
      JSON.stringify({ error: "Required: { name: string, enabled: boolean } or { action: 'create'|'delete', ... }" }),
      { status: 400, headers: { "Content-Type": "application/json" } }
    );
  }

  await loadPlugins();

  let success;
  if (enabled) {
    success = enablePlugin(name);
  } else {
    success = disablePlugin(name);
  }

  if (!success) {
    return new Response(
      JSON.stringify({ error: `Plugin "${name}" not found.` }),
      { status: 404, headers: { "Content-Type": "application/json" } }
    );
  }

  return new Response(
    JSON.stringify({ ok: true, name, enabled }),
    { status: 200, headers: { "Content-Type": "application/json" } }
  );
}
