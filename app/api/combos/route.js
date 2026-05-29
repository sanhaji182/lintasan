import { validateDashboardSession } from "@/lib/auth";
import { listCombos, getCombo, saveCombo, deleteCombo } from "@/lib/combo";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// GET /api/combos — list all combos
export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  return Response.json({ data: listCombos() });
}

// POST /api/combos — create/update a combo
export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  try {
    const body = await request.json();
    const { name, models, stickyLimit, description } = body;

    if (!name || !models || !Array.isArray(models) || models.length === 0) {
      return Response.json({ error: { message: "name and models[] are required" } }, { status: 400 });
    }

    // Validate models format: [{ model: "deepseek/deepseek-v4-pro", label: "DeepSeek V4" }, ...]
    const validModels = models.map(m => ({
      model: m.model || m,
      label: m.label || m.model || m,
    }));

    const combo = saveCombo({ name, models: validModels, stickyLimit: stickyLimit || 3, description: description || "" });
    return Response.json({ data: combo }, { status: 201 });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}

// DELETE /api/combos?name=xxx — delete a combo
export async function DELETE(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { searchParams } = new URL(request.url);
  const name = searchParams.get("name");
  if (!name) return Response.json({ error: { message: "name is required" } }, { status: 400 });
  deleteCombo(name);
  return Response.json({ success: true });
}
