import { validateDashboardSession } from "@/lib/auth";
import {
  registerWebhook,
  removeWebhook,
  updateWebhook,
  listWebhooks,
  getWebhookById,
  testWebhook,
  getNotificationHistory,
  getErrorStats,
  EVENT_TYPES,
} from "@/lib/webhook-notifications";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const webhooks = listWebhooks();
  const history = getNotificationHistory(50);
  const errorStats = getErrorStats();

  return Response.json({
    data: {
      webhooks,
      notification_history: history,
      error_stats: errorStats,
      event_types: EVENT_TYPES,
    },
  });
}

export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const url = new URL(request.url);
  const action = url.searchParams.get("action");

  // Handle test action
  if (action === "test") {
    const id = url.searchParams.get("id");
    if (!id) {
      return Response.json({ error: "Missing webhook id" }, { status: 400 });
    }
    const result = await testWebhook(id);
    if (!result) {
      return Response.json({ error: "Webhook not found" }, { status: 404 });
    }
    return Response.json({ data: result });
  }

  // Register new webhook
  const body = await request.json();
  const { name, url: webhookUrl, secret, events } = body;

  if (!name || !webhookUrl) {
    return Response.json({ error: "name and url are required" }, { status: 400 });
  }

  try {
    new URL(webhookUrl);
  } catch {
    return Response.json({ error: "Invalid URL format" }, { status: 400 });
  }

  const webhook = registerWebhook({ name, url: webhookUrl, secret, events });
  return Response.json({ data: webhook }, { status: 201 });
}

export async function PUT(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const { id, ...updates } = body;

  if (!id) {
    return Response.json({ error: "Missing webhook id" }, { status: 400 });
  }

  if (updates.url) {
    try {
      new URL(updates.url);
    } catch {
      return Response.json({ error: "Invalid URL format" }, { status: 400 });
    }
  }

  const result = updateWebhook(id, updates);
  if (!result) {
    return Response.json({ error: "Webhook not found" }, { status: 404 });
  }

  return Response.json({ data: result });
}

export async function DELETE(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const url = new URL(request.url);
  let id = url.searchParams.get("id");

  // Also support JSON body
  if (!id) {
    try {
      const body = await request.json();
      id = body.id;
    } catch {
      // no body
    }
  }

  if (!id) {
    return Response.json({ error: "Missing webhook id" }, { status: 400 });
  }

  const removed = removeWebhook(id);
  if (!removed) {
    return Response.json({ error: "Webhook not found" }, { status: 404 });
  }

  return Response.json({ success: true });
}
