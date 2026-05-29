import { validateDashboardSession } from "@/lib/auth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  const valid = validateDashboardSession(request);
  return Response.json({ authenticated: valid });
}
