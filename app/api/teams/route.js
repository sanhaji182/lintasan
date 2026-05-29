// Teams API: GET (list) and POST (create)
import { validateDashboardSessionMultiUser, canManageUsers } from "@/lib/users.js";
import { createTeam, listTeams } from "@/lib/teams.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  const session = validateDashboardSessionMultiUser(request);
  if (!session) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  try {
    // Admins see all teams, others see only their teams
    const isAdmin = canManageUsers(session);
    const teams = isAdmin ? listTeams() : listTeams(session.userId);
    return Response.json({ data: teams });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}

export async function POST(request) {
  const session = validateDashboardSessionMultiUser(request);
  if (!session) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  try {
    const body = await request.json();
    const { name, description } = body;

    if (!name) {
      return Response.json({ error: { message: "Team name is required" } }, { status: 400 });
    }

    const team = createTeam({ name, description, ownerId: session.userId });
    return Response.json({ data: team }, { status: 201 });
  } catch (error) {
    const status = error.message.includes("already exists") ? 409 : 400;
    return Response.json({ error: { message: error.message } }, { status });
  }
}
