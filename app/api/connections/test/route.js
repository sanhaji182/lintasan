import { validateDashboardSession } from "@/lib/auth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Test a connection before saving — tries to reach the provider
export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  try {
    const body = await request.json();
    const { baseUrl, apiKey, modelsPath, chatPath, authHeader, authPrefix, extraHeaders, format } = body;

    if (!baseUrl || !apiKey) {
      return Response.json({ error: { message: "baseUrl and apiKey are required" } }, { status: 400 });
    }

    const cleanBase = baseUrl.replace(/\/+$/, "");
    const headers = {
      "Content-Type": "application/json",
      ...(extraHeaders ? JSON.parse(extraHeaders) : {}),
    };

    if (authHeader && apiKey) {
      headers[authHeader || "Authorization"] = (authPrefix || "Bearer ") + apiKey;
    }

    // Strategy 1: Try /models endpoint if available
    if (modelsPath) {
      try {
        const modelsUrl = cleanBase + modelsPath;
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), 10000);

        const res = await fetch(modelsUrl, {
          method: "GET",
          headers,
          signal: controller.signal,
        });
        clearTimeout(timeout);

        if (res.ok) {
          const data = await res.json();
          const modelCount = data?.data?.length || (Array.isArray(data) ? data.length : 0);
          return Response.json({
            success: true,
            message: `Connected! Found ${modelCount} models.`,
            models: modelCount,
            latency: null,
          });
        }

        // If models endpoint returns 401/403, key is wrong
        if (res.status === 401 || res.status === 403) {
          return Response.json({
            success: false,
            message: `Authentication failed (${res.status}). Check your API key.`,
          }, { status: 200 });
        }
      } catch (e) {
        if (e.name === "AbortError") {
          return Response.json({
            success: false,
            message: "Connection timeout (10s). Check the base URL.",
          }, { status: 200 });
        }
        // Fall through to strategy 2
      }
    }

    // Strategy 2: Try a minimal chat completion request
    try {
      const chatUrl = cleanBase + (chatPath || "/v1/chat/completions");
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), 15000);

      // CommandCode format needs special body
      let testBody;
      if (format === "commandcode") {
        const { randomUUID } = await import("crypto");
        testBody = {
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
            model: "deepseek/deepseek-v4-pro",
            messages: [{ role: "user", content: "hi" }],
            system: "",
            max_tokens: 1,
            stream: true,
          },
        };
        // CommandCode needs x-command-code-version header
        if (!headers["x-command-code-version"]) {
          headers["x-command-code-version"] = "0.26.25";
        }
        if (!headers["x-cli-environment"]) {
          headers["x-cli-environment"] = "cli";
        }
      } else {
        testBody = {
          model: "test",
          messages: [{ role: "user", content: "hi" }],
          max_tokens: 1,
        };
      }

      const res = await fetch(chatUrl, {
        method: "POST",
        headers,
        body: JSON.stringify(testBody),
        signal: controller.signal,
      });
      clearTimeout(timeout);

      // Any response (even 404 model not found) means the server is reachable and auth works
      if (res.status === 200 || res.status === 400 || res.status === 404) {
        return Response.json({
          success: true,
          message: "Connected! Server is reachable and API key is valid.",
          models: 0,
        });
      }

      if (res.status === 401 || res.status === 403) {
        return Response.json({
          success: false,
          message: `Authentication failed (${res.status}). Check your API key.`,
        }, { status: 200 });
      }

      if (res.status === 429) {
        // Rate limited but connection works
        return Response.json({
          success: true,
          message: "Connected! (Rate limited but reachable)",
          models: 0,
        });
      }

      return Response.json({
        success: false,
        message: `Server returned ${res.status}. Check base URL and API key.`,
      }, { status: 200 });

    } catch (e) {
      if (e.name === "AbortError") {
        return Response.json({
          success: false,
          message: "Connection timeout (15s). Check the base URL.",
        }, { status: 200 });
      }

      return Response.json({
        success: false,
        message: `Connection failed: ${e.message}`,
      }, { status: 200 });
    }

  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}
