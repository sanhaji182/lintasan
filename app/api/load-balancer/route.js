import { validateDashboardSession } from "@/lib/auth";
import { getAllStrategies, setLoadBalanceStrategy, getLatencyStats, STRATEGIES } from "@/lib/load-balancer";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  return Response.json({
    data: {
      strategies: getAllStrategies(),
      available: STRATEGIES,
      latency_stats: getLatencyStats(),
    },
  });
}

export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { provider, strategy } = await request.json();
  if (!provider || !strategy) return Response.json({ error: "provider and strategy required" }, { status: 400 });
  if (!STRATEGIES.includes(strategy)) return Response.json({ error: "Invalid strategy. Available: " + STRATEGIES.join(", ") }, { status: 400 });
  setLoadBalanceStrategy(provider, strategy);
  return Response.json({ success: true });
}
