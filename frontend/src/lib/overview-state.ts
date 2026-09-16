type Source<T> = PromiseSettledResult<T>;

type Health = { status?: string; version?: string };
type Stats = { total_requests?: number; cache_hit_rate?: number; avg_latency?: number; uptime?: string };
export type OverviewLog = { model?: string; provider?: string; status?: number; input_tokens?: number; output_tokens?: number; latency_ms?: number; created_at?: string };
type Connection = { id: string; name: string; is_active: number | boolean; format?: string };

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
  const failedSources = [
    input.health.status === 'rejected' ? 'Gateway health' : '',
    input.stats.status === 'rejected' ? 'Traffic stats' : '',
    input.logs.status === 'rejected' ? 'Recent requests' : '',
    input.connections.status === 'rejected' ? 'Connections' : '',
  ].filter(Boolean);

  const health = input.health.status === 'fulfilled' ? input.health.value : null;
  const stats = input.stats.status === 'fulfilled' ? input.stats.value : null;
  const logs = input.logs.status === 'fulfilled' ? input.logs.value : null;
  const connections = input.connections.status === 'fulfilled' ? input.connections.value : null;
  const healthy = health?.status === 'ok' || health?.status === 'healthy';
  const tokenCount = logs?.reduce((sum, log) => sum + (log.input_tokens || 0) + (log.output_tokens || 0), 0);
  const active = connections?.filter(connection => Boolean(connection.is_active)).length;

  return {
    gateway: {
      label: health ? (healthy ? 'Operational' : 'Attention needed') : 'Status unavailable',
      detail: health ? [health.version, stats?.uptime ? `Up ${stats.uptime}` : ''].filter(Boolean).join(' · ') : 'Health endpoint could not be reached',
      tone: health ? (healthy ? 'success' : 'warning') : 'neutral',
    },
    requests: {
      value: stats ? readableNumber(stats.total_requests || 0) : 'Unavailable',
      detail: 'Recorded gateway requests',
    },
    latency: {
      value: stats ? `${Math.round(stats.avg_latency || 0)}ms` : 'Unavailable',
      detail: 'Average successful response',
    },
    tokens: {
      value: logs ? readableNumber(tokenCount || 0) : 'Unavailable',
      detail: `Across ${logs?.length || 0} recent request${logs?.length === 1 ? '' : 's'}`,
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
