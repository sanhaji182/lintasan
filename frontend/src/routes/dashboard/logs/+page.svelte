<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import StatusBadge from '$lib/components/StatusBadge.svelte';
  import {
    ScrollText, Search, RefreshCw, Filter,
    Clock, Zap, ArrowDownToLine, ArrowUpFromLine,
    Database, Timer, X
  } from 'lucide-svelte/icons';

  interface LogEntry {
    id: string;
    connection_id?: string;
    model: string;
    status: number;
    input_tokens: number;
    output_tokens: number;
    provider?: string;
    latency_ms?: number;
    cached?: number;
    created_at?: string;
    endpoint?: string;
  }

  let allLogs = $state<LogEntry[]>([]);
  let loading = $state(true);
  let error = $state('');
  let searchQuery = $state('');
  let statusFilter = $state('all');
  let autoRefresh = $state(false);
  let refreshInterval = $state<ReturnType<typeof setInterval> | null>(null);
  let lastRefresh = $state('');

  const statusOptions = ['all', 'success', 'error', 'cached', 'pending', 'failed'];

  async function loadLogs() {
    try {
      const res = await api.get<{ data: LogEntry[] }>('/api/logs');
      allLogs = res.data || [];
      lastRefresh = new Date().toLocaleTimeString();
    } catch (e: any) {
      error = e.message || 'Failed to load logs';
    }
  }

  let logs = $derived.by(() => {
    let filtered = allLogs;
    if (searchQuery.trim()) {
      const q = searchQuery.trim().toLowerCase();
      filtered = filtered.filter(l =>
        (l.model || '').toLowerCase().includes(q) ||
        (l.provider || '').toLowerCase().includes(q)
      );
    }
    if (statusFilter !== 'all') {
      filtered = filtered.filter(l => {
        const display = l.cached ? 'cached' : (l.status >= 200 && l.status < 300) ? 'success' : (l.status >= 400) ? 'error' : 'pending';
        return display === statusFilter;
      });
    }
    return filtered;
  });

  onMount(async () => {
    loading = true;
    await loadLogs();
    loading = false;
  });

  onDestroy(() => {
    stopAutoRefresh();
  });

  function toggleAutoRefresh() {
    autoRefresh = !autoRefresh;
    if (autoRefresh) {
      startAutoRefresh();
    } else {
      stopAutoRefresh();
    }
  }

  function startAutoRefresh() {
    stopAutoRefresh();
    refreshInterval = setInterval(async () => {
      await loadLogs();
    }, 5000);
  }

  function stopAutoRefresh() {
    if (refreshInterval) {
      clearInterval(refreshInterval);
      refreshInterval = null;
    }
  }

  function handleSearch() {
    loadLogs();
  }

  function handleStatusChange(newStatus: string) {
    statusFilter = newStatus;
    loadLogs();
  }

  function clearFilters() {
    searchQuery = '';
    statusFilter = 'all';
    loadLogs();
  }

  function formatTimestamp(ts: string): string {
    try {
      const date = new Date(ts);
      return date.toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
      });
    } catch {
      return ts;
    }
  }

  function formatLatency(ms: number): string {
    if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
    return `${ms}ms`;
  }

  function getStatusDisplay(status: number, cached?: boolean): string {
    if (cached) return 'cached';
    if (status >= 200 && status < 300) return 'success';
    if (status >= 400) return 'error';
    return 'pending';
  }

  let hasActiveFilters = $derived(searchQuery.trim() !== '' || statusFilter !== 'all');
</script>

<div style="display: flex; flex-direction: column; gap: 24px;">
  <!-- Filters Card -->
  <div class="card" style="padding: 16px 20px;">
    <div class="flex items-center gap-3 flex-wrap">
      <!-- Search -->
      <div class="search-wrapper">
        <Search size={14} style="color: var(--color-fg-3); position: absolute; left: 10px; top: 50%; transform: translateY(-50%); pointer-events: none;" />
        <input
          class="input-field search-input"
          placeholder="Search logs..."
          bind:value={searchQuery}
          onkeydown={(e) => e.key === 'Enter' && handleSearch()}
        />
      </div>

      <!-- Status Filter -->
      <div class="flex items-center gap-1.5">
        <Filter size={14} style="color: var(--color-fg-3);" />
        <select
          class="input-field"
          style="width: 140px; font-size: 12px; padding: 7px 10px;"
          value={statusFilter}
          onchange={(e) => handleStatusChange((e.target as HTMLSelectElement).value)}
        >
          {#each statusOptions as opt}
            <option value={opt}>{opt === 'all' ? 'All Statuses' : opt.charAt(0).toUpperCase() + opt.slice(1)}</option>
          {/each}
        </select>
      </div>

      <!-- Search Button -->
      <button class="btn-primary" onclick={handleSearch} style="padding: 7px 14px;">
        Search
      </button>

      {#if hasActiveFilters}
        <button class="btn-secondary" onclick={clearFilters} style="padding: 7px 14px;">
          <X size={14} />
          Clear
        </button>
      {/if}

      <div style="flex: 1;"></div>

      <!-- Auto Refresh -->
      <div class="flex items-center gap-2">
        <button
          class="auto-refresh-btn"
          class:active={autoRefresh}
          onclick={toggleAutoRefresh}
          title={autoRefresh ? 'Disable auto-refresh' : 'Enable auto-refresh (5s)'}
        >
          <RefreshCw size={14} class={autoRefresh ? 'spin-icon' : ''} stroke-width={2} />
          <span>{autoRefresh ? 'Live' : 'Auto-refresh'}</span>
        </button>
        {#if lastRefresh}
          <span style="font-size: 11px; color: var(--color-fg-3);">Updated {lastRefresh}</span>
        {/if}
      </div>
    </div>
  </div>

  <!-- Logs Table -->
  <div class="card" style="padding: 0; overflow: hidden;">
    {#if loading}
      <Spinner />
    {:else if logs.length === 0}
      <EmptyState
        icon={ScrollText}
        title={hasActiveFilters ? 'No matching logs' : 'No request logs'}
        description={hasActiveFilters ? 'Try adjusting your search or filters.' : 'Request logs will appear here as traffic flows through the gateway.'}
      />
    {:else}
      <div style="overflow-x: auto;">
        <table class="logs-table">
          <thead>
            <tr>
              <th style="width: 160px;">
                <div class="flex items-center gap-1.5">
                  <Clock size={12} />
                  Timestamp
                </div>
              </th>
              <th style="width: 180px;">Model</th>
              <th style="width: 100px;">Status</th>
              <th style="width: 90px;">
                <div class="flex items-center gap-1.5">
                  <ArrowDownToLine size={12} />
                  Tokens In
                </div>
              </th>
              <th style="width: 90px;">
                <div class="flex items-center gap-1.5">
                  <ArrowUpFromLine size={12} />
                  Tokens Out
                </div>
              </th>
              <th style="width: 90px;">
                <div class="flex items-center gap-1.5">
                  <Timer size={12} />
                  Latency
                </div>
              </th>
              <th style="width: 80px;">
                <div class="flex items-center gap-1.5">
                  <Database size={12} />
                  Cached
                </div>
              </th>
            </tr>
          </thead>
          <tbody>
            {#each logs as log (log.id)}
              <tr>
                <td>
                  <span class="font-mono" style="font-size: 11px; color: var(--color-fg-2);">
                    {log.created_at ? formatTimestamp(log.created_at) : '—'}
                  </span>
                </td>
                <td>
                  <span class="font-mono" style="font-size: 12px; font-weight: 500; color: var(--color-fg-0);">
                    {log.model}
                  </span>
                  {#if log.provider}
                    <span style="font-size: 10px; color: var(--color-fg-3); display: block;">{log.provider}</span>
                  {/if}
                </td>
                <td>
                  <StatusBadge status={getStatusDisplay(log.status, log.cached === 1) as any} />
                </td>
                <td>
                  <span class="font-mono" style="font-size: 12px; color: var(--color-fg-1);">
                    {(log.input_tokens || 0).toLocaleString()}
                  </span>
                </td>
                <td>
                  <span class="font-mono" style="font-size: 12px; color: var(--color-fg-1);">
                    {(log.output_tokens || 0).toLocaleString()}
                  </span>
                </td>
                <td>
                  <span
                    class="font-mono"
                    style="font-size: 12px; color: {(log.latency_ms || 0) > 5000 ? 'var(--color-error)' : (log.latency_ms || 0) > 2000 ? 'var(--color-warning)' : 'var(--color-fg-1)'};"
                  >
                    {formatLatency(log.latency_ms || 0)}
                  </span>
                </td>
                <td>
                  {#if log.cached}
                    <span class="badge badge-info" style="font-size: 10px; padding: 2px 8px;">
                      <Zap size={10} style="display: inline; vertical-align: -1px;" /> HIT
                    </span>
                  {:else}
                    <span style="font-size: 11px; color: var(--color-fg-3);">&mdash;</span>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <!-- Table Footer -->
      <div
        class="flex items-center justify-between"
        style="padding: 12px 16px; border-top: 1px solid var(--color-border); background: var(--color-bg-body);"
      >
        <span style="font-size: 12px; color: var(--color-fg-3);">
          Showing {logs.length} log{logs.length !== 1 ? 's' : ''}
        </span>
        <button class="btn-secondary" onclick={loadLogs} style="padding: 5px 12px; font-size: 12px;">
          <RefreshCw size={12} style="display: inline; vertical-align: -1px;" />
          Refresh
        </button>
      </div>
    {/if}
  </div>

  {#if error}
    <div
      class="flex items-center gap-2"
      style="
        padding: 12px 16px; border-radius: var(--radius-sm);
        background: var(--color-error-light); color: var(--color-error);
        font-size: 13px; font-weight: 500;
      "
    >
      {error}
      <button style="margin-left: auto; cursor: pointer; color: var(--color-error); background: none; border: none;" onclick={() => error = ''}>&times;</button>
    </div>
  {/if}
</div>

<style>
  .search-wrapper {
    position: relative;
    flex: 1;
    min-width: 200px;
    max-width: 360px;
  }
  .search-input {
    padding-left: 32px !important;
  }
  .auto-refresh-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--color-border);
    background: transparent;
    color: var(--color-fg-2);
    font-size: 12px;
    font-weight: 500;
    cursor: pointer;
    transition: var(--transition);
  }
  .auto-refresh-btn:hover {
    background: var(--color-bg-sidebar-hover);
    border-color: var(--color-fg-3);
  }
  .auto-refresh-btn.active {
    background: var(--color-success-light);
    border-color: var(--color-success);
    color: var(--color-success);
  }
  .auto-refresh-btn :global(.spin-icon) {
    animation: spin 1.5s linear infinite;
  }
  .logs-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  .logs-table th {
    padding: 10px 14px;
    text-align: left;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: var(--color-fg-3);
    background: var(--color-bg-body);
    border-bottom: 1px solid var(--color-border);
    white-space: nowrap;
  }
  .logs-table td {
    padding: 10px 14px;
    border-bottom: 1px solid var(--color-border-light);
    vertical-align: middle;
  }
  .logs-table tbody tr {
    transition: var(--transition);
  }
  .logs-table tbody tr:hover {
    background: var(--color-primary-light);
  }
  .logs-table tbody tr:last-child td {
    border-bottom: none;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
  @media (max-width: 768px) {
    .search-wrapper {
      max-width: 100%;
    }
  }
</style>
