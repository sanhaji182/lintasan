import { validateDashboardSession } from "@/lib/auth";
import { getCostSummary, getCustomPricing, setCustomPricing } from "@/lib/cost-tracker";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { searchParams } = new URL(request.url);
  const days = parseInt(searchParams.get("days") || "7");
  const summary = getCostSummary(days);
  const customPricing = getCustomPricing();
  return Response.json({ data: { ...summary, customPricing } });
}

export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { pricing } = await request.json();
  if (pricing) setCustomPricing(pricing);
  return Response.json({ success: true });
}
