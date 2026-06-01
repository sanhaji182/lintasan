const BASE = '';

// authHeaders builds the request headers, always attaching the JWT as a
// Bearer token when present. This is the SINGLE source of truth for dashboard
// auth on the client: every API call (JSON, text, blob, or streaming) must go
// through here so auth never depends on cookie state. Relying on the cookie
// alone caused a split-brain transport — some calls sent Bearer, others relied
// on the cookie — which desynced and produced a mix of 200/401 responses
// (partial dashboard render + false "Session expired") once auth became
// fail-closed in v2.3.1.
function authHeaders(extra?: Record<string, string>): Record<string, string> {
  const headers: Record<string, string> = { ...(extra || {}) };
  if (typeof window !== 'undefined') {
    const token = localStorage.getItem('lintasan_token');
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
  }
  return headers;
}

// handleAuthFailure centralizes the 401/403 reactions so every transport
// behaves identically. Returns true if it consumed the response (caller should
// stop), false if the caller should continue handling.
function reactToStatus(status: number, bodyError?: string) {
  if (status === 401) {
    if (typeof window !== 'undefined' && !window.location.pathname.startsWith('/login')) {
      localStorage.removeItem('lintasan_token');
      localStorage.removeItem('lintasan_user');
      window.location.href = '/login';
    }
    return new Error('Session expired. Please sign in again.');
  }
  if (status === 403 && bodyError === 'password_change_required') {
    if (
      typeof window !== 'undefined' &&
      !window.location.pathname.startsWith('/change-password')
    ) {
      window.location.href = '/change-password';
    }
    return new Error('Password change required. Please update your password to continue.');
  }
  return null;
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const headers = authHeaders({
    'Content-Type': 'application/json',
    ...(options?.headers as Record<string, string> || {})
  });

  const res = await fetch(`${BASE}${path}`, { ...options, headers });

  // A 401 from the login endpoint itself is a CREDENTIAL rejection, not a
  // session expiry. Surface the server's actual message ("invalid credentials")
  // via the generic handler below — never clear tokens, redirect, or show the
  // misleading "Session expired" copy when the user is actively signing in.
  const isLoginEndpoint = path === '/api/auth/login';

  if (res.status === 401 && !isLoginEndpoint) {
    throw reactToStatus(401)!;
  }
  if (res.status === 403) {
    const body = await res.clone().json().catch(() => ({}));
    const err = reactToStatus(403, body.error);
    if (err) throw err;
    // Other 403s (e.g. admin-only routes) fall through to the generic handler.
  }
  if (!res.ok) {
    // Parse the body and surface a rich ApiError so callers can read the
    // OpenAI-standard envelope (code/type/message/param/hint) instead of a
    // flattened string. ApiError preserves the original shape on `.detail`.
    const body = await res.json().catch(() => ({}));
    const inner = (body && typeof body === 'object' && body.error !== undefined) ? body.error : body;
    const message =
      (inner && typeof inner === 'object' && inner.message) ? inner.message :
      (typeof inner === 'string' ? inner : res.statusText);
    const err: any = new Error(message);
    err.status = res.status;
    // Preserve the full envelope (code/type/param/hint) and the Lintasan fields
    // (latency_ms, data, success) so the UI can show them all.
    err.detail = (inner && typeof inner === 'object') ? inner : { message: String(inner) };
    err.envelope = body; // the full {data, error, hint, latency_ms, success} shape
    throw err;
  }
  return res.json();
}

// apiRaw performs an authenticated fetch and returns the raw Response so callers
// can read .text(), .blob(), or stream .body — used for /metrics, file
// downloads, MCP JSON-RPC, and streaming chat completions. It attaches the same
// Bearer token and applies the same 401/403 reactions, so these previously
// cookie-only calls now use the unified transport. On 401/403 it throws (after
// triggering redirect); otherwise it returns the Response untouched, including
// non-2xx (so callers can inspect error bodies for non-auth failures).
async function apiRaw(path: string, options?: RequestInit): Promise<Response> {
  const headers = authHeaders(options?.headers as Record<string, string>);
  const res = await fetch(`${BASE}${path}`, { ...options, headers });
  if (res.status === 401) {
    throw reactToStatus(401)!;
  }
  if (res.status === 403) {
    const body = await res.clone().json().catch(() => ({}));
    const err = reactToStatus(403, body.error);
    if (err) throw err;
  }
  return res;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  put: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined }),
  patch: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PATCH', body: body ? JSON.stringify(body) : undefined }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
  // Raw authenticated fetch for non-JSON responses (text/blob/stream).
  raw: (path: string, options?: RequestInit) => apiRaw(path, options)
};
