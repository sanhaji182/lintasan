import { NextResponse } from "next/server";
import { validateDashboardSession } from "@/lib/auth";
import { getMarketplaceProviders, getMarketplaceCategories } from "@/lib/provider-marketplace";
import { createConnection, listConnections } from "@/lib/db";

// GET /api/marketplace — list providers
export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const category = searchParams.get("category") || "all";
  const search = searchParams.get("search") || "";
  const sort = searchParams.get("sort") || "popularity";

  const providers = getMarketplaceProviders({ category, search, sort });
  const categories = getMarketplaceCategories();

  // Mark installed providers
  const connections = listConnections();
  const installedUrls = new Set(connections.map((c) => c.base_url));

  const withStatus = providers.map((p) => ({
    ...p,
    installed: installedUrls.has(p.baseUrl),
  }));

  return NextResponse.json({ data: { providers: withStatus, categories } });
}

// POST /api/marketplace — install a provider
export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const { providerId, apiKey } = body;

  const providers = getMarketplaceProviders();
  const provider = providers.find((p) => p.id === providerId);

  if (!provider) {
    return NextResponse.json({ error: "Provider not found" }, { status: 404 });
  }

  if (provider.authRequired && !apiKey) {
    return NextResponse.json({ error: "API key required for this provider" }, { status: 400 });
  }

  // Check if already installed
  const connections = listConnections();
  const exists = connections.some((c) => c.base_url === provider.baseUrl);
  if (exists) {
    return NextResponse.json({ error: "Provider already installed" }, { status: 409 });
  }

  try {
    const id = createConnection({
      name: provider.name,
      baseUrl: provider.baseUrl,
      apiKey: apiKey || "",
      format: provider.format,
      chatPath: provider.chatPath,
      modelsPath: provider.modelsPath,
      authHeader: provider.authRequired ? "Authorization" : "",
      authPrefix: provider.authRequired ? "Bearer " : "",
      priority: 0,
    });

    return NextResponse.json({
      success: true,
      data: { id, name: provider.name },
      message: `${provider.name} installed. Sync models to discover available models.`,
    });
  } catch (err) {
    return NextResponse.json({ error: err.message }, { status: 500 });
  }
}
