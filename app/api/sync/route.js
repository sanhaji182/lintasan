import { NextResponse } from "next/server";
import { validateDashboardSession } from "@/lib/auth";
import { exportConfig, importConfig, generateShareCode, decodeShareCode } from "@/lib/cloud-sync";

// GET /api/sync — export config
export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const includeKeys = searchParams.get("includeKeys") === "true";
  const format = searchParams.get("format") || "json"; // json or share-code

  const config = exportConfig({ includeKeys });

  if (format === "share-code") {
    const code = generateShareCode(config);
    return NextResponse.json({ data: { code, length: code.length } });
  }

  return NextResponse.json({ data: config });
}

// POST /api/sync — import config
export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const { config, shareCode, overwrite = false, mergeConnections = true } = body;

  let configData = config;

  // Decode share code if provided
  if (shareCode && !configData) {
    try {
      configData = decodeShareCode(shareCode);
    } catch (err) {
      return NextResponse.json({ error: err.message }, { status: 400 });
    }
  }

  if (!configData) {
    return NextResponse.json({ error: "config or shareCode required" }, { status: 400 });
  }

  // Validate config structure
  if (!configData.version || !configData.exported_at) {
    return NextResponse.json({ error: "Invalid config format" }, { status: 400 });
  }

  const results = importConfig(configData, { overwrite, mergeConnections });

  return NextResponse.json({
    success: true,
    data: results,
    message: `Imported: ${results.settings} settings, ${results.connections} connections, ${results.combos} combos`,
  });
}
