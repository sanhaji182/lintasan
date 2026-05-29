import { PROVIDER_PRESETS, PRESET_CATEGORIES } from "@/lib/provider-presets";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// GET /api/providers/presets — list all available provider presets
export async function GET() {
  return Response.json({
    data: PROVIDER_PRESETS.map(p => ({
      id: p.id,
      name: p.name,
      category: p.category,
      description: p.description,
      website: p.website || null,
      icon: p.icon,
      noAuth: p.noAuth || false,
    })),
    categories: PRESET_CATEGORIES,
  });
}
