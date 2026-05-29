// Users API: GET (list) and POST (create)
import { validateDashboardSessionMultiUser, canManageUsers, listUsers, createUser } from "@/lib/users.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request) {
  const session = validateDashboardSessionMultiUser(request);
  if (!session) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  // Only admins can list users
  if (!canManageUsers(session)) {
    return Response.json({ error: "Forbidden: admin access required" }, { status: 403 });
  }

  try {
    const users = listUsers();
    return Response.json({ data: users });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}

export async function POST(request) {
  const session = validateDashboardSessionMultiUser(request);
  if (!session) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  // Only admins can create users
  if (!canManageUsers(session)) {
    return Response.json({ error: "Forbidden: admin access required" }, { status: 403 });
  }

  try {
    const body = await request.json();
    const { username, password, role, email } = body;

    if (!username || !password) {
      return Response.json({ error: { message: "username and password are required" } }, { status: 400 });
    }

    const user = createUser({ username, password, role, email });
    return Response.json({ data: user }, { status: 201 });
  } catch (error) {
    const status = error.message.includes("already exists") ? 409 : 400;
    return Response.json({ error: { message: error.message } }, { status });
  }
}
