import { getDashboardPassword } from "@/lib/auth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Rate limiting: max 5 attempts per IP per 5 minutes
const attempts = new Map();
const MAX_ATTEMPTS = 5;
const WINDOW_MS = 5 * 60 * 1000; // 5 minutes

function getClientIP(request) {
  return request.headers.get("x-forwarded-for")?.split(",")[0]?.trim()
    || request.headers.get("x-real-ip")
    || "unknown";
}

function isRateLimited(ip) {
  const now = Date.now();
  const record = attempts.get(ip);

  if (!record) return false;

  // Clean expired entries
  if (now - record.firstAttempt > WINDOW_MS) {
    attempts.delete(ip);
    return false;
  }

  return record.count >= MAX_ATTEMPTS;
}

function recordAttempt(ip) {
  const now = Date.now();
  const record = attempts.get(ip);

  if (!record || now - record.firstAttempt > WINDOW_MS) {
    attempts.set(ip, { count: 1, firstAttempt: now });
  } else {
    record.count++;
  }
}

// Cleanup stale entries every 10 minutes
setInterval(() => {
  const now = Date.now();
  for (const [ip, record] of attempts) {
    if (now - record.firstAttempt > WINDOW_MS) {
      attempts.delete(ip);
    }
  }
}, 10 * 60 * 1000);

export async function POST(request) {
  const ip = getClientIP(request);

  // Check rate limit
  if (isRateLimited(ip)) {
    const record = attempts.get(ip);
    const retryAfter = Math.ceil((record.firstAttempt + WINDOW_MS - Date.now()) / 1000);
    return Response.json(
      { error: "Too many login attempts. Try again later.", retryAfter },
      { status: 429, headers: { "Retry-After": String(retryAfter) } }
    );
  }

  const { password } = await request.json();
  const correctPassword = getDashboardPassword();

  if (password !== correctPassword) {
    recordAttempt(ip);
    const record = attempts.get(ip);
    const remaining = MAX_ATTEMPTS - record.count;
    return Response.json(
      { error: "Invalid password", attemptsRemaining: Math.max(0, remaining) },
      { status: 401 }
    );
  }

  // Successful login — clear attempts
  attempts.delete(ip);

  const token = Buffer.from(password + ":" + Date.now()).toString("base64");

  return Response.json({ success: true }, {
    headers: {
      "Set-Cookie": `sr_session=${token}; Path=/; HttpOnly; SameSite=Strict; Max-Age=86400`,
    },
  });
}
