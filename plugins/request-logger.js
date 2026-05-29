/**
 * Request Logger Plugin
 * Logs all requests to console with timing information.
 */

const requestLogger = {
  name: "request-logger",
  version: "1.0.0",
  enabled: true,
  priority: 10,

  hooks: {
    beforeRequest(ctx) {
      ctx.metadata = ctx.metadata || {};
      ctx.metadata._loggerStart = performance.now();
      console.log(
        `[request-logger] → ${ctx.model} | stream=${ctx.stream} | messages=${ctx.messages?.length || 0} | ${new Date().toISOString()}`
      );
      return null;
    },

    afterRequest(ctx, response) {
      const start = ctx.metadata?._loggerStart;
      const duration = start ? (performance.now() - start).toFixed(2) : "?";
      console.log(
        `[request-logger] ← ${ctx.model} | ${duration}ms | status=${response?.status || "ok"} | ${new Date().toISOString()}`
      );
      return response;
    },

    onError(ctx, error) {
      const start = ctx.metadata?._loggerStart;
      const duration = start ? (performance.now() - start).toFixed(2) : "?";
      console.error(
        `[request-logger] ✗ ${ctx.model} | ${duration}ms | error=${error.message} | ${new Date().toISOString()}`
      );
      return null;
    },

    onStream(ctx, chunk) {
      return chunk;
    },
  },
};

export default requestLogger;
