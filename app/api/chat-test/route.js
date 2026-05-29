import { getProvider } from "@/lib/providers/registry";
import { listConnections, getConnection } from "@/lib/db";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(request) {
  try {
    const { provider: providerId, model, message } = await request.json();

    const provider = getProvider(providerId);
    if (!provider) {
      return Response.json({ error: "Unknown provider" }, { status: 400 });
    }

    const connections = listConnections(providerId);
    const activeConn = connections.find((c) => c.is_active);
    if (!activeConn) {
      return Response.json({ error: "No active connection for " + providerId }, { status: 503 });
    }

    const fullConn = getConnection(activeConn.id);
    const url = provider.baseUrl + provider.chatPath;

    const res = await fetch(url, {
      method: "POST",
      headers: {
        ...provider.headers,
        [provider.authHeader]: provider.authPrefix + fullConn.api_key,
      },
      body: JSON.stringify({
        model: model || provider.defaultModels[0],
        messages: [{ role: "user", content: message }],
        stream: false,
      }),
    });

    if (!res.ok) {
      const errText = await res.text();
      return Response.json({ error: errText }, { status: res.status });
    }

    const data = await res.json();
    return Response.json({ data });
  } catch (error) {
    return Response.json({ error: error.message }, { status: 500 });
  }
}
