import { findModelConnections, getConnection, getActiveConnections, listDiscoveredModels, logRequest } from "@/lib/db";
import { validateApiKey } from "@/lib/auth";
import { getCacheKey, getCachedResponse, setCachedResponse, isCacheEnabled, getCacheTTL } from "@/lib/cache";
import { replayAsStream } from "@/lib/stream-cache";
import { checkRateLimit, resolveModelAlias, getFallbackChain } from "@/lib/router";
import { getProviderTimeout, getRetryConfig, getBackoffDelay, sleep } from "@/lib/provider-config";
import { trackError } from "@/lib/webhooks";
import { compressContext } from "@/lib/context-compressor";
import { findSemanticMatch, storeSemanticCache, isSemanticCacheEnabled } from "@/lib/semantic-cache";
import { findEmbeddingMatch, storeEmbeddingCache, isEmbeddingCacheEnabled } from "@/lib/embedding-cache";
import { smartMaxTokens } from "@/lib/smart-tokens";
import { isProviderAvailable, recordSuccess, recordFailure } from "@/lib/circuit-breaker";
import { pooledFetch } from "@/lib/connection-pool";
import { tryCoalesce, registerInflight } from "@/lib/request-coalescing";
import { optimizePromptOrder } from "@/lib/prompt-reorder";
import { checkBudget, recordUsage } from "@/lib/token-budget";
import { getCachedStream, replayCachedStream, createCapturingStream, isStreamCacheEnabled } from "@/lib/stream-response-cache";
import { isCombo, resolveComboModel, recordComboSuccess, recordComboFailure } from "@/lib/combo";
import { compressToolResults, isRtkEnabled } from "@/lib/rtk";
import { injectCavemanPrompt, isCavemanEnabled } from "@/lib/caveman";
import { recordQuotaUsage, isQuotaExhausted } from "@/lib/quota-tracking";
import { getNextKey, recordKeyUsage, cooldownKey } from "@/lib/multi-account";
import { routeByComplexity } from "@/lib/prompt-router.js";
import { isPromptOptimizerEnabled, optimizePrompt } from "@/lib/prompt-optimizer.js";
import { isChatSummaryEnabled, summarizeHistory } from "@/lib/chat-summary.js";
import { injectWebSearch, isWebSearchEnabled } from "@/lib/web-search.js";
import { buildCommandCodeBody, buildCommandCodeHeaders, createStreamResponse, createJsonResponse, isAgentMode, cleanModelName } from "@/lib/providers/commandcode-executor.js";
import { enhancePrompt } from "@/lib/prompt-enhancer.js";
import { getAdaptiveBudget, recordTokenUsage, classifyTask } from "@/lib/adaptive-budget.js";
import { scoreResponse, getQualityFilterStatus } from "@/lib/quality-filter.js";
import { getRetryModels, shouldRetry, recordRetryOutcome } from "@/lib/multishot-router.js";
import { compressContext as compressContextV2 } from "@/lib/context-compression-v2.js";
import { extractReasoningContent } from "@/lib/reasoning-extractor.js";
import { randomUUID } from "crypto";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Build headers for a connection
function buildHeaders(conn, body, agentMode = false) {
  const format = conn.format || "openai";
  
  // CommandCode needs special headers
  if (format === "commandcode") {
    return buildCommandCodeHeaders(conn.api_key, agentMode);
  }
  
  const headers = {
    "Content-Type": "application/json",
    ...(conn.extra_headers ? JSON.parse(conn.extra_headers) : {}),
  };
  if (conn.auth_header && conn.api_key) {
    headers[conn.auth_header] = (conn.auth_prefix || "") + conn.api_key;
  }
  return headers;
}

// Build request body based on format
function buildRequestBody(conn, model, body, stream, agentMode = false, thinkingMode = "auto") {
  const format = conn.format || "openai";

  if (format === "anthropic") {
    return {
      model,
      max_tokens: body.max_tokens || 16384,
      messages: body.messages.filter(m => m.role !== "system"),
      ...(body.messages.find(m => m.role === "system") && { system: body.messages.find(m => m.role === "system").content }),
      ...(stream && { stream: true }),
    };
  }

  if (format === "commandcode") {
    return buildCommandCodeBody(model, { ...body, stream }, agentMode, thinkingMode);
  }

  // OpenAI-compatible (default)
  return { ...body, model, stream };
}

// Parse response based on format
function parseResponse(format, data, model) {
  if (format === "anthropic") {
    return {
      id: data.id,
      object: "chat.completion",
      created: Math.floor(Date.now() / 1000),
      model: data.model || model,
      choices: [{
        index: 0,
        message: { role: "assistant", content: data.content?.map(c => c.text).join("") || "" },
        finish_reason: data.stop_reason === "end_turn" ? "stop" : data.stop_reason,
      }],
      usage: {
        prompt_tokens: data.usage?.input_tokens || 0,
        completion_tokens: data.usage?.output_tokens || 0,
        total_tokens: (data.usage?.input_tokens || 0) + (data.usage?.output_tokens || 0),
      },
    };
  }
  // OpenAI / CommandCode — already in correct format
  return data;
}

// Execute single request to a connection
async function executeRequest({ conn, model, body, stream, agentMode = false, thinkingMode = "auto" }) {
  const timeoutMs = getProviderTimeout(conn.name);
  const startTime = Date.now();
  const format = conn.format || "openai";
  const chatPath = conn.chat_path || "/v1/chat/completions";
  const url = conn.base_url + chatPath;

  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), timeoutMs);

    const headers = buildHeaders(conn, body, agentMode);
    const reqBody = buildRequestBody(conn, model, body, stream, agentMode, thinkingMode);

    const upstream = await fetch(url, {
      method: "POST",
      headers,
      body: JSON.stringify(reqBody),
      signal: controller.signal,
    });
    clearTimeout(timeout);
    const latencyMs = Date.now() - startTime;

    if (!upstream.ok) {
      const errorText = await upstream.text();
      logRequest({ connectionId: conn.id, provider: conn.name, model, status: upstream.status, latencyMs, error: errorText });
      return { ok: false, error: errorText, status: upstream.status, latencyMs };
    }

    // CommandCode always returns SSE stream — use executor to parse
    if (format === "commandcode") {
      if (stream) {
        logRequest({ connectionId: conn.id, provider: conn.name, model, status: 200, latencyMs });
        const streamResp = createStreamResponse(upstream, model);
        return { ok: true, response: streamResp, connId: conn.id, connName: conn.name };
      } else {
        const data = await createJsonResponse(upstream, model);
        // Reasoning extraction for CommandCode responses too
        extractReasoningContent(data);
        logRequest({ connectionId: conn.id, provider: conn.name, model, status: 200, inputTokens: data.usage?.prompt_tokens, outputTokens: data.usage?.completion_tokens, latencyMs });
        return { ok: true, data, connId: conn.id, connName: conn.name, latencyMs };
      }
    }

    if (stream) {
      logRequest({ connectionId: conn.id, provider: conn.name, model, status: 200, latencyMs });
      return {
        ok: true,
        response: new Response(upstream.body, {
          headers: { "Content-Type": "text/event-stream", "Cache-Control": "no-cache", "Connection": "keep-alive" },
        }),
        connId: conn.id,
        connName: conn.name,
      };
    }

    const rawData = await upstream.json();
    const data = parseResponse(format, rawData, model);
    
    // Reasoning extraction: if content is empty but reasoning_content exists, extract code
    extractReasoningContent(data);
    
    logRequest({ connectionId: conn.id, provider: conn.name, model, status: 200, inputTokens: data.usage?.prompt_tokens, outputTokens: data.usage?.completion_tokens, latencyMs });
    return { ok: true, data, connId: conn.id, connName: conn.name, latencyMs };

  } catch (error) {
    const latencyMs = Date.now() - startTime;
    const errMsg = error.name === "AbortError" ? "Request timeout (" + timeoutMs + "ms)" : error.message;
    logRequest({ connectionId: conn.id, provider: conn.name, model, status: 0, latencyMs, error: errMsg });
    return { ok: false, error: errMsg, status: 504, latencyMs };
  }
}

// Execute with retry + backoff
async function executeWithRetry({ conn, model, body, stream, agentMode = false, thinkingMode = "auto" }) {
  const { maxRetries, retryDelayMs, retryOnStatus } = getRetryConfig();

  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    const result = await executeRequest({ conn, model, body, stream, agentMode, thinkingMode });
    if (result.ok) return result;
    if (!retryOnStatus.includes(result.status)) return result;
    if (stream) return result;
    if (attempt === maxRetries) return result;
    const delay = getBackoffDelay(attempt, retryDelayMs);
    await sleep(delay);
  }
}

// Find connections that can serve a model
function resolveModelToConnections(model) {
  // 1. Check discovered models in DB
  const matches = findModelConnections(model);
  if (matches.length > 0) {
    return matches.map(m => ({
      id: m.connection_id,
      name: m.name,
      base_url: m.base_url,
      api_key: m.api_key,
      format: m.format,
      chat_path: m.chat_path,
      auth_header: m.auth_header,
      auth_prefix: m.auth_prefix,
      extra_headers: m.extra_headers,
    }));
  }

  // 2. Fallback: try all active connections (model might exist but not yet synced)
  const active = getActiveConnections();
  return active;
}

export async function POST(request) {
  // Auth (master key + user keys)
  const auth = validateApiKey(request);
  if (!auth.valid) {
    return Response.json({ error: { message: auth.reason || "Invalid API key." } }, { status: 401 });
  }

  // Rate limit
  const rateCheck = checkRateLimit(auth.token);
  if (!rateCheck.allowed) {
    return Response.json(
      { error: { message: "Rate limit exceeded. Try again in " + rateCheck.resetIn + "s" } },
      { status: 429, headers: { "Retry-After": String(rateCheck.resetIn) } }
    );
  }

  try {
    const body = await request.json();
    let { model, stream = false } = body;

    if (!model) {
      return Response.json({ error: { message: "model is required" } }, { status: 400 });
    }

    // Read thinking mode setting
    let thinkingMode = "auto";
    try {
      const { getDb } = await import("@/lib/db/index.js");
      const row = getDb().prepare("SELECT value FROM settings WHERE key='thinking_mode'").get();
      if (row) thinkingMode = row.value;
    } catch {}

    // Detect CommandCode agent mode (header or model suffix)
    const reqHeaders = Object.fromEntries(request.headers.entries());
    const agentMode = isAgentMode(model, reqHeaders);
    // Strip -agent suffix from model for actual API call
    if (model.endsWith("-agent")) {
      model = cleanModelName(model);
      body.model = model;
    }

    // Model alias resolution
    const alias = resolveModelAlias(model);
    if (alias) {
      model = alias.model;
      body.model = model;
    }

    // Prompt routing / auto-model selection (after alias, before combo)
    const routingResult = routeByComplexity(body.messages);
    if (routingResult) {
      model = routingResult.model;
      body.model = model;
    }

    // Combo resolution — if model is a combo name, resolve to sequence
    let comboMode = false;
    let comboName = null;
    let comboSequence = null;
    if (isCombo(model)) {
      comboMode = true;
      comboName = model;
      const resolved = resolveComboModel(model);
      if (!resolved) {
        return Response.json({ error: { message: `Combo "${model}" has no models configured` } }, { status: 400 });
      }
      comboSequence = resolved;
      // Use first model in sequence as starting point
      model = comboSequence.models[comboSequence.startIndex].model;
      body.model = model;
    }

    // Token budget check
    const budgetCheck = checkBudget(auth.token);
    if (!budgetCheck.allowed) {
      return Response.json(
        { error: { message: `Token budget exceeded: ${budgetCheck.reason}`, usage: budgetCheck.usage, limit: budgetCheck.limit } },
        { status: 429 }
      );
    }
    if (budgetCheck.downgrade) {
      model = budgetCheck.model;
      body.model = model;
    }

    // Smart max_tokens
    const smartMax = smartMaxTokens(body.messages, body.max_tokens);
    if (smartMax !== undefined) {
      body.max_tokens = smartMax;
    } else {
      delete body.max_tokens; // Don't send max_tokens — let model decide
    }

    // === INTELLIGENCE LAYER ===
    // Skip heavy processing for CommandCode format (it has its own server-side processing)
    // Quick check: if model resolves to a commandcode connection, skip middleware
    const quickConns = resolveModelToConnections(model);
    const isCommandCodeRequest = quickConns.some(c => c.format === "commandcode");
    const taskCategory = classifyTask(body.messages);

    if (!isCommandCodeRequest) {
      // 1. Prompt Enhancement — inject meta-instructions for better quality
      body.messages = enhancePrompt(body.messages);

      // 2. Adaptive Token Budget — override smart tokens with learned optimal
      const adaptiveBudget = getAdaptiveBudget(body.messages, model, body.max_tokens);
      if (adaptiveBudget.source === "adaptive") {
        body.max_tokens = adaptiveBudget.maxTokens;
      }
      // 3. Context Compression v2 — reduce noise in long prompts
      const compressionResult = compressContextV2(body.messages);
      if (compressionResult.compressed) {
        body.messages = compressionResult.messages;
      }

      // Context compression
      body.messages = compressContext(body.messages);

      // RTK — compress tool_result content (20-40% input savings)
      if (isRtkEnabled()) {
        body.messages = compressToolResults(body.messages);
      }

      // Prompt optimizer (after RTK, before caveman)
      if (isPromptOptimizerEnabled()) {
        const optimized = optimizePrompt(body.messages);
        body.messages = optimized.messages;
      }

      // Caveman Mode — inject terse system prompt (up to 65% output savings)
      if (isCavemanEnabled()) {
        body.messages = injectCavemanPrompt(body.messages);
      }

      // Chat summary mode (condense history for better caching)
      if (isChatSummaryEnabled()) {
        body.messages = summarizeHistory(body.messages);
      }

      // Prompt reordering
      body.messages = optimizePromptOrder(body.messages);
    }

    // Web search injection (before cache, adds search context)
    if (isWebSearchEnabled()) {
      body.messages = await injectWebSearch(body.messages);
    }

    // Cache check (exact match)
    const cacheEnabled = isCacheEnabled();
    let cacheHash = null;

    if (cacheEnabled) {
      cacheHash = getCacheKey(model, body.messages, { temperature: body.temperature, max_tokens: body.max_tokens, top_p: body.top_p });
      const cached = getCachedResponse(cacheHash);
      if (cached) {
        logRequest({ connectionId: "cache", provider: "cache", model, status: 200, inputTokens: cached.usage?.prompt_tokens, outputTokens: cached.usage?.completion_tokens, latencyMs: 1 });
        if (stream) return replayAsStream(cached, model);
        return Response.json(cached, { headers: { "X-Provider": "cache", "X-Cache": "HIT" } });
      }
    }

    // Streaming cache check
    if (stream && isStreamCacheEnabled()) {
      const cachedStream = getCachedStream(model, body.messages, { temperature: body.temperature, max_tokens: body.max_tokens });
      if (cachedStream) {
        logRequest({ connectionId: "stream-cache", provider: "stream-cache", model, status: 200, latencyMs: 1 });
        return replayCachedStream(cachedStream, model);
      }
    }

    // Embedding cache check (more accurate, checked first)
    if (!stream && isEmbeddingCacheEnabled()) {
      const embeddingHit = await findEmbeddingMatch(model, body.messages);
      if (embeddingHit) {
        logRequest({ connectionId: "embedding-cache", provider: "embedding-cache", model, status: 200, inputTokens: embeddingHit.response.usage?.prompt_tokens, outputTokens: embeddingHit.response.usage?.completion_tokens, latencyMs: 3 });
        return Response.json(embeddingHit.response, { headers: { "X-Provider": "embedding-cache", "X-Cache": "EMBEDDING-HIT", "X-Similarity": String(embeddingHit.score.toFixed(4)) } });
      }
    }

    // Semantic cache check (TF-IDF fallback)
    if (!stream && isSemanticCacheEnabled()) {
      const semanticHit = findSemanticMatch(model, body.messages);
      if (semanticHit) {
        logRequest({ connectionId: "semantic-cache", provider: "semantic-cache", model, status: 200, inputTokens: semanticHit.response.usage?.prompt_tokens, outputTokens: semanticHit.response.usage?.completion_tokens, latencyMs: 2 });
        return Response.json(semanticHit.response, { headers: { "X-Provider": "semantic-cache", "X-Cache": "SEMANTIC-HIT", "X-Similarity": String(semanticHit.score.toFixed(3)) } });
      }
    }

    // Request coalescing
    if (!stream) {
      const coalesce = tryCoalesce(model, body.messages, { temperature: body.temperature, max_tokens: body.max_tokens });
      if (coalesce.coalesced) {
        const coalescedResult = await coalesce.promise;
        if (coalescedResult) {
          return Response.json(coalescedResult, { headers: { "X-Provider": "coalesced", "X-Cache": "COALESCED" } });
        }
      }
    }

    // Resolve model → connections (from discovered models DB)
    let connections;
    if (comboMode && comboSequence) {
      // Combo mode: build connection list from combo sequence
      connections = [];
      const startIdx = comboSequence.startIndex;
      for (let i = 0; i < comboSequence.models.length; i++) {
        const idx = (startIdx + i) % comboSequence.models.length;
        const comboEntry = comboSequence.models[idx];
        const entryConns = resolveModelToConnections(comboEntry.model);
        for (const conn of entryConns) {
          // Skip if quota exhausted
          if (isQuotaExhausted(conn.id)) continue;
          connections.push({ ...conn, _comboModel: comboEntry.model, _comboIndex: idx });
        }
      }
    } else {
      connections = resolveModelToConnections(model);
      // Filter out quota-exhausted connections
      connections = connections.filter(c => !isQuotaExhausted(c.id));
    }

    if (connections.length === 0) {
      return Response.json(
        { error: { message: `No provider found for model: ${model}. Add a connection and sync models first.` } },
        { status: 404 }
      );
    }

    // Execute with fallback across connections
    let resolveCoalesce;
    const coalescePromise = new Promise((resolve) => { resolveCoalesce = resolve; });
    const coalesceHash = !stream ? tryCoalesce(model, body.messages, { temperature: body.temperature, max_tokens: body.max_tokens }).hash : null;
    if (coalesceHash && !stream) {
      registerInflight(coalesceHash, coalescePromise);
    }

    let lastError = null;
    for (const conn of connections) {
      // Circuit breaker check (by connection id)
      if (!isProviderAvailable(conn.id)) {
        lastError = { error: `Connection ${conn.name} circuit open`, status: 503 };
        continue;
      }

      // Get full connection data if we only have partial
      const fullConn = conn.api_key ? conn : getConnection(conn.id);
      if (!fullConn) continue;

      // For combo mode, use the model from the combo entry
      const requestModel = conn._comboModel || model;
      const requestBody = { ...body, model: requestModel };

      // Multi-account: get next available key
      const { key: activeKey, keyId } = getNextKey(fullConn.id, fullConn.api_key);
      const connWithKey = { ...fullConn, api_key: activeKey };

      const result = await executeWithRetry({ conn: connWithKey, model: requestModel, body: requestBody, stream, agentMode, thinkingMode });

      if (result.ok) {
        recordSuccess(fullConn.id);

        if (result.data?.usage) {
          recordUsage(auth.token, model, result.data.usage.prompt_tokens, result.data.usage.completion_tokens);
          // Quota tracking
          recordQuotaUsage(fullConn.id, result.data.usage.prompt_tokens, result.data.usage.completion_tokens);
          // 2. Adaptive budget learning
          recordTokenUsage(taskCategory, model, body.max_tokens, result.data.usage.completion_tokens, result.data.choices?.[0]?.finish_reason);
        }

        // 4. Quality Filter + 5. Multi-shot Routing (non-stream only)
        if (!stream && result.data) {
          const finishReason = result.data.choices?.[0]?.finish_reason;
          const qualityResult = scoreResponse(result.data, body.messages, finishReason);

          if (shouldRetry(qualityResult, 0, null)) {
            // Try alternative model
            const alternatives = getRetryModels(requestModel);
            if (alternatives.length > 0) {
              const alt = alternatives[0];
              const altConn = getConnection(alt.connectionId);
              if (altConn) {
                const altBody = { ...body, model: alt.model };
                const altResult = await executeWithRetry({ conn: altConn, model: alt.model, body: altBody, stream: false });
                if (altResult.ok) {
                  const altQuality = scoreResponse(altResult.data, body.messages, altResult.data?.choices?.[0]?.finish_reason);
                  recordRetryOutcome(requestModel, alt.model, qualityResult.score, altQuality.score, taskCategory);
                  // Use retry result if better
                  if (altQuality.score > qualityResult.score) {
                    result.data = altResult.data;
                    result.connName = alt.connectionName;
                  }
                }
              }
            }
          }
        }

        // Combo success tracking
        if (comboMode && comboName && conn._comboIndex !== undefined) {
          recordComboSuccess(comboName, conn._comboIndex);
        }

        if (!stream && cacheEnabled && cacheHash && result.data) {
          setCachedResponse(cacheHash, {
            provider: result.connName, model, requestBody: body, responseBody: result.data,
            inputTokens: result.data.usage?.prompt_tokens, outputTokens: result.data.usage?.completion_tokens,
            ttlSeconds: getCacheTTL(),
          });
        }

        if (!stream && result.data) {
          storeSemanticCache(model, body.messages, result.data);
          storeEmbeddingCache(model, body.messages, result.data);
          if (resolveCoalesce) resolveCoalesce(result.data);
        }

        if (stream) {
          if (isStreamCacheEnabled()) {
            return createCapturingStream(result.response.body, model, body.messages, { temperature: body.temperature, max_tokens: body.max_tokens });
          }
          const headers = new Headers(result.response.headers);
          headers.set("X-Provider", result.connName || "");
          headers.set("X-Cache", "MISS");
          return new Response(result.response.body, { status: 200, headers });
        }

        return Response.json(result.data, {
          headers: { "X-Provider": result.connName || "", "X-Connection": result.connId || "", "X-Cache": "MISS", "X-Latency": String(result.latencyMs || 0) },
        });
      }

      recordFailure(fullConn.id);
      lastError = result;
      trackError(fullConn.name, result.error, model);

      // Combo failure tracking
      if (comboMode && comboName && conn._comboIndex !== undefined) {
        recordComboFailure(comboName, conn._comboIndex);
      }

      // Don't fallback on 4xx (except 429)
      if (result.status >= 400 && result.status < 500 && result.status !== 429) break;
    }

    if (resolveCoalesce) resolveCoalesce(null);

    return Response.json(
      { error: { message: lastError?.error || "All providers failed", connections_tried: connections.map(c => c.name) } },
      { status: lastError?.status || 502 }
    );

  } catch (error) {
    return Response.json({ error: { message: "Internal proxy error: " + error.message } }, { status: 500 });
  }
}
