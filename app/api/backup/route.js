import { validateDashboardSession } from "@/lib/auth";
import {
  createBackup,
  listBackups,
  restoreBackup,
  cleanOldBackups,
} from "@/lib/export-backup";
import fs from "fs";
import path from "path";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const BACKUP_DIR = path.join(process.cwd(), "data", "backups");

export async function GET(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  try {
    const backups = listBackups();
    return Response.json({ data: backups });
  } catch (err) {
    return Response.json({ error: err.message }, { status: 500 });
  }
}

export async function POST(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const action = searchParams.get("action");

  try {
    if (action === "restore") {
      const file = searchParams.get("file");
      if (!file) {
        return Response.json({ error: "file parameter required" }, { status: 400 });
      }
      const result = restoreBackup(file);
      return Response.json({ success: true, ...result });
    }

    if (action === "clean") {
      const keepDays = parseInt(searchParams.get("keepDays") || "7", 10);
      const result = cleanOldBackups(keepDays);
      return Response.json({ success: true, ...result });
    }

    // Default: create backup
    const result = createBackup();
    return Response.json({ success: true, backup: result });
  } catch (err) {
    return Response.json({ error: err.message }, { status: 500 });
  }
}

export async function DELETE(request) {
  if (!validateDashboardSession(request)) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const file = searchParams.get("file");

  if (!file) {
    return Response.json({ error: "file parameter required" }, { status: 400 });
  }

  try {
    // Sanitize to prevent path traversal
    const sanitized = path.basename(file);
    const filePath = path.join(BACKUP_DIR, sanitized);

    if (!fs.existsSync(filePath)) {
      return Response.json({ error: "Backup file not found" }, { status: 404 });
    }

    fs.unlinkSync(filePath);
    return Response.json({ success: true, deleted: sanitized });
  } catch (err) {
    return Response.json({ error: err.message }, { status: 500 });
  }
}
