import { PROVIDER_PRESETS } from "@/lib/provider-presets";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// GET /api/providers/presets/config?id=openai — get full config for a preset
export async function GET(request) {
  const { searchParams } = new URL(request.url);
  const id = searchParams.get("id");

  if (!id) {
    return Response.json({ error: { message: "id parameter required" } }, { status: 400 });
  }

  const preset = PROVIDER_PRESETS.find(p => p.id === id);
  if (!preset) {
    return Response.json({ error: { message: "Preset not found: " + id } }, { status: 404 });
  }

  return Response.json({
    data: {
      id: preset.id,
      name: preset.name,
      baseUrl: preset.baseUrl,
      format: preset.format,
      chatPath: preset.chatPath,
      modelsPath: preset.modelsPath,
      authHeader: preset.authHeader || "Authorization",
      authPrefix: preset.authPrefix || "Bearer ",
      extraHeaders: preset.extraHeaders || "{}",
      noAuth: preset.noAuth || false,
      knownModels: preset.knownModels || null,
      description: preset.description,
    },
  });
}
