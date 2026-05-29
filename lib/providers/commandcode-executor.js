// CommandCode Executor - dual mode: standard (clean) and agent (full CC format)
// Standard mode: sends OpenAI-compatible body directly (like 9Router)
// Agent mode: wraps in threadId/config/params format (full CommandCode agent features)
import { randomUUID } from "crypto";

const COMMAND_CODE_VERSION = "0.26.25";
const MAX_COMMAND_CODE_TOKENS = 200000;
const DEFAULT_MAX_TOKENS = 16384;

// === Helpers ===

function normalizeContentText(content) {
  if (typeof content === "string") return content;
  if (Array.isArray(content)) {
    return content
      .filter((part) => part.type === "text")
      .map((part) => part.text || "")
      .join("\n");
  }
  return "";
}

function convertTools(tools) {
  if (!Array.isArray(tools)) return [];
  return tools.map((tool) => {
    const fn = tool.function || tool;
    return {
      type: "function",
      name: fn.name || "",
      description: fn.description || "",
      input_schema: fn.parameters || {},
    };
  });
}

function completeToolCallIds(messages) {
  const callIds = new Set();
  const resultIds = new Set();

  for (const message of messages) {
    if (message.role === "assistant" && Array.isArray(message.tool_calls)) {
      for (const call of message.tool_calls) {
        if (call.id) callIds.add(call.id);
      }
    } else if (message.role === "tool") {
      if (message.tool_call_id) resultIds.add(message.tool_call_id);
    }
  }

  return new Set([...callIds].filter((id) => resultIds.has(id)));
}

function recordOrEmpty(value) {
  if (typeof value === "object" && value !== null && !Array.isArray(value)) return value;
  if (typeof value === "string" && value.trim()) {
    try {
      const parsed = JSON.parse(value);
      if (typeof parsed === "object" && parsed !== null) return parsed;
    } catch {}
  }
  return {};
}

function convertMessages(messages) {
  if (!Array.isArray(messages)) return { system: "", messages: [] };
  const pairedToolCallIds = completeToolCallIds(messages);
  const out = [];
  const system = [];

  for (const message of messages) {
    const role = message.role;

    if (role === "system" || role === "developer") {
      const text = normalizeContentText(message.content);
      if (text) system.push(text);
      continue;
    }

    if (role === "user") {
      out.push({ role: "user", content: message.content ?? "" });
      continue;
    }

    if (role === "assistant") {
      const parts = [];
      const text = normalizeContentText(message.content);
      if (text) parts.push({ type: "text", text });

      if (Array.isArray(message.tool_calls)) {
        for (const call of message.tool_calls) {
          const id = call.id || "";
          if (!id || !pairedToolCallIds.has(id)) continue;
          const fn = call.function || {};
          parts.push({
            type: "tool-call",
            toolCallId: id,
            toolName: fn.name || "",
            input: recordOrEmpty(fn.arguments),
          });
        }
      }

      if (parts.length > 0) out.push({ role: "assistant", content: parts });
      continue;
    }

    if (role === "tool") {
      const toolCallId = message.tool_call_id || "";
      if (!toolCallId || !pairedToolCallIds.has(toolCallId)) continue;
      out.push({
        role: "tool",
        content: [
          {
            type: "tool-result",
            toolCallId,
            toolName: message.name || "",
            output: { type: "text", value: normalizeContentText(message.content) },
          },
        ],
      });
    }
  }

  return { system: system.join("\n\n"), messages: out };
}

// === Detect mode ===

/**
 * Determine if request should use agent mode.
 * Agent mode triggers:
 * - Header: x-commandcode-mode: agent
 * - Model suffix: -agent (e.g. deepseek/deepseek-v4-pro-agent)
 */
export function isAgentMode(model, headers = {}) {
  if (headers["x-commandcode-mode"] === "agent") return true;
  if (model && model.endsWith("-agent")) return true;
  return false;
}

/**
 * Strip -agent suffix from model name for actual API call
 */
export function cleanModelName(model) {
  if (model && model.endsWith("-agent")) {
    return model.slice(0, -6);
  }
  return model;
}

// === Build body: Standard mode (minimal CC format, like 9Router) ===

export function buildStandardBody(model, body, thinkingMode = "auto") {
  const converted = convertMessages(body.messages);
  // No anti-agent prefix, no extra system injection — just pass through client's system prompt
  const system = converted.system || "";

  // Detect DeepSeek reasoning models for thinking control
  const isDeepSeekReasoning = model && (model.includes("deepseek-v4-pro") || model.includes("deepseek-r1"));

  // If thinking is disabled, we don't need the 8192 floor — model won't generate reasoning
  const floorTokens = (isDeepSeekReasoning && thinkingMode === "disabled") ? 1024 : 8192;
  const rawMax = Math.floor(body.max_tokens || body.max_completion_tokens || DEFAULT_MAX_TOKENS);
  const maxTokens = Math.max(floorTokens, Math.min(rawMax, MAX_COMMAND_CODE_TOKENS));

  const result = {
    threadId: randomUUID(),
    memory: "",
    config: {
      workingDir: "/tmp",
      date: new Date().toISOString().slice(0, 10),
      environment: "linux",
      structure: [],
      isGitRepo: false,
      currentBranch: "",
      mainBranch: "",
      gitStatus: "",
      recentCommits: [],
    },
    params: {
      model,
      messages: converted.messages,
      system,
      max_tokens: maxTokens,
      ...(body.stream !== false && { stream: true }),
    },
  };

  // Thinking mode control for DeepSeek models (mirrors 9router's thinking.type injection)
  if (isDeepSeekReasoning && thinkingMode !== "auto") {
    result.params.thinking = { type: thinkingMode === "enabled" ? "enabled" : "disabled" };
    if (thinkingMode === "enabled") {
      result.params.reasoning_effort = "max";
    }
  }

  // Only include tools if present
  const tools = convertTools(body.tools);
  if (tools.length > 0) {
    result.params.tools = tools;
  }

  return result;
}

// === Build body: Agent mode (threadId/config/params format) ===

export function buildAgentBody(model, body, thinkingMode = "auto") {
  const converted = convertMessages(body.messages);
  const explicitSystem = typeof body.system === "string" ? body.system : "";
  const system = [converted.system, explicitSystem].filter(Boolean).join("\n\n");

  // Detect DeepSeek reasoning models for thinking control
  const isDeepSeekReasoning = model && (model.includes("deepseek-v4-pro") || model.includes("deepseek-r1"));

  const floorTokens = (isDeepSeekReasoning && thinkingMode === "disabled") ? 1024 : 8192;

  const result = {
    threadId: randomUUID(),
    memory: "",
    config: {
      workingDir: process.cwd(),
      date: new Date().toISOString().slice(0, 10),
      environment: process.platform,
      structure: [],
      isGitRepo: false,
      currentBranch: "",
      mainBranch: "",
      gitStatus: "",
      recentCommits: [],
    },
    params: {
      model,
      messages: converted.messages,
      system,
      max_tokens: maxTokens,
      ...(body.stream !== false && { stream: true }),
    },
  };

  // Only include tools if present
  const tools = convertTools(body.tools);
  if (tools.length > 0) {
    result.params.tools = tools;
  }

  // Thinking mode control for DeepSeek models
  if (isDeepSeekReasoning && thinkingMode !== "auto") {
    result.params.thinking = { type: thinkingMode === "enabled" ? "enabled" : "disabled" };
    if (thinkingMode === "enabled") {
      result.params.reasoning_effort = "max";
    }
  }

  return result;
}

// === Unified build function ===

export function buildCommandCodeBody(model, body, agentMode = false, thinkingMode = "auto") {
  if (agentMode) {
    return buildAgentBody(model, body, thinkingMode);
  }
  return buildStandardBody(model, body, thinkingMode);
}

// === Build headers ===

export function buildCommandCodeHeaders(apiKey, agentMode = false) {
  const headers = {
    "Content-Type": "application/json",
    Authorization: `Bearer ${apiKey}`,
    "x-command-code-version": COMMAND_CODE_VERSION,
    "x-cli-environment": "cli",
  };
  // In standard mode, add session id like 9Router does
  if (!agentMode) {
    headers["x-session-id"] = randomUUID();
  }
  return headers;
}

// === Parse stream events ===

function parseStreamLine(line) {
  let trimmed = line.trim();
  if (!trimmed || trimmed.startsWith(":") || trimmed.startsWith("event:")) return undefined;
  if (trimmed.startsWith("data:")) trimmed = trimmed.slice(5).trim();
  if (!trimmed || trimmed === "[DONE]") return undefined;

  try {
    return JSON.parse(trimmed);
  } catch {
    return undefined;
  }
}

function mapFinishReason(reason) {
  if (reason === "tool-calls" || reason === "tool_calls" || reason === "toolUse")
    return "tool_calls";
  if (reason === "length" || reason === "max_tokens" || reason === "max-tokens" || reason === "max_output_tokens")
    return "length";
  return "stop";
}

function chatCompletionChunk(id, model, delta, finishReason = null) {
  return {
    id,
    object: "chat.completion.chunk",
    created: Math.floor(Date.now() / 1000),
    model,
    choices: [{ index: 0, delta, finish_reason: finishReason }],
  };
}

// === Streaming response transformer ===

export function createStreamResponse(upstream, model) {
  const id = `chatcmpl-${randomUUID()}`;
  const reader = upstream.body.getReader();
  const decoder = new TextDecoder();
  const encoder = new TextEncoder();
  let buffer = "";
  let sentRole = false;
  let closed = false;
  let toolCallIndex = 0;

  const stream = new ReadableStream({
    start(controller) {
      const emitEvent = (event) => {
        if (!event || typeof event !== "object" || closed) return;

        if (!sentRole) {
          sentRole = true;
          controller.enqueue(encoder.encode(`data: ${JSON.stringify(chatCompletionChunk(id, model, { role: "assistant" }))}\n\n`));
        }

        switch (event.type) {
          case "text-delta": {
            const text = event.text || "";
            if (text) {
              controller.enqueue(encoder.encode(`data: ${JSON.stringify(chatCompletionChunk(id, model, { content: text }))}\n\n`));
            }
            break;
          }
          case "reasoning-delta": {
            const text = event.text || "";
            if (text) {
              controller.enqueue(encoder.encode(`data: ${JSON.stringify(chatCompletionChunk(id, model, { reasoning_content: text }))}\n\n`));
            }
            break;
          }
          case "tool-call": {
            const args = recordOrEmpty(event.input ?? event.args ?? event.arguments);
            const toolCall = {
              id: event.toolCallId || event.id || randomUUID(),
              type: "function",
              function: {
                name: event.toolName || event.name || "",
                arguments: JSON.stringify(args),
              },
            };
            controller.enqueue(encoder.encode(`data: ${JSON.stringify(chatCompletionChunk(id, model, { tool_calls: [{ index: toolCallIndex++, ...toolCall }] }))}\n\n`));
            break;
          }
          case "finish": {
            const finishReason = mapFinishReason(event.finishReason);
            controller.enqueue(encoder.encode(`data: ${JSON.stringify(chatCompletionChunk(id, model, {}, finishReason))}\n\n`));
            controller.enqueue(encoder.encode("data: [DONE]\n\n"));
            closed = true;
            controller.close();
            reader.cancel().catch(() => {});
            break;
          }
          case "error": {
            const error = typeof event.error === "object" ? event.error : {};
            const msg = error.message || event.error || "Command Code stream error";
            controller.enqueue(encoder.encode(`data: ${JSON.stringify({ error: { message: msg } })}\n\n`));
            controller.enqueue(encoder.encode("data: [DONE]\n\n"));
            closed = true;
            controller.close();
            reader.cancel().catch(() => {});
            break;
          }
        }
      };

      const pump = async () => {
        try {
          while (!closed) {
            const { done, value } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split("\n");
            buffer = lines.pop() || "";
            for (const line of lines) emitEvent(parseStreamLine(line));
          }
          if (buffer.trim()) emitEvent(parseStreamLine(buffer));
          if (!closed) {
            if (!sentRole) {
              controller.enqueue(encoder.encode(`data: ${JSON.stringify(chatCompletionChunk(id, model, { role: "assistant" }))}\n\n`));
            }
            controller.enqueue(encoder.encode(`data: ${JSON.stringify(chatCompletionChunk(id, model, {}, "stop"))}\n\n`));
            controller.enqueue(encoder.encode("data: [DONE]\n\n"));
            controller.close();
          }
        } catch (error) {
          if (!closed) controller.error(error);
        }
      };

      pump();
    },
    cancel() {
      closed = true;
      return reader?.cancel();
    },
  });

  return new Response(stream, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream; charset=utf-8",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    },
  });
}

// === Non-streaming JSON response ===

export async function createJsonResponse(upstream, model) {
  const reader = upstream.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  const state = {
    content: "",
    reasoning: "",
    toolCalls: [],
    finishReason: "stop",
    usage: null,
  };

  try {
    let finished = false;
    while (!finished) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";
      for (const line of lines) {
        const event = parseStreamLine(line);
        if (!event || typeof event !== "object") continue;

        switch (event.type) {
          case "text-delta":
            state.content += event.text || "";
            break;
          case "reasoning-delta":
            state.reasoning += event.text || "";
            break;
          case "tool-call": {
            const args = recordOrEmpty(event.input ?? event.args ?? event.arguments);
            state.toolCalls.push({
              id: event.toolCallId || event.id || randomUUID(),
              type: "function",
              function: {
                name: event.toolName || event.name || "",
                arguments: JSON.stringify(args),
              },
            });
            break;
          }
          case "finish":
            state.finishReason = mapFinishReason(event.finishReason);
            state.usage = event.totalUsage || null;
            finished = true;
            break;
          case "error": {
            const error = typeof event.error === "object" ? event.error : {};
            throw new Error(error.message || event.error || "Command Code stream error");
          }
        }
      }
    }
    if (buffer.trim()) {
      const event = parseStreamLine(buffer);
      if (event && typeof event === "object") {
        if (event.type === "text-delta") state.content += event.text || "";
        if (event.type === "finish") {
          state.finishReason = mapFinishReason(event.finishReason);
          state.usage = event.totalUsage || null;
        }
      }
    }
  } finally {
    reader.releaseLock();
  }

  // Build OpenAI-compatible response
  let cleanContent = state.content;

  // Strip CommandCode-specific tags
  cleanContent = cleanContent.replace(/<commentary>[\s\S]*?<\/commentary>/g, "").trim();
  cleanContent = cleanContent.replace(/<tool_call[^>]*>[\s\S]*?<\/tool_call>/g, "").trim();
  cleanContent = cleanContent.replace(/<\/?tool_calls>/g, "").trim();

  // Detect spam patterns
  const isSpam = /^(<\/?tool_calls>\s*){5,}$/s.test(cleanContent) ||
                 cleanContent.split("<tool_calls>").length > 10;
  if (isSpam) cleanContent = "";

  const message = { role: "assistant", content: cleanContent };
  if (state.reasoning) message.reasoning_content = state.reasoning;
  if (state.toolCalls.length > 0) message.tool_calls = state.toolCalls;

  const payload = {
    id: `chatcmpl-${randomUUID()}`,
    object: "chat.completion",
    created: Math.floor(Date.now() / 1000),
    model,
    choices: [{ index: 0, message, finish_reason: state.finishReason }],
  };

  if (state.usage) {
    const prompt = state.usage.inputTokens || 0;
    const completion = state.usage.outputTokens || 0;
    payload.usage = {
      prompt_tokens: prompt,
      completion_tokens: completion,
      total_tokens: prompt + completion,
    };
  }

  return payload;
}
