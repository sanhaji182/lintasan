import https from "https";
import { getValidCopilotToken } from "../oauth/github.js";

// Execute request via GitHub Copilot OAuth token
export async function executeCopilotRequest(body, stream = true) {
  const token = await getValidCopilotToken();

  const payload = JSON.stringify({
    model: body.model || "gpt-4o",
    messages: body.messages,
    stream: stream,
    max_tokens: body.max_tokens,
    temperature: body.temperature,
  });

  return new Promise((resolve, reject) => {
    const req = https.request(
      {
        hostname: "api.individual.githubcopilot.com",
        path: "/chat/completions",
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Content-Length": Buffer.byteLength(payload),
          Authorization: `Bearer ${token}`,
          "User-Agent": "Lintasan/1.0",
          "Editor-Version": "vscode/1.96.0",
          "Copilot-Integration-Id": "vscode-chat",
          "Openai-Intent": "conversation-panel",
        },
      },
      (res) => {
        if (stream) {
          resolve({
            status: res.statusCode,
            headers: res.headers,
            stream: res,
          });
        } else {
          let data = "";
          res.on("data", (c) => (data += c));
          res.on("end", () => {
            try {
              resolve({ status: res.statusCode, data: JSON.parse(data) });
            } catch {
              resolve({ status: res.statusCode, data: { error: data } });
            }
          });
        }
      }
    );

    req.on("error", reject);
    req.write(payload);
    req.end();
  });
}
