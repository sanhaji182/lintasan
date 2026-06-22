<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import StatCard from '$lib/components/StatCard.svelte';
  import StatusBadge from '$lib/components/StatusBadge.svelte';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import {
    Activity, Zap, Clock, Coins, RefreshCw, ArrowUpRight,
    CircleAlert, Link2, CheckCircle2
  } from 'lucide-svelte/icons';

  interface DashboardStats {
    total_requests: number;
    cache_hit_rate: number;
    avg_latency: number;
    active_connections: number;
    uptime: string;
  }

  interface RecentRequest {
    id: string;
    model: string;
    provider: string;
    status: number;
    input_tokens: number;
    output_tokens: number;
    created_at?: string;
    latency_ms?: number;
    cached?: boolean;
  }

  interface ConnectionStatus {
    id: string;
    name: string;
    base_url: string;
    format: string;
    is_active: number;
    priority: number;
    models_count: number;
    created_at: string;
  }

  let stats = $state<DashboardStats | null>(null);
  let recentRequests = $state<RecentRequest[]>([]);
  let connections = $state<ConnectionStatus[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let refreshing = $state(false);

  async function fetchDashboardData() {
    try {
      error = null;
      // Fetch independently — don't let one hanging endpoint block the whole page
      const statsPromise = api.get<DashboardStats>('/api/dashboard/stats').catch(() => null);
      const logsPromise = api.get<{ data: RecentRequest[] }>('/api/logs').catch(() => ({ data: [] }));
      const connectionsPromise = api.get<{ data: ConnectionStatus[] }>('/api/connections').catch(() => ({ data: [] }));

      const [statsData, logsData, connectionsData] = await Promise.all([
        statsPromise, logsPromise, connectionsPromise
      ]);
      stats = (statsData as any)?.data || statsData;
      recentRequests = (logsData?.data || []).slice(0, 10);
      connections = connectionsData?.data || [];
    } catch (e: any) {
      error = e.message || 'Failed to load dashboard data';
    } finally {
      loading = false;
      refreshing = false;
    }
  }

  async function handleRefresh() {
    refreshing = true;
    await fetchDashboardData();
  }

  onMount(() => {
    fetchDashboardData();
  });

  function formatNumber(n: number): string {
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
    if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
    return n.toLocaleString();
  }

  function formatLatency(ms: number): string {
    return ms >= 1000 ? (ms / 1000).toFixed(1) + 's' : ms + 'ms';
  }

  function timeAgo(ts: string): string {
    const diff = Date.now() - new Date(ts).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ago`;
    return `${Math.floor(hrs / 24)}d ago`;
  }

  const activeConnections = $derived(connections.filter(c => c.is_active).length);
  const totalConnections = $derived(connections.length);
</script>

<svelte:head>
  <title>Overview — Lintasan</title>
</svelte:head>

{#if loading}
  <!-- Skeleton loading state -->
  <div style="margin-bottom: 24px;">
    <div class="flex items-center justify-between" style="margin-bottom: 24px;">
      <div>
        <div class="skeleton" style="width: 200px; height: 24px; margin-bottom: 8px;"></div>
        <div class="skeleton" style="width: 280px; height: 14px;"></div>
      </div>
      <div class="skeleton" style="width: 90px; height: 36px; border-radius: var(--radius-sm);"></div>
    </div>

    <div
      class="grid gap-5"
      style="grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); margin-bottom: 24px;"
    >
      {#each Array(4) as _}
        <div class="card" style="padding: 20px;">
          <div class="skeleton" style="width: 44px; height: 44px; border-radius: 12px; margin-bottom: 16px;"></div>
          <div class="skeleton" style="width: 60%; height: 28px; margin-bottom: 6px;"></div>
          <div class="skeleton" style="width: 80%; height: 12px;"></div>
        </div>
      {/each}
    </div>

    <div class="grid gap-5" style="grid-template-columns: 1fr 340px;">
      <div class="card" style="padding: 0; overflow: hidden;">
        <div style="padding: 18px 20px; border-bottom: 1px solid var(--color-border);">
          <div class="skeleton" style="width: 130px; height: 16px;"></div>
        </div>
        <div style="padding: 16px 20px;">
          {#each Array(5) as _}
            <div class="flex items-center gap-3" style="margin-bottom: 14px;">
              <div class="skeleton" style="width: 28px; height: 28px; border-radius: 6px; flex-shrink: 0;"></div>
              <div style="flex: 1;">
                <div class="skeleton" style="width: 70%; height: 12px; margin-bottom: 4px;"></div>
                <div class="skeleton" style="width: 40%; height: 10px;"></div>
              </div>
              <div class="skeleton" style="width: 50px; height: 20px; border-radius: 999px;"></div>
            </div>
          {/each}
        </div>
      </div>
      <div class="card" style="padding: 0; overflow: hidden;">
        <div style="padding: 18px 20px; border-bottom: 1px solid var(--color-border);">
          <div class="skeleton" style="width: 100px; height: 16px;"></div>
        </div>
        <div style="padding: 16px 20px;">
          {#each Array(4) as _}
            <div class="flex items-center gap-3" style="margin-bottom: 12px;">
              <div class="skeleton" style="width: 32px; height: 32px; border-radius: 8px; flex-shrink: 0;"></div>
              <div style="flex: 1;">
                <div class="skeleton" style="width: 60%; height: 12px; margin-bottom: 4px;"></div>
                <div class="skeleton" style="width: 40%; height: 10px;"></div>
              </div>
              <div class="skeleton" style="width: 55px; height: 20px; border-radius: 999px;"></div>
            </div>
          {/each}
        </div>
      </div>
    </div>
  </div>
{:else if error}
  <div class="card" style="text-align: center;">
    <div class="flex flex-col items-center gap-3">
      <CircleAlert size={32} style="color: var(--color-error);" />
      <div style="font-size: 14px; color: var(--color-fg-1);">{error}</div>
      <button class="btn-primary" onclick={handleRefresh}>
        <RefreshCw size={14} style="display: inline; vertical-align: middle; margin-right: 4px;" />
        Retry
      </button>
    </div>
  </div>
{:else}
  <!-- Page header -->
  <div class="flex items-center justify-between" style="margin-bottom: 24px;">
    <div>
      <h2 style="font-size: 20px; font-weight: 700; color: var(--color-fg-0); letter-spacing: -0.3px;">
        Dashboard Overview
      </h2>
      <p style="font-size: 13px; color: var(--color-fg-2); margin-top: 2px;">
        Monitor your AI gateway performance and connections
      </p>
    </div>
    <button
      class="btn-secondary"
      onclick={handleRefresh}
      disabled={refreshing}
      style="display: flex; align-items: center; gap: 6px;"
    >
      <RefreshCw
        size={14}
        style="animation: {refreshing ? 'spin 0.8s linear infinite' : 'none'};"
      />
      Refresh
    </button>
  </div>

  <!-- Stat cards -->
  <div
    class="grid gap-5"
    style="grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); margin-bottom: 24px; animation: fadeInUp 0.4s ease-out;"
  >
    <StatCard
      label="Total Requests"
      value={formatNumber((stats as any)?.total_requests ?? 0)}
      icon={Activity}
      color="var(--color-primary)"
      subtitle="Last 24 hours"
    />
    <StatCard
      label="Cache Hit Rate"
      value="{(stats as any)?.cache_hit_rate ?? 0}%"
      icon={Zap}
      color="var(--color-success)"
      subtitle="Requests served from cache"
    />
    <StatCard
      label="Avg Latency"
      value={formatLatency((stats as any)?.avg_latency ?? 0)}
      icon={Clock}
      color="var(--color-warning)"
      subtitle="Mean response time"
    />
    <StatCard
      label="Tokens Today"
      value="0"
      icon={Coins}
      color="var(--color-purple)"
      subtitle="Total tokens processed"
    />
  </div>

  <!-- Bottom section: two columns -->
  <div class="grid gap-5 dashboard-grid-bottom">
    <!-- Recent requests table -->
    <div class="card" style="padding: 0; overflow: hidden;">
      <div
        class="flex items-center justify-between"
        style="padding: 18px 20px; border-bottom: 1px solid var(--color-border);"
      >
        <div class="flex items-center gap-2">
          <Activity size={16} style="color: var(--color-primary);" />
          <span style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">Recent Requests</span>
        </div>
        <a
          href="/dashboard/logs"
          class="flex items-center gap-1"
          style="font-size: 12px; font-weight: 500; color: var(--color-primary); text-decoration: none;"
        >
          View all
          <ArrowUpRight size={13} />
        </a>
      </div>

      {#if recentRequests.length === 0}
        <EmptyState
          icon={Activity}
          title="No recent requests"
          description="Requests will appear here once traffic flows through the gateway"
        />
      {:else}
        <div style="overflow-x: auto;">
          <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
            <thead>
              <tr style="background: var(--color-bg-body);">
                <th class="table-header">Model</th>
                <th class="table-header">Provider</th>
                <th class="table-header">Status</th>
                <th class="table-header">Latency</th>
                <th class="table-header">Tokens</th>
                <th class="table-header">Time</th>
              </tr>
            </thead>
            <tbody>
              {#each recentRequests as req, i}
                <tr
                  class="table-row"
                  style="animation: fadeInUp {0.3 + i * 0.05}s ease-out;"
                >
                  <td class="table-cell" style="font-weight: 500; color: var(--color-fg-0);">
                    <div class="flex items-center gap-2">
                      {#if req.cached}
                        <span
                          class="inline-block"
                          style="width: 6px; height: 6px; border-radius: 50%; background: var(--color-info);"
                          title="Cached"
                        ></span>
                      {/if}
                      {req.model}
                    </div>
                  </td>
                  <td class="table-cell">
                    <span
                      class="inline-block px-2 py-0.5 rounded-md text-xs font-mono"
                      style="background: var(--color-bg-body); color: var(--color-fg-2); font-size: 11px;"
                    >{req.provider}</span>
                  </td>
                  <td class="table-cell">
                    <StatusBadge status={req.status >= 200 && req.status < 300 ? 'success' : req.status >= 400 ? 'error' : 'pending'} />
                  </td>
                  <td class="table-cell font-mono" style="color: var(--color-fg-2);">
                    {formatLatency(req.latency_ms || 0)}
                  </td>
                  <td class="table-cell font-mono" style="color: var(--color-fg-2);">
                    {formatNumber((req.input_tokens || 0) + (req.output_tokens || 0))}
                  </td>
                  <td class="table-cell" style="color: var(--color-fg-3); font-size: 12px;">
                    {req.created_at ? timeAgo(req.created_at) : '—'}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- Connection status sidebar -->
    <div class="card" style="padding: 0; overflow: hidden;">
      <div
        class="flex items-center justify-between"
        style="padding: 18px 20px; border-bottom: 1px solid var(--color-border);"
      >
        <div class="flex items-center gap-2">
          <Link2 size={16} style="color: var(--color-primary);" />
          <span style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">Connections</span>
        </div>
        <a
          href="/dashboard/connections"
          class="flex items-center gap-1"
          style="font-size: 12px; font-weight: 500; color: var(--color-primary); text-decoration: none;"
        >
          Manage
          <ArrowUpRight size={13} />
        </a>
      </div>

      <!-- Summary strip -->
      <div
        class="flex items-center gap-4"
        style="padding: 14px 20px; border-bottom: 1px solid var(--color-border-light);"
      >
        <div class="flex items-center gap-1.5">
          <CheckCircle2 size={14} style="color: var(--color-success);" />
          <span style="font-size: 12px; font-weight: 500; color: var(--color-fg-2);">
            {activeConnections} active
          </span>
        </div>
        <div style="width: 1px; height: 14px; background: var(--color-border);" class="hidden sm:block"></div>
        <span style="font-size: 12px; color: var(--color-fg-3);">
          {totalConnections} total
        </span>
      </div>

      {#if connections.length === 0}
        <EmptyState
          icon={Link2}
          title="No connections"
          description="Add a provider connection to get started"
        />
      {:else}
        <div style="padding: 8px;">
          {#each connections as conn}
            <div
              class="flex items-center justify-between rounded-lg transition-all duration-200"
              style="padding: 10px 12px; margin-bottom: 2px;"
            >
              <div class="flex items-center gap-3">
                <div
                  class="flex items-center justify-center rounded-lg"
                  style="
                    width: 32px; height: 32px;
                    background: {conn.is_active ? 'var(--color-success-light)' : 'var(--color-error-light)'};
                  "
                >
                  <Link2
                    size={16}
                    style="color: {conn.is_active ? 'var(--color-success)' : 'var(--color-error)'};"
                  />
                </div>
                <div>
                  <div style="font-size: 13px; font-weight: 500; color: var(--color-fg-0);">{conn.name}</div>
                  <div style="font-size: 11px; color: var(--color-fg-3); font-family: var(--font-mono);">{conn.format}</div>
                </div>
              </div>
              <StatusBadge status={conn.is_active ? 'active' : 'inactive'} />
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .table-header {
    padding: 10px 16px;
    text-align: left;
    font-size: 11px;
    font-weight: 600;
    color: var(--color-fg-3);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .table-cell {
    padding: 12px 16px;
    border-bottom: 1px solid var(--color-border-light);
  }

  .table-row {
    transition: var(--transition);
  }

  .table-row:hover {
    background: var(--color-bg-body);
  }

  .table-row:last-child .table-cell {
    border-bottom: none;
  }

  .dashboard-grid-bottom {
    grid-template-columns: 1fr 340px;
  }

  @media (max-width: 1024px) {
    :global(.grid) {
      grid-template-columns: 1fr !important;
    }
  }
</style>
