type Source<T> = PromiseSettledResult<T>;

type Health = { status?: unknown; version?: unknown };
type Stats = { total_requests?: unknown; cache_hit_rate?: unknown; avg_latency?: unknown; uptime?: unknown };
export type OverviewLog = { model?: string; provider?: string; status?: number; input_tokens?: number; output_tokens?: number; latency_ms?: number; created_at?: string };
type Connection = { id: string; name: string; is_active: number | boolean; format?: string };

const validNonNegativeNumber = (value: unknown): value is number =>
  typeof value === 'number' && Number.isFinite(value) && value >= 0;

const validHealth = (value: Health | null): value is Health & { status: string } =>
  !!value && typeof value.status === 'string' && value.status.trim().length > 0;

const validLogs = (value: unknown): value is OverviewLog[] => Array.isArray(value) && value.every(log =>
  !!log && typeof log === 'object' && validNonNegativeNumber(log.input_tokens) && validNonNegativeNumber(log.output_tokens)
);

const validConnections = (value: unknown): value is Connection[] => Array.isArray(value) && value.every(connection =>
  !!connection && typeof connection.id === 'string' && typeof connection.name === 'string'
  && (typeof connection.is_active === 'boolean' || connection.is_active === 0 || connection.is_active === 1)
);

function readableNumber(value: number) {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return value.toLocaleString();
}

export type OverviewState = ReturnType<typeof deriveOverview>;

export function deriveOverview(input: {
  health: Source<Health>;
  stats: Source<Stats>;
  logs: Source<OverviewLog[]>;
  connections: Source<Connection[]>;
}) {
  const healthValue = input.health.status === 'fulfilled' ? input.health.value : null;
  const stats = input.stats.status === 'fulfilled' && input.stats.value && typeof input.stats.value === 'object' ? input.stats.value : null;
  const logsValue = input.logs.status === 'fulfilled' ? input.logs.value : null;
  const connectionsValue = input.connections.status === 'fulfilled' ? input.connections.value : null;
  const health = validHealth(healthValue) ? healthValue : null;
  const logs = validLogs(logsValue) ? logsValue : null;
  const connections = validConnections(connectionsValue) ? connectionsValue : null;
  const hasRequests = validNonNegativeNumber(stats?.total_requests);
  const hasLatency = validNonNegativeNumber(stats?.avg_latency);
  const statsPartial = input.stats.status === 'fulfilled' && (!hasRequests || !hasLatency);
  const failedSources = [
    input.health.status === 'rejected' ? 'Gateway health' : !health ? 'Gateway health (partial)' : '',
    input.stats.status === 'rejected' ? 'Traffic stats' : statsPartial ? 'Traffic stats (partial)' : '',
    input.logs.status === 'rejected' ? 'Recent requests' : !logs ? 'Recent requests (partial)' : '',
    input.connections.status === 'rejected' ? 'Connections' : !connections ? 'Connections (partial)' : '',
  ].filter(Boolean);
  const healthy = health?.status === 'ok' || health?.status === 'healthy';
  const tokenCount = logs?.reduce((sum, log) => sum + log.input_tokens! + log.output_tokens!, 0);
  const active = connections?.filter(connection => Boolean(connection.is_active)).length;

  return {
    gateway: {
      label: health ? (healthy ? 'Operational' : 'Attention needed') : 'Status unavailable',
      detail: health ? [typeof health.version === 'string' ? health.version : '', typeof stats?.uptime === 'string' && stats.uptime ? `Up ${stats.uptime}` : ''].filter(Boolean).join(' · ') : 'Health data is unavailable or incomplete',
      tone: health ? (healthy ? 'success' : 'warning') : 'neutral',
    },
    requests: {
      value: hasRequests ? readableNumber(stats!.total_requests as number) : 'Unavailable',
      detail: 'Recorded gateway requests',
    },
    latency: {
      value: hasLatency ? `${Math.round(stats!.avg_latency as number)}ms` : 'Unavailable',
      detail: 'Average successful response',
    },
    tokens: {
      value: logs ? readableNumber(tokenCount!) : 'Unavailable',
      detail: logs ? `Across ${logs.length} recent request${logs.length === 1 ? '' : 's'}` : 'Recent token data is incomplete',
    },
    providers: {
      value: connections ? `${active} / ${connections.length}` : 'Unavailable',
      detail: 'Active provider accounts',
    },
    failedSources,
    recentRequests: logs || [],
    connections: connections || [],
  };
}
