export type CallableKind = 'route' | 'cloud_agent' | 'provider';

export type CallableModel = {
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
  const push = (row: CallableModel) => {
    if (!row.id || seen.has(row.id)) return;
    seen.add(row.id);
    out.push(row);
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
    const cloud = entries.some((entry: any) => String(entry.model || '').startsWith('hoplite-'));
    push({ id, label: id, kind: 'route', route: (combo.models || entries.map((e: any) => e.model)).filter(Boolean).join(' → ') || null,
      provider: cloud ? 'Cloud Agent combo' : 'Combo', account: null, connectionId: null, health: 'configured',
      supportsStreaming: cloud ? false : null, contextWindow: null, price: null, capabilities: [] });
  }

  const models = [...(input.models || [])].sort((a, b) => {
    const ac = a.provider_kind === 'cloud_agent' || String(a.id || '').startsWith('hoplite-') ? 0 : 1;
    const bc = b.provider_kind === 'cloud_agent' || String(b.id || '').startsWith('hoplite-') ? 0 : 1;
    return ac - bc;
  });
  for (const model of models) {
    const id = clean(model.id) || clean(model.model_id);
    if (!id) continue;
    const connId = clean(model.connection_id);
    const conn = connId ? connections.get(connId) : null;
    const cloud = model.provider_kind === 'cloud_agent' || conn?.provider_kind === 'cloud_agent' || id.startsWith('hoplite-');
    const rawCaps = model.capabilities;
    const capabilities = Array.isArray(rawCaps) ? rawCaps.map(String) : [];
    push({ id, label: clean(model.display_name) || clean(model.model_name) || id, kind: cloud ? 'cloud_agent' : 'provider',
      route: null, provider: clean(model.owned_by) || clean(conn?.format), account: clean(conn?.name), connectionId: connId,
      health: clean(model.health_status) || clean(conn?.health_status) || (conn && !conn.is_active ? 'inactive' : 'unknown'),
      supportsStreaming: typeof model.supports_streaming === 'boolean' ? model.supports_streaming :
        (typeof conn?.supports_streaming === 'boolean' ? conn.supports_streaming : null),
      contextWindow: Number.isFinite(model.context_window) ? model.context_window : null,
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

export function groupCallableModels(rows: CallableModel[], recent: string[], recommended?: string | null) {
  const priority = rememberRecentModel(recent, recommended || '', 6).filter(Boolean);
  const used = new Set<string>();
  const pick = (predicate: (row: CallableModel) => boolean) => rows.filter(row => predicate(row) && !used.has(row.id)).map(row => (used.add(row.id), row));
  const groups = [
    { label: 'Recommended & Recent', items: priority.flatMap(id => rows.filter(row => row.id === id)).filter(row => !used.has(row.id)).map(row => (used.add(row.id), row)) },
    { label: 'Aliases & Combos', items: pick(row => row.kind === 'route') },
    { label: 'Cloud Agents', items: pick(row => row.kind === 'cloud_agent') },
    { label: 'Provider Models', items: pick(row => row.kind === 'provider') },
  ];
  return groups.filter(group => group.items.length > 0);
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

export function analyticsScope(globalTotal: number, snapshotTotal: number) {
  const difference = Math.max(0, globalTotal - snapshotTotal);
  return {
    globalLabel: 'All recorded requests',
    snapshotLabel: `${snapshotTotal} retained request rows`,
    note: difference ? `${difference} requests are outside the retained log snapshot or its current filters.` : 'Global and retained snapshot totals currently reconcile.',
    reconciled: difference === 0,
  };
}
