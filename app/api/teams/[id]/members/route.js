// Team members API: GET (list), POST (add), DELETE (remove)
import { validateDashboardSessionMultiUser, canManageUsers } from "@/lib/users.js";
import { addMember, removeMember, listMembers, isTeamMember, isTeamOwner, getTeam } from "@/lib/teams.js";

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
    return Response.json({ data: members });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}

export async function POST(request, { params }) {
  const session = validateDashboardSessionMultiUser(request);
  if (!session) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;

  try {
    // Only owner or admin can add members
    const isAdmin = canManageUsers(session);
    if (!isAdmin && !isTeamOwner(id, session.userId)) {
      return Response.json({ error: "Forbidden: owner or admin access required" }, { status: 403 });
    }

    const body = await request.json();
    const { userId, role } = body;

    if (!userId) {
      return Response.json({ error: { message: "userId is required" } }, { status: 400 });
    }

    const member = addMember(id, userId, role || "member");
    return Response.json({ data: member }, { status: 201 });
  } catch (error) {
    const status = error.message.includes("not found") ? 404
      : error.message.includes("already a member") ? 409
      : 400;
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
    // Only owner or admin can remove members
    const isAdmin = canManageUsers(session);
    if (!isAdmin && !isTeamOwner(id, session.userId)) {
      return Response.json({ error: "Forbidden: owner or admin access required" }, { status: 403 });
    }

    const url = new URL(request.url);
    const userId = url.searchParams.get("userId");

    if (!userId) {
      return Response.json({ error: { message: "userId query parameter is required" } }, { status: 400 });
    }

    const result = removeMember(id, userId);
    return Response.json({ data: result });
  } catch (error) {
    const status = error.message.includes("not a member") ? 404
      : error.message.includes("last owner") ? 400
      : 400;
    return Response.json({ error: { message: error.message } }, { status });
  }
}
