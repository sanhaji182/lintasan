import { NextResponse } from "next/server";
import { validateDashboardSession } from "@/lib/auth";
import {
  OAUTH_PROVIDERS,
  requestDeviceCode,
  pollForToken,
  isOAuthConnected,
  disconnectOAuth,
  listOAuthConnections,
} from "@/lib/oauth/github";

// GET /api/oauth — list providers and status
// GET /api/oauth?action=device-code&provider=github — start device code flow
export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const action = searchParams.get("action");
  const provider = searchParams.get("provider");

  // List all OAuth providers with connection status
  if (!action) {
    return NextResponse.json({ data: listOAuthConnections() });
  }

  // Request device code
  if (action === "device-code") {
    if (!provider) {
      return NextResponse.json({ error: "provider required" }, { status: 400 });
    }
    try {
      const result = await requestDeviceCode(provider);
      return NextResponse.json({ success: true, data: result });
    } catch (err) {
      return NextResponse.json({ error: err.message }, { status: 500 });
    }
  }

  return NextResponse.json({ error: "Unknown action" }, { status: 400 });
}

// POST /api/oauth — poll for token or disconnect
export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const { action, provider, deviceCode } = body;

  // Poll for token
  if (action === "poll") {
    if (!provider || !deviceCode) {
      return NextResponse.json({ error: "provider and deviceCode required" }, { status: 400 });
    }
    try {
      const result = await pollForToken(provider, deviceCode);
      return NextResponse.json(result);
    } catch (err) {
      return NextResponse.json({ error: err.message }, { status: 500 });
    }
  }

  // Disconnect
  if (action === "disconnect") {
    if (!provider) {
      return NextResponse.json({ error: "provider required" }, { status: 400 });
    }
    disconnectOAuth(provider);
    return NextResponse.json({ success: true });
  }

  return NextResponse.json({ error: "Unknown action" }, { status: 400 });
}
