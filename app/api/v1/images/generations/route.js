import { validateApiKey } from "@/lib/auth";
import { findModelConnections, getActiveConnections, getConnection, logRequest } from "@/lib/db";
import { isProviderAvailable, recordSuccess, recordFailure } from "@/lib/circuit-breaker";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

function resolveModelToConnections(model) {
  const matches = findModelConnections(model);
  if (matches.length > 0) {
    return matches.map(m => ({
      id: m.connection_id,
      name: m.name,
      base_url: m.base_url,
      api_key: m.api_key,
      format: m.format,
      auth_header: m.auth_header,
      auth_prefix: m.auth_prefix,
      extra_headers: m.extra_headers,
    }));
  }
  return getActiveConnections();
}

function buildHeaders(conn) {
  const headers = {
    "Content-Type": "application/json",
    ...(conn.extra_headers ? JSON.parse(conn.extra_headers) : {}),
  };
  if (conn.auth_header && conn.api_key) {
    headers[conn.auth_header] = (conn.auth_prefix || "") + conn.api_key;
  }
  return headers;
}

export async function POST(request) {
  const auth = validateApiKey(request);
  if (!auth.valid) {
    return Response.json(
      { error: { message: auth.reason || "Invalid API key.", type: "invalid_request_error", param: null, code: "invalid_api_key" } },
      { status: 401 }
    );
  }

  try {
    const body = await request.json();
    const { model, prompt, n, size, quality, response_format } = body;

    if (!model) {
      return Response.json(
        { error: { message: "model is required", type: "invalid_request_error", param: "model", code: null } },
        { status: 400 }
      );
    }

    if (!prompt) {
      return Response.json(
        { error: { message: "prompt is required", type: "invalid_request_error", param: "prompt", code: null } },
        { status: 400 }
      );
    }

    const connections = resolveModelToConnections(model);
    if (connections.length === 0) {
      return Response.json(
        { error: { message: `No provider found for model: ${model}`, type: "invalid_request_error", param: "model", code: "model_not_found" } },
        { status: 404 }
      );
    }

    let lastError = null;

    for (const conn of connections) {
      if (!isProviderAvailable(conn.id)) {
        lastError = { error: `Connection ${conn.name} circuit open`, status: 503 };
        continue;
      }

      const fullConn = conn.api_key ? conn : getConnection(conn.id);
      if (!fullConn) continue;

      const imagesPath = fullConn.images_path || "/v1/images/generations";
      const url = fullConn.base_url + imagesPath;
      const startTime = Date.now();

      try {
        const reqBody = { model, prompt };
        if (n != null) reqBody.n = n;
        if (size) reqBody.size = size;
        if (quality) reqBody.quality = quality;
        if (response_format) reqBody.response_format = response_format;

        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), 120000);

        const upstream = await fetch(url, {
          method: "POST",
          headers: buildHeaders(fullConn),
          body: JSON.stringify(reqBody),
          signal: controller.signal,
        });
        clearTimeout(timeout);

        const latencyMs = Date.now() - startTime;

        if (!upstream.ok) {
          const errorText = await upstream.text();
          logRequest({ connectionId: fullConn.id, provider: fullConn.name, model, status: upstream.status, latencyMs, error: errorText });
          recordFailure(fullConn.id);
          lastError = { error: errorText, status: upstream.status };
          if (upstream.status >= 400 && upstream.status < 500 && upstream.status !== 429) break;
          continue;
        }

        const data = await upstream.json();
        logRequest({ connectionId: fullConn.id, provider: fullConn.name, model, status: 200, latencyMs });
        recordSuccess(fullConn.id);

        return Response.json(data, {
          headers: { "X-Provider": fullConn.name || "" },
        });
      } catch (error) {
        const latencyMs = Date.now() - startTime;
        const errMsg = error.name === "AbortError" ? "Request timeout" : error.message;
        logRequest({ connectionId: fullConn.id, provider: fullConn.name, model, status: 0, latencyMs, error: errMsg });
        recordFailure(fullConn.id);
        lastError = { error: errMsg, status: 504 };
      }
    }

    return Response.json(
      { error: { message: lastError?.error || "All providers failed", type: "server_error", param: null, code: null } },
      { status: lastError?.status || 502 }
    );
  } catch (error) {
    return Response.json(
      { error: { message: "Internal proxy error: " + error.message, type: "server_error", param: null, code: null } },
      { status: 500 }
    );
  }
}
