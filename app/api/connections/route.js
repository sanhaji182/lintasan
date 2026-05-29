import { validateDashboardSession } from "@/lib/auth";
import { createConnection, listConnections, deleteConnection, updateConnection, getConnection } from "@/lib/db";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  try {
    const connections = listConnections();
    return Response.json({ data: connections });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}

export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  try {
    const body = await request.json();
    const { name, baseUrl, apiKey, format, chatPath, modelsPath, authHeader, authPrefix, extraHeaders, priority } = body;

    if (!name || !baseUrl || !apiKey) {
      return Response.json({ error: { message: "name, baseUrl, and apiKey are required" } }, { status: 400 });
    }

    const conn = createConnection({
      name,
      baseUrl: baseUrl.replace(/\/+$/, ""), // strip trailing slash
      apiKey,
      format: format || "openai",
      chatPath: chatPath || "/v1/chat/completions",
      modelsPath: modelsPath || "/v1/models",
      authHeader: authHeader || "Authorization",
      authPrefix: authPrefix || "Bearer ",
      extraHeaders: extraHeaders || "{}",
      priority: priority || 0,
    });
    return Response.json({ data: conn }, { status: 201 });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}

export async function PATCH(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  try {
    const body = await request.json();
    const { id, ...updates } = body;
    if (!id) return Response.json({ error: { message: "id is required" } }, { status: 400 });
    if (updates.baseUrl) updates.baseUrl = updates.baseUrl.replace(/\/+$/, "");
    updateConnection(id, updates);
    return Response.json({ success: true });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}

export async function DELETE(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  try {
    const { searchParams } = new URL(request.url);
    const id = searchParams.get("id");
    if (!id) return Response.json({ error: { message: "id is required" } }, { status: 400 });
    deleteConnection(id);
    return Response.json({ success: true });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}
