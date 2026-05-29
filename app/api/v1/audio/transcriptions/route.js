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
  // No Content-Type — let fetch set it for multipart
  const headers = {
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
    const formData = await request.formData();
    const model = formData.get("model");
    const file = formData.get("file");

    if (!model) {
      return Response.json(
        { error: { message: "model is required", type: "invalid_request_error", param: "model", code: null } },
        { status: 400 }
      );
    }

    if (!file) {
      return Response.json(
        { error: { message: "file is required", type: "invalid_request_error", param: "file", code: null } },
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

      const transcriptionsPath = fullConn.transcriptions_path || "/v1/audio/transcriptions";
      const url = fullConn.base_url + transcriptionsPath;
      const startTime = Date.now();

      try {
        // Rebuild FormData to forward upstream
        const upstreamForm = new FormData();
        upstreamForm.append("file", file);
        upstreamForm.append("model", model);

        const language = formData.get("language");
        if (language) upstreamForm.append("language", language);

        const responseFormat = formData.get("response_format");
        if (responseFormat) upstreamForm.append("response_format", responseFormat);

        const prompt = formData.get("prompt");
        if (prompt) upstreamForm.append("prompt", prompt);

        const temperature = formData.get("temperature");
        if (temperature) upstreamForm.append("temperature", temperature);

        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), 120000);

        const headers = buildHeaders(fullConn);
        // Remove Content-Type so fetch auto-sets multipart boundary
        delete headers["Content-Type"];

        const upstream = await fetch(url, {
          method: "POST",
          headers,
          body: upstreamForm,
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

        const contentType = upstream.headers.get("content-type") || "";
        let data;
        if (contentType.includes("application/json")) {
          data = await upstream.json();
        } else {
          // Plain text response (e.g., when response_format=text)
          const text = await upstream.text();
          data = { text };
        }

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
