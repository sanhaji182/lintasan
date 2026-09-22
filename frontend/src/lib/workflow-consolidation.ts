export type CallableKind = 'route' | 'provider';

export type CallableModel = {
  rowKey: string;
  id: string;
  label: string;
  kind: CallableKind;
  route: string | null;
  provider: string | null;
  account: string | null;
  connectionId: string | null;
  health: string;
  supportsStreaming: boolean | null;
  contextWindow: number | null;
  price: unknown | null;
  capabilities: string[];
};

export function callableRowKey(row: Pick<CallableModel, 'kind' | 'id' | 'connectionId' | 'account' | 'provider'>): string {
  const identity = row.kind === 'route' ? '' : row.connectionId || row.account || row.provider || '';
  return JSON.stringify([row.kind, identity, row.id]);
}

type CatalogInput = {
  aliases?: Record<string, string | { model?: string; target?: string }>;
  combos?: any[];
  connections?: any[];
  models?: any[];
};

function clean(value: unknown): string | null {
  const result = typeof value === 'string' ? value.trim() : '';
  return result || null;
}

function searchable(value: unknown): string {
  return String(value ?? '').toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim();
}

function fuzzy(haystack: string, query: string): boolean {
  const h = searchable(haystack), q = searchable(query);
  if (!q) return true;
  if (h.includes(q)) return true;
  let pos = 0;
  for (const ch of q.replace(/\s/g, '')) {
    pos = h.replace(/\s/g, '').indexOf(ch, pos);
    if (pos < 0) return false;
    pos++;
  }
  return true;
}

export function buildCallableCatalog(input: CatalogInput): CallableModel[] {
  const connections = new Map((input.connections || []).map(c => [String(c.id), c]));
  const out: CallableModel[] = [];
  const seen = new Set<string>();
  const push = (row: Omit<CallableModel, 'rowKey'>) => {
    const rowKey = callableRowKey(row);
    if (!row.id || seen.has(rowKey)) return;
    seen.add(rowKey);
    out.push({ ...row, rowKey });
  };

  for (const [id, cfg] of Object.entries(input.aliases || {})) {
    const target = typeof cfg === 'string' ? cfg : clean(cfg?.model) || clean(cfg?.target);
    push({ id, label: id, kind: 'route', route: target, provider: 'Alias', account: null, connectionId: null,
      health: 'configured', supportsStreaming: null, contextWindow: null, price: null, capabilities: [] });
  }
  for (const combo of input.combos || []) {
    const id = clean(combo.name) || clean(combo.provider);
    if (!id) continue;
    const entries = Array.isArray(combo.entries) ? combo.entries : [];
    push({ id, label: id, kind: 'route', route: (combo.models || entries.map((e: any) => e.model)).filter(Boolean).join(' → ') || null,
      provider: 'Combo', account: null, connectionId: null, health: 'configured',
      supportsStreaming: null, contextWindow: null, price: null, capabilities: [] });
  }

  const models = [...(input.models || [])];
  for (const model of models) {
    const id = clean(model.id) || clean(model.model_id);
    if (!id) continue;
    const connId = clean(model.connection_id);
    const conn = connId ? connections.get(connId) : null;
    const rawCaps = model.capabilities;
    const capabilities = Array.isArray(rawCaps) ? rawCaps.map(String) : [];
    push({ id, label: clean(model.display_name) || clean(model.model_name) || id, kind: 'provider',
      route: null, provider: clean(model.owned_by) || clean(conn?.format), account: clean(conn?.name) || connId, connectionId: connId,
      health: clean(model.health_status) || clean(conn?.health_status) || (conn && !conn.is_active ? 'inactive' : 'unknown'),
      supportsStreaming: typeof model.supports_streaming === 'boolean' ? model.supports_streaming :
        (typeof conn?.supports_streaming === 'boolean' ? conn.supports_streaming : null),
      contextWindow: Number.isFinite(model.context_window_tokens) ? model.context_window_tokens :
        (Number.isFinite(model.context_window) ? model.context_window : null),
      price: model.price ?? model.pricing ?? null, capabilities });
  }
  return out;
}

export function filterCallableModels(rows: CallableModel[], query: string): CallableModel[] {
  return rows.filter(row => fuzzy([row.id, row.label, row.route, row.provider, row.account, row.health].join(' '), query));
}

export function rememberRecentModel(current: string[], id: string, limit = 5): string[] {
  return [id, ...current.filter(item => item !== id)].slice(0, limit);
}

export function selectCatalogModel(rows: CallableModel[], requested: string, remembered: string): string {
  const callableIDs = new Set(rows.map(row => row.id));
  if (requested && callableIDs.has(requested)) return requested;
  if (remembered && callableIDs.has(remembered)) return remembered;
  return rows[0]?.id || '';
}

export function groupCallableModels(rows: CallableModel[], recent: string[], recommended?: string | null) {
  const priority = rememberRecentModel(recent, recommended || '', 6).filter(Boolean);
  const used = new Set<string>();
  const pick = (predicate: (row: CallableModel) => boolean) => rows.filter(row => predicate(row) && !used.has(row.id)).map(row => (used.add(row.id), row));
  const groups = [
    { label: 'Recommended & Recent', items: priority.flatMap(id => rows.filter(row => row.id === id)).filter(row => !used.has(row.id)).map(row => (used.add(row.id), row)) },
    { label: 'Aliases & Combos', items: pick(row => row.kind === 'route') },
    { label: 'Provider Models', items: pick(row => row.kind === 'provider') },
  ];
  return groups.filter(group => group.items.length > 0);
}

export function shouldUseStreaming(model: Pick<CallableModel, 'kind' | 'supportsStreaming'> | undefined): boolean {
  return model?.supportsStreaming !== false;
}

export function comboOrderFingerprint(combos: Array<{ id: string }>): string {
  return JSON.stringify(combos.map(combo => combo.id));
}

export function buildPolicyPayload(smart: {
  ml_router_enabled: unknown;
  ml_router_cheap_model: unknown;
  ml_router_expensive_model: unknown;
  ml_router_threshold: unknown;
  cost_quality_floor: unknown;
  cost_expensive_anchor: unknown;
}) {
  return {
    ml_router_enabled: smart.ml_router_enabled,
    ml_router_cheap_model: smart.ml_router_cheap_model,
    ml_router_expensive_model: smart.ml_router_expensive_model,
    ml_router_threshold: smart.ml_router_threshold,
    cost_quality_floor: smart.cost_quality_floor,
    cost_expensive_anchor: smart.cost_expensive_anchor,
  };
}

export function buildQuotaPayload(rows: Array<{ connId: string; maxPerDay: string }>) {
  const quota_limits: Record<string, { max_tokens_per_day: number }> = {};
  for (const row of rows) {
    const id = row.connId.trim();
    const max = Number.parseInt(row.maxPerDay, 10);
    if (id && Number.isFinite(max) && max > 0) quota_limits[id] = { max_tokens_per_day: max };
  }
  return { quota_limits };
}

export function routingDirtyState(dirty: { policy: boolean; combos: boolean; quotas: boolean }) {
  const scopes = [dirty.policy && 'Policies', dirty.combos && 'Combos', dirty.quotas && 'Quotas'].filter(Boolean) as string[];
  return { count: scopes.length, scopes };
}

function comparable(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}

export function analyticsScope(globalTotal: unknown, snapshotTotal: unknown) {
  const globalLabel = comparable(globalTotal) ? 'All recorded requests' : 'All-recorded request count unavailable';
  const snapshotLabel = comparable(snapshotTotal) ? `${snapshotTotal} retained request rows` : 'Retained request row count unavailable';
  if (!comparable(globalTotal) || !comparable(snapshotTotal)) {
    return { globalLabel, snapshotLabel, note: 'Reconciliation is unknown because comparable counts are unavailable.', discrepancy: null, reconciled: false, state: 'unknown' as const };
  }
  const difference = globalTotal - snapshotTotal;
  const note = difference > 0
    ? `The all-recorded counter exceeds the retained rows by ${difference}. These independently collected sources do not reconcile.`
    : difference < 0
      ? `The retained rows exceed the all-recorded counter by ${Math.abs(difference)}. These independently collected sources do not reconcile.`
      : 'The independently collected counts match.';
  return {
    globalLabel,
    snapshotLabel,
    note,
    discrepancy: difference,
    reconciled: difference === 0,
    state: difference === 0 ? 'reconciled' as const : 'mismatch' as const,
  };
}

export function sourceFreshness(collectedAt: number | null, now: number, failed: boolean, staleAfterMs = 30_000) {
  if (collectedAt == null) return { state: 'unknown' as const, ageMs: null };
  const ageMs = Math.max(0, now - collectedAt);
  return { state: failed || ageMs > staleAfterMs ? 'stale' as const : 'fresh' as const, ageMs };
}
