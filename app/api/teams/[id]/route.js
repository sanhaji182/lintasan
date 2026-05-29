// Team detail API: GET, PUT, DELETE
import { validateDashboardSessionMultiUser, canManageUsers } from "@/lib/users.js";
import { getTeam, updateTeam, deleteTeam, listMembers, isTeamMember, isTeamOwner, getTeamUsage } from "@/lib/teams.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request, { params }) {
  const session = validateDashboardSessionMultiUser(request);
  if (!session) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;

  try {
    const team = getTeam(id);
    if (!team) {
      return Response.json({ error: { message: "Team not found" } }, { status: 404 });
    }

    // Check access: admin or team member
    const isAdmin = canManageUsers(session);
    if (!isAdmin && !isTeamMember(id, session.userId)) {
      return Response.json({ error: "Forbidden" }, { status: 403 });
    }

    const members = listMembers(id);
    const usage = getTeamUsage(id, "24h");

    return Response.json({ data: { ...team, members, usage } });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}

export async function PUT(request, { params }) {
  const session = validateDashboardSessionMultiUser(request);
  if (!session) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;

  try {
    // Only owner or admin can update
    const isAdmin = canManageUsers(session);
    if (!isAdmin && !isTeamOwner(id, session.userId)) {
      return Response.json({ error: "Forbidden: owner or admin access required" }, { status: 403 });
    }

    const body = await request.json();
    const { name, description, is_active } = body;

    const updates = {};
    if (name !== undefined) updates.name = name;
    if (description !== undefined) updates.description = description;
    if (is_active !== undefined) updates.is_active = is_active;

    const team = updateTeam(id, updates);
    return Response.json({ data: team });
  } catch (error) {
    const status = error.message.includes("not found") ? 404 : 400;
    return Response.json({ error: { message: error.message } }, { status });
  }
}

export async function DELETE(request, { params }) {
  const session = validateDashboardSessionMultiUser(request);
  if (!session) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;

  try {
    // Only owner or admin can delete
    const isAdmin = canManageUsers(session);
    if (!isAdmin && !isTeamOwner(id, session.userId)) {
      return Response.json({ error: "Forbidden: owner or admin access required" }, { status: 403 });
    }

    const result = deleteTeam(id);
    return Response.json({ data: result });
  } catch (error) {
    const status = error.message.includes("not found") ? 404 : 400;
    return Response.json({ error: { message: error.message } }, { status });
  }
}
