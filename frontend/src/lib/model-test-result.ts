export type ModelTestResult = {
  ok: boolean;
  code: string;
  httpStatus?: number;
  latencyMs?: number;
  message: string;
  detail?: string;
  hint?: string;
};

const MAX_DETAIL = 240;
const MAX_MESSAGE = 180;
const SECRET_PATTERNS = [
  /["']?Authorization["']?\s*:\s*["']?(?:Bearer\s+)?[^\s,"';}]+["']?/gi,
  /["']?(?:api[_-]?key|access[_-]?token|auth[_-]?token)["']?\s*[:=]\s*["']?[^\s,"';}]+["']?/gi,
  /\b(?:sk|pk|rk)-[A-Za-z0-9._-]{6,}/g,
  /\bBearer\s+[A-Za-z0-9._~+\/-]+=*/gi,
];

function text(value: unknown): string {
  if (value == null) return '';
  if (typeof value === 'string') return value;
  try { return JSON.stringify(value); } catch { return String(value); }
}

export function sanitizeModelTestText(value: unknown, maxLength = MAX_DETAIL): string {
  let clean = text(value)
    .replace(/<(script|style)\b[^>]*>[\s\S]*?<\/\1>/gi, ' ')
    .replace(/<[^>]*>/g, ' ')
    .replace(/[\u0000-\u001F\u007F]/g, ' ');
  for (const pattern of SECRET_PATTERNS) clean = clean.replace(pattern, '[redacted]');
  clean = clean.replace(/\s+/g, ' ').trim();
  return clean.length > maxLength ? `${clean.slice(0, maxLength - 1).trimEnd()}…` : clean;
}

function finiteNumber(value: unknown): number | undefined {
  const number = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(number) ? number : undefined;
}

function unwrap(source: any): any {
  if (!source || typeof source !== 'object') return {};
  if (source.error && typeof source.error === 'object') {
    return { ...source, ...source.error, hint: source.error.hint ?? source.hint };
  }
  return source;
}

function format(source: any, fallbackStatus?: number): ModelTestResult {
  const value = unwrap(source);
  const success = value.success === true || value.status === 'ok';
  const code = sanitizeModelTestText(value.code || value.status || (success ? 'ok' : 'test_failed'), 48) || 'test_failed';
  const message = sanitizeModelTestText(value.message || (success ? 'Available' : 'Test failed'), MAX_MESSAGE);
  const detail = sanitizeModelTestText(value.body ?? value.detail ?? value.data, MAX_DETAIL);
  const hint = sanitizeModelTestText(value.hint, MAX_MESSAGE);
  const httpStatus = finiteNumber(value.http_status ?? value.httpStatus ?? fallbackStatus);
  const latencyMs = finiteNumber(value.latency_ms ?? value.latencyMs);
  return {
    ok: success,
    code,
    ...(httpStatus !== undefined ? { httpStatus } : {}),
    ...(latencyMs !== undefined ? { latencyMs } : {}),
    message,
    ...(detail && detail !== message ? { detail } : {}),
    ...(hint ? { hint } : {}),
  };
}

export function formatModelTestResponse(response: unknown): ModelTestResult {
  return format(response);
}

export function formatModelTestError(error: any): ModelTestResult {
  const envelope = error?.envelope && typeof error.envelope === 'object' ? error.envelope : {};
  const detail = error?.detail && typeof error.detail === 'object' ? error.detail : {};
  const hasStructuredCode = detail.code || detail.status || envelope?.error?.code || envelope?.error?.status;
  return format({
    ...envelope,
    ...detail,
    status: detail.status || (!hasStructuredCode && error instanceof TypeError ? 'network_error' : undefined),
    message: detail.message || envelope?.error?.message || error?.message,
  }, error?.status);
}
