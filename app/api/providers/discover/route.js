import { NextResponse } from "next/server";
import { validateDashboardSession } from "@/lib/auth";
import { scanFreeProviders, autoAddFreeProviders } from "@/lib/free-providers";

// GET /api/providers/discover — scan for free/local providers
export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const results = await scanFreeProviders();
  return NextResponse.json({ data: results });
}

// POST /api/providers/discover — auto-add discovered providers
export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { discovered, added } = await autoAddFreeProviders();
  return NextResponse.json({
    data: { discovered, added },
    message: added.length > 0
      ? `Added ${added.length} provider(s): ${added.map((a) => a.name).join(", ")}`
      : "No new providers found to add",
  });
}
