// Stream Cache - replay cached JSON response as SSE stream
import { randomUUID } from "crypto";

function sseChunk(data) {
  return "data: " + JSON.stringify(data) + "\n\n";
}

export function replayAsStream(cachedResponse, model) {
  const encoder = new TextEncoder();
  const id = "chatcmpl-" + randomUUID();
  const content = cachedResponse.choices?.[0]?.message?.content || "";
  const reasoningContent = cachedResponse.choices?.[0]?.message?.reasoning_content || "";
  const toolCalls = cachedResponse.choices?.[0]?.message?.tool_calls || [];
  const finishReason = cachedResponse.choices?.[0]?.finish_reason || "stop";
  const created = Math.floor(Date.now() / 1000);

  const stream = new ReadableStream({
    start(controller) {
      // Role chunk
      controller.enqueue(encoder.encode(sseChunk({
        id, object: "chat.completion.chunk", created, model,
        choices: [{ index: 0, delta: { role: "assistant" }, finish_reason: null }],
      })));

      // Reasoning content (if any)
      if (reasoningContent) {
        const chunks = reasoningContent.match(/.{1,20}/g) || [];
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(sseChunk({
            id, object: "chat.completion.chunk", created, model,
            choices: [{ index: 0, delta: { reasoning_content: chunk }, finish_reason: null }],
          })));
        }
      }

      // Content chunks
      if (content) {
        const chunks = content.match(/.{1,20}/g) || [];
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(sseChunk({
            id, object: "chat.completion.chunk", created, model,
            choices: [{ index: 0, delta: { content: chunk }, finish_reason: null }],
          })));
        }
      }

      // Tool calls (if any)
      for (let i = 0; i < toolCalls.length; i++) {
        controller.enqueue(encoder.encode(sseChunk({
          id, object: "chat.completion.chunk", created, model,
          choices: [{ index: 0, delta: { tool_calls: [{ index: i, ...toolCalls[i] }] }, finish_reason: null }],
        })));
      }

      // Finish chunk
      controller.enqueue(encoder.encode(sseChunk({
        id, object: "chat.completion.chunk", created, model,
        choices: [{ index: 0, delta: {}, finish_reason: finishReason }],
      })));

      controller.enqueue(encoder.encode("data: [DONE]\n\n"));
      controller.close();
    },
  });

  return new Response(stream, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream; charset=utf-8",
      "Cache-Control": "no-cache",
      "Connection": "keep-alive",
      "X-Cache": "HIT",
    },
  });
}
