import { getRealtimeStats, getTimeSeries, getTopModels, getTopProviders } from "@/lib/streaming-analytics.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET() {
  const encoder = new TextEncoder();

  const stream = new ReadableStream({
    start(controller) {
      const send = (event, data) => {
        controller.enqueue(encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`));
      };

      // Send initial stats immediately
      try {
        send("stats", {
          stats: getRealtimeStats(),
          timeSeries: getTimeSeries("1h"),
          topModels: getTopModels(10),
          topProviders: getTopProviders(10),
          generatedAt: new Date().toISOString(),
        });
      } catch (e) {
        send("error", { message: e.message });
      }

      // Push updates every 5 seconds
      const interval = setInterval(() => {
        try {
          send("stats", {
            stats: getRealtimeStats(),
            timeSeries: getTimeSeries("1h"),
            topModels: getTopModels(10),
            topProviders: getTopProviders(10),
            generatedAt: new Date().toISOString(),
          });
        } catch (e) {
          send("error", { message: e.message });
        }
      }, 5000);

      // Send heartbeat every 30s to keep connection alive
      const heartbeat = setInterval(() => {
        try {
          controller.enqueue(encoder.encode(": heartbeat\n\n"));
        } catch {
          // Connection closed
        }
      }, 30000);

      // Cleanup on cancel
      const cleanup = () => {
        clearInterval(interval);
        clearInterval(heartbeat);
      };

      // Store cleanup ref for cancel signal
      controller._cleanup = cleanup;
    },
    cancel(controller) {
      if (controller._cleanup) {
        controller._cleanup();
      }
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache, no-transform",
      "Connection": "keep-alive",
      "X-Accel-Buffering": "no",
    },
  });
}
