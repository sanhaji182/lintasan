import https from "https";
import { getSetting, setSetting } from "../db/index.js";

// Known OAuth providers with device-code flow
export const OAUTH_PROVIDERS = {
  github: {
    id: "github",
    name: "GitHub Copilot",
    clientId: "Iv1.b507a08c87ecfe98",
    scope: "read:user",
    deviceCodeUrl: "https://github.com/login/device/code",
    tokenUrl: "https://github.com/login/oauth/access_token",
    copilotTokenUrl: "https://api.github.com/copilot_internal/v2/token",
    apiBase: "https://api.individual.githubcopilot.com",
    chatPath: "/chat/completions",
  },
};

// Request device code
export async function requestDeviceCode(providerId) {
  const provider = OAUTH_PROVIDERS[providerId];
  if (!provider) throw new Error(`Unknown OAuth provider: ${providerId}`);

  const body = `client_id=${provider.clientId}&scope=${provider.scope}`;

  const data = await httpPost(provider.deviceCodeUrl, body, {
    "Content-Type": "application/x-www-form-urlencoded",
    Accept: "application/json",
  });

  return {
    device_code: data.device_code,
    user_code: data.user_code,
    verification_uri: data.verification_uri,
    verification_uri_complete: data.verification_uri_complete || `${data.verification_uri}?user_code=${data.user_code}`,
    expires_in: data.expires_in,
    interval: data.interval || 5,
  };
}

// Poll for access token
export async function pollForToken(providerId, deviceCode) {
  const provider = OAUTH_PROVIDERS[providerId];
  if (!provider) throw new Error(`Unknown OAuth provider: ${providerId}`);

  const body = `client_id=${provider.clientId}&device_code=${deviceCode}&grant_type=urn:ietf:params:oauth:grant-type:device_code`;

  const data = await httpPost(provider.tokenUrl, body, {
    "Content-Type": "application/x-www-form-urlencoded",
    Accept: "application/json",
  });

  if (data.error === "authorization_pending") {
    return { pending: true };
  }
  if (data.error === "slow_down") {
    return { pending: true, slowDown: true };
  }
  if (data.error) {
    return { error: data.error_description || data.error };
  }

  // Success — save the OAuth token
  const tokenKey = `oauth_${providerId}_token`;
  setSetting(tokenKey, data.access_token);

  return { success: true, access_token: data.access_token, token_type: data.token_type };
}

// Get Copilot session token (short-lived, ~30min)
export async function getCopilotSessionToken() {
  const oauthToken = getSetting("oauth_github_token", "");
  if (!oauthToken) throw new Error("No GitHub OAuth token. Run device-code flow first.");

  const provider = OAUTH_PROVIDERS.github;
  const data = await httpGet(provider.copilotTokenUrl, {
    Authorization: `token ${oauthToken}`,
    Accept: "application/json",
    "User-Agent": "Lintasan/1.0",
  });

  if (data.token) {
    // Cache the session token with expiry
    setSetting("oauth_github_session_token", data.token);
    setSetting("oauth_github_session_expires", data.expires_at || String(Date.now() + 30 * 60 * 1000));
    return data.token;
  }

  throw new Error(data.message || "Failed to get Copilot session token");
}

// Get valid session token (refresh if expired)
export async function getValidCopilotToken() {
  const token = getSetting("oauth_github_session_token", "");
  const expires = getSetting("oauth_github_session_expires", "0");

  // Check if expired (with 60s buffer)
  if (token && Date.now() < (parseInt(expires) || new Date(expires).getTime()) - 60000) {
    return token;
  }

  // Refresh
  return getCopilotSessionToken();
}

// Check if provider is connected
export function isOAuthConnected(providerId) {
  const token = getSetting(`oauth_${providerId}_token`, "");
  return !!token;
}

// Disconnect provider
export function disconnectOAuth(providerId) {
  setSetting(`oauth_${providerId}_token`, "");
  setSetting(`oauth_${providerId}_session_token`, "");
  setSetting(`oauth_${providerId}_session_expires`, "0");
}

// List connected OAuth providers
export function listOAuthConnections() {
  return Object.values(OAUTH_PROVIDERS).map((p) => ({
    ...p,
    connected: isOAuthConnected(p.id),
  }));
}

// --- HTTP helpers ---

function httpPost(url, body, headers) {
  return new Promise((resolve, reject) => {
    const parsed = new URL(url);
    const req = https.request(
      {
        hostname: parsed.hostname,
        path: parsed.pathname + parsed.search,
        method: "POST",
        headers: {
          ...headers,
          "Content-Length": Buffer.byteLength(body),
          "User-Agent": "Lintasan/1.0",
        },
      },
      (res) => {
        let data = "";
        res.on("data", (c) => (data += c));
        res.on("end", () => {
          try {
            resolve(JSON.parse(data));
          } catch {
            reject(new Error(`Invalid JSON: ${data.substring(0, 200)}`));
          }
        });
      }
    );
    req.on("error", reject);
    req.write(body);
    req.end();
  });
}

function httpGet(url, headers) {
  return new Promise((resolve, reject) => {
    const parsed = new URL(url);
    const req = https.request(
      {
        hostname: parsed.hostname,
        path: parsed.pathname + parsed.search,
        method: "GET",
        headers: {
          ...headers,
          "User-Agent": "Lintasan/1.0",
        },
      },
      (res) => {
        let data = "";
        res.on("data", (c) => (data += c));
        res.on("end", () => {
          try {
            resolve(JSON.parse(data));
          } catch {
            reject(new Error(`Invalid JSON: ${data.substring(0, 200)}`));
          }
        });
      }
    );
    req.on("error", reject);
    req.end();
  });
}
