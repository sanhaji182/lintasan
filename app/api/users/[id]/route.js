// Users API: PUT (update) and DELETE by ID
import { validateDashboardSessionMultiUser, canManageUsers, updateUser, deleteUser, changePassword, getUser } from "@/lib/users.js";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request, { params }) {
  const session = validateDashboardSessionMultiUser(request);
  if (!session) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (!canManageUsers(session)) {
    return Response.json({ error: "Forbidden: admin access required" }, { status: 403 });
  }

  try {
    const { id } = await params;
    const user = getUser(id);
    if (!user) {
      return Response.json({ error: { message: "User not found" } }, { status: 404 });
    }
    return Response.json({ data: user });
  } catch (error) {
    return Response.json({ error: { message: error.message } }, { status: 500 });
  }
}

export async function PUT(request, { params }) {
  const session = validateDashboardSessionMultiUser(request);
  if (!session) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (!canManageUsers(session)) {
    return Response.json({ error: "Forbidden: admin access required" }, { status: 403 });
  }

  try {
    const { id } = await params;
    const body = await request.json();
    const { role, email, is_active, password } = body;

    // Handle password change separately
    if (password) {
      changePassword(id, password);
    }

    // Update other fields
    const updates = {};
    if (role !== undefined) updates.role = role;
    if (email !== undefined) updates.email = email;
    if (is_active !== undefined) updates.is_active = is_active ? 1 : 0;

    let user;
    if (Object.keys(updates).length > 0) {
      user = updateUser(id, updates);
    } else {
      user = getUser(id);
    }

    if (!user) {
      return Response.json({ error: { message: "User not found" } }, { status: 404 });
    }

    return Response.json({ data: user });
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

  if (!canManageUsers(session)) {
    return Response.json({ error: "Forbidden: admin access required" }, { status: 403 });
  }

  try {
    const { id } = await params;
    const result = deleteUser(id);
    return Response.json({ data: result });
  } catch (error) {
    const status = error.message.includes("not found") ? 404 : 400;
    return Response.json({ error: { message: error.message } }, { status });
  }
}
