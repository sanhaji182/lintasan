import { validateDashboardSession } from "@/lib/auth";
import {
  getAllFallbackChains,
  getFallbackModels,
  getFallbackConnections,
  setFallbackModels,
  setFallbackConnections,
  deleteFallbackChain,
  getFallbackStats,
} from "@/lib/fallback-chain.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const type = searchParams.get("type"); // 'model', 'connection', or null for all
  const id = searchParams.get("id"); // specific chain lookup
  const stats = searchParams.get("stats"); // 'true' to get metrics
  const hours = parseInt(searchParams.get("hours") || "24", 10);

  // Return fallback stats
  if (stats === "true") {
    const statsData = getFallbackStats({
      from: id || undefined,
      chainType: type || undefined,
      hours,
    });
    return Response.json({ data: statsData });
  }

  // Return specific chain
  if (id) {
    if (type === "model") {
      return Response.json({ data: { id, type: "model", fallbacks: getFallbackModels(id) } });
    }
    if (type === "connection") {
      return Response.json({ data: { id, type: "connection", fallbacks: getFallbackConnections(id) } });
    }
    // Return both if type not specified
    return Response.json({
      data: {
        model: getFallbackModels(id),
        connection: getFallbackConnections(id),
      },
    });
  }

  // Return all chains
  const chains = getAllFallbackChains();
  if (type === "model") {
    return Response.json({ data: chains.model_chains });
  }
  if (type === "connection") {
    return Response.json({ data: chains.connection_chains });
  }
  return Response.json({ data: chains });
}

export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const { type, id, fallbacks } = body;

  if (!type || !id) {
    return Response.json(
      { error: "type ('model' or 'connection') and id are required" },
      { status: 400 }
    );
  }

  if (!Array.isArray(fallbacks)) {
    return Response.json(
      { error: "fallbacks must be an array of IDs" },
      { status: 400 }
    );
  }

  if (type === "model") {
    setFallbackModels(id, fallbacks);
  } else if (type === "connection") {
    setFallbackConnections(id, fallbacks);
  } else {
    return Response.json(
      { error: "type must be 'model' or 'connection'" },
      { status: 400 }
    );
  }

  return Response.json({ success: true, id, type, fallbacks });
}

export async function PUT(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  // PUT behaves the same as POST for upsert semantics
  const body = await request.json();
  const { type, id, fallbacks } = body;

  if (!type || !id) {
    return Response.json(
      { error: "type ('model' or 'connection') and id are required" },
      { status: 400 }
    );
  }

  if (!Array.isArray(fallbacks)) {
    return Response.json(
      { error: "fallbacks must be an array of IDs" },
      { status: 400 }
    );
  }

  if (type === "model") {
    setFallbackModels(id, fallbacks);
  } else if (type === "connection") {
    setFallbackConnections(id, fallbacks);
  } else {
    return Response.json(
      { error: "type must be 'model' or 'connection'" },
      { status: 400 }
    );
  }

  return Response.json({ success: true, id, type, fallbacks });
}

export async function DELETE(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const type = searchParams.get("type");
  const id = searchParams.get("id");

  if (!type || !id) {
    return Response.json(
      { error: "type and id query params are required" },
      { status: 400 }
    );
  }

  if (type !== "model" && type !== "connection") {
    return Response.json(
      { error: "type must be 'model' or 'connection'" },
      { status: 400 }
    );
  }

  deleteFallbackChain(type, id);
  return Response.json({ success: true, deleted: { type, id } });
}
