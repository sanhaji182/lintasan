<script lang="ts">
  import TabNav from '$lib/components/TabNav.svelte';
  const __tabs = [
    { label: 'Requests', path: '/dashboard/analytics' },
    { label: 'Usage & Quota', path: '/dashboard/usage' },
    { label: 'Savings', path: '/dashboard/savings' },
    { label: 'Logs', path: '/dashboard/logs' },
    { label: 'Metrics', path: '/dashboard/observability' }
  ];
  import { onMount, onDestroy } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import StatusBadge from '$lib/components/StatusBadge.svelte';
  import {
    ScrollText, Search, RefreshCw, Filter, Calendar, ChevronDown, ChevronUp,
    ChevronLeft, ChevronRight, Clock, Zap, ArrowDownToLine, ArrowUpFromLine,
    Database, Timer, X, AlertTriangle, CheckCircle2, Activity, Gauge
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
    error?: string;
    created_at?: string;
  }

  let logs = $state<LogEntry[]>([]);
  let loading = $state(true);
  let error = $state('');
  let searchQuery = $state('');
  let statusFilter = $state('all');
  let providerFilter = $state('');
  let dateRange = $state('24h');
  let pageSize = $state(100);
  let currentPage = $state(0);
  let hasMore = $state(false);
  let autoRefresh = $state(false);
  let refreshInterval = $state<ReturnType<typeof setInterval> | null>(null);
  let lastRefresh = $state('');
  let expandedId = $state<string | null>(null);

  const statusOptions = ['all', 'success', 'error', 'cached'];
  const dateOptions = [
    { value: '1h', label: 'Last hour' },
    { value: '24h', label: 'Last 24 hours' },
    { value: '7d', label: 'Last 7 days' },
    { value: '30d', label: 'Last 30 days' },
    { value: 'all', label: 'All time' }
  ];

  function sinceValue(): string | null {
    if (dateRange === 'all') return null;
    const now = Date.now();
    const offsets: Record<string, number> = { '1h': 3600000, '24h': 86400000, '7d': 604800000, '30d': 2592000000 };
    return new Date(now - (offsets[dateRange] || 0)).toISOString().slice(0, 19);
  }

  async function loadLogs(resetPage = false) {
    if (resetPage) currentPage = 0;
    loading = true;
    error = '';
    try {
      const params = new URLSearchParams({ limit: String(pageSize + 1), offset: String(currentPage * pageSize) });
      const since = sinceValue();
      if (since) params.set('since', since);
      if (statusFilter !== 'all') params.set('status', statusFilter);
      if (providerFilter.trim()) params.set('provider', providerFilter.trim());
      const res = await api.get<{ data: LogEntry[] }>(`/api/logs?${params}`);
      const received = res.data || [];
      hasMore = received.length > pageSize;
      logs = received.slice(0, pageSize);
      lastRefresh = new Date().toLocaleTimeString();
    } catch (e: any) {
      error = e.message || 'Failed to load logs';
    } finally {
      loading = false;
    }
  }

  let displayedLogs = $derived.by(() => {
    if (!searchQuery.trim()) return logs;
    const q = searchQuery.trim().toLowerCase();
    return logs.filter(l =>
      (l.model || '').toLowerCase().includes(q) ||
      (l.provider || '').toLowerCase().includes(q) ||
      (l.connection_id || '').toLowerCase().includes(q) ||
      (l.error || '').toLowerCase().includes(q) ||
      String(l.status).includes(q)
    );
  });

  let summary = $derived.by(() => {
    const success = logs.filter(l => l.status >= 200 && l.status < 300).length;
    const errors = logs.filter(l => l.status >= 400).length;
    const cached = logs.filter(l => !!l.cached).length;
    const latency = logs.length ? Math.round(logs.reduce((sum, l) => sum + (l.latency_ms || 0), 0) / logs.length) : 0;
    return { success, errors, cached, latency };
  });

  onMount(() => loadLogs());
  onDestroy(() => stopAutoRefresh());

  function toggleAutoRefresh() {
    autoRefresh = !autoRefresh;
    autoRefresh ? startAutoRefresh() : stopAutoRefresh();
  }
  function startAutoRefresh() {
    stopAutoRefresh();
    refreshInterval = setInterval(() => loadLogs(), 10000);
  }
  function stopAutoRefresh() {
    if (refreshInterval) { clearInterval(refreshInterval); refreshInterval = null; }
  }
  function applyFilters() { loadLogs(true); }
  function clearFilters() {
    searchQuery = '';
    statusFilter = 'all';
    providerFilter = '';
    dateRange = '24h';
    loadLogs(true);
  }
  function changePage(delta: number) {
    currentPage = Math.max(0, currentPage + delta);
    loadLogs();
  }
  function exportCSV() {
    const headers = ['timestamp','provider','model','status','input_tokens','output_tokens','latency_ms','cached','error'];
    const rows = displayedLogs.map(l => headers.map(k => JSON.stringify(String((l as any)[k === 'timestamp' ? 'created_at' : k] ?? ''))).join(','));
    const blob = new Blob([[headers.join(','), ...rows].join('\n')], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a'); a.href = url; a.download = `lintasan-logs-${new Date().toISOString().slice(0,10)}.csv`; a.click();
    URL.revokeObjectURL(url);
  }
  function formatTimestamp(ts: string): string {
    const normalized = ts.includes('T') ? ts : ts.replace(' ', 'T') + 'Z';
    const date = new Date(normalized);
    return Number.isNaN(date.getTime()) ? ts : date.toLocaleString(undefined, { month:'short', day:'numeric', hour:'2-digit', minute:'2-digit', second:'2-digit' });
  }
  function formatLatency(ms: number): string { return ms >= 1000 ? `${(ms/1000).toFixed(2)}s` : `${ms}ms`; }
  function getStatusDisplay(status: number, cached?: boolean): string {
    if (cached) return 'cached';
    if (status >= 200 && status < 300) return 'success';
    if (status >= 400) return 'error';
    return 'pending';
  }
  let hasActiveFilters = $derived(searchQuery.trim() !== '' || statusFilter !== 'all' || providerFilter.trim() !== '' || dateRange !== '24h');
</script>

<TabNav tabs={__tabs} />

<div class="logs-page">
  <div class="page-heading">
    <div>
      <h2><Activity size={20} /> Request Logs</h2>
      <p>Inspect gateway traffic, errors, tokens, cache behavior, and latency. Showing up to {pageSize} rows per page.</p>
    </div>
    <div class="heading-actions">
      <button class="btn-secondary" onclick={exportCSV} disabled={displayedLogs.length === 0}>Export CSV</button>
      <button class="auto-refresh-btn" class:active={autoRefresh} onclick={toggleAutoRefresh}>
        <RefreshCw size={14} class={autoRefresh ? 'spin-icon' : ''} /> {autoRefresh ? 'Live · 10s' : 'Auto-refresh'}
      </button>
    </div>
  </div>

  <div class="summary-grid">
    <div class="summary-card success"><CheckCircle2 size={17}/><div><strong>{summary.success}</strong><span>Successful</span></div></div>
    <div class="summary-card error"><AlertTriangle size={17}/><div><strong>{summary.errors}</strong><span>Errors</span></div></div>
    <div class="summary-card cache"><Zap size={17}/><div><strong>{summary.cached}</strong><span>Cache hits</span></div></div>
    <div class="summary-card latency"><Gauge size={17}/><div><strong>{formatLatency(summary.latency)}</strong><span>Avg latency</span></div></div>
  </div>

  <div class="card filters-card">
    <div class="filters-row">
      <div class="search-wrapper">
        <Search size={14}/>
        <input class="input-field search-input" placeholder="Search loaded rows: model, provider, error, status..." bind:value={searchQuery} />
      </div>
      <div class="field-with-icon"><Filter size={14}/><select class="input-field" bind:value={statusFilter} onchange={applyFilters}>{#each statusOptions as opt}<option value={opt}>{opt === 'all' ? 'All statuses' : opt[0].toUpperCase()+opt.slice(1)}</option>{/each}</select></div>
      <div class="field-with-icon"><Calendar size={14}/><select class="input-field" bind:value={dateRange} onchange={applyFilters}>{#each dateOptions as opt}<option value={opt.value}>{opt.label}</option>{/each}</select></div>
      <input class="input-field provider-input" placeholder="Filter provider" bind:value={providerFilter} onkeydown={(e) => e.key === 'Enter' && applyFilters()} />
      <button class="btn-primary" onclick={applyFilters}>Apply</button>
      {#if hasActiveFilters}<button class="btn-secondary" onclick={clearFilters}><X size={14}/> Clear</button>{/if}
    </div>
    <div class="filter-meta">
      <span>Page {currentPage + 1} · {logs.length} loaded · {displayedLogs.length} visible</span>
      {#if lastRefresh}<span>Updated {lastRefresh}</span>{/if}
    </div>
  </div>

  <div class="card table-card">
    {#if loading}<div class="loading-wrap"><Spinner /></div>
    {:else if displayedLogs.length === 0}<EmptyState icon={ScrollText} title={hasActiveFilters ? 'No matching logs' : 'No request logs'} description={hasActiveFilters ? 'Adjust the search, time range, or server-side filters.' : 'Request logs appear as traffic flows through the gateway.'}/>
    {:else}
      <div class="table-scroll">
        <table class="logs-table">
          <thead><tr><th>Time</th><th>Provider / Model</th><th>Status</th><th>Tokens</th><th>Latency</th><th>Cache</th><th></th></tr></thead>
          <tbody>
            {#each displayedLogs as log (log.id)}
              <tr class:expanded={expandedId === log.id} onclick={() => expandedId = expandedId === log.id ? null : log.id}>
                <td><span class="mono muted">{log.created_at ? formatTimestamp(log.created_at) : '—'}</span></td>
                <td><strong class="model">{log.model || 'Unknown model'}</strong><span class="provider">{log.provider || 'Unknown provider'}</span></td>
                <td><StatusBadge status={getStatusDisplay(log.status, log.cached === 1) as any}/><span class="http-code">HTTP {log.status}</span></td>
                <td><span class="mono">{(log.input_tokens||0).toLocaleString()} in</span><span class="token-out">{(log.output_tokens||0).toLocaleString()} out</span></td>
                <td><span class="mono" class:slow={(log.latency_ms||0)>5000} class:medium={(log.latency_ms||0)>2000 && (log.latency_ms||0)<=5000}>{formatLatency(log.latency_ms||0)}</span></td>
                <td>{#if log.cached}<span class="cache-hit"><Zap size={10}/> HIT</span>{:else}<span class="muted">—</span>{/if}</td>
                <td>{#if expandedId === log.id}<ChevronUp size={15}/>{:else}<ChevronDown size={15}/>{/if}</td>
              </tr>
              {#if expandedId === log.id}
                <tr class="detail-row"><td colspan="7"><div class="detail-grid">
                  <div><label>Request ID</label><code>{log.id}</code></div>
                  <div><label>Connection ID</label><code>{log.connection_id || '—'}</code></div>
                  <div><label>Timestamp (raw)</label><code>{log.created_at || '—'}</code></div>
                  <div><label>HTTP Status</label><code>{log.status}</code></div>
                  <div><label>Total Tokens</label><code>{((log.input_tokens||0)+(log.output_tokens||0)).toLocaleString()}</code></div>
                  <div><label>Latency</label><code>{log.latency_ms||0} ms</code></div>
                  {#if log.error}<div class="error-detail"><label>Error</label><code>{log.error}</code></div>{/if}
                </div></td></tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
      <div class="table-footer">
        <div><select class="input-field size-select" bind:value={pageSize} onchange={() => loadLogs(true)}><option value={50}>50 / page</option><option value={100}>100 / page</option><option value={250}>250 / page</option><option value={500}>500 / page</option></select></div>
        <div class="pagination"><button class="btn-secondary" onclick={() => changePage(-1)} disabled={currentPage === 0}><ChevronLeft size={14}/> Previous</button><span>Page {currentPage + 1}</span><button class="btn-secondary" onclick={() => changePage(1)} disabled={!hasMore}>Next <ChevronRight size={14}/></button></div>
        <button class="btn-secondary" onclick={() => loadLogs()}><RefreshCw size={12}/> Refresh</button>
      </div>
    {/if}
  </div>

  {#if error}<div class="error-banner"><AlertTriangle size={15}/>{error}<button onclick={() => error=''}>&times;</button></div>{/if}
</div>

<style>
  .logs-page{display:flex;flex-direction:column;gap:18px;animation:fadeInUp .35s ease-out}.page-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.page-heading h2{font-size:18px;font-weight:650;color:var(--color-fg-0);display:flex;align-items:center;gap:8px}.page-heading h2 :global(svg){color:var(--color-primary)}.page-heading p{font-size:12px;color:var(--color-fg-2);margin-top:4px}.heading-actions{display:flex;gap:8px}.summary-grid{display:grid;grid-template-columns:repeat(4,minmax(140px,1fr));gap:12px}.summary-card{display:flex;align-items:center;gap:11px;padding:14px 16px;background:var(--color-bg-card);border:1px solid var(--color-border);border-radius:10px}.summary-card strong{display:block;font:700 18px var(--font-mono);color:var(--color-fg-0)}.summary-card span{display:block;font-size:11px;color:var(--color-fg-3)}.summary-card.success :global(svg){color:var(--color-success)}.summary-card.error :global(svg){color:var(--color-error)}.summary-card.cache :global(svg){color:var(--color-warning)}.summary-card.latency :global(svg){color:var(--color-primary)}.filters-card{padding:14px 16px}.filters-row{display:flex;align-items:center;gap:9px;flex-wrap:wrap}.search-wrapper{position:relative;flex:1;min-width:260px}.search-wrapper>:global(svg){position:absolute;left:10px;top:50%;transform:translateY(-50%);color:var(--color-fg-3);pointer-events:none}.search-input{padding-left:32px!important}.field-with-icon{position:relative}.field-with-icon>:global(svg){position:absolute;left:9px;top:50%;transform:translateY(-50%);color:var(--color-fg-3);pointer-events:none;z-index:1}.field-with-icon select{padding-left:30px!important;width:150px}.provider-input{width:145px}.filter-meta{display:flex;justify-content:space-between;margin-top:10px;padding-top:10px;border-top:1px solid var(--color-border-light);font-size:11px;color:var(--color-fg-3)}.auto-refresh-btn{display:flex;align-items:center;gap:6px;padding:7px 12px;border-radius:var(--radius-sm);border:1px solid var(--color-border);background:transparent;color:var(--color-fg-2);font-size:12px;font-weight:500;cursor:pointer}.auto-refresh-btn.active{background:var(--color-success-light);border-color:var(--color-success);color:var(--color-success)}.auto-refresh-btn :global(.spin-icon){animation:spin 1.5s linear infinite}.table-card{padding:0;overflow:hidden}.loading-wrap{padding:60px}.table-scroll{overflow:auto;max-height:70vh}.logs-table{width:100%;border-collapse:separate;border-spacing:0;font-size:13px}.logs-table th{position:sticky;top:0;z-index:2;padding:10px 14px;text-align:left;font-size:10px;font-weight:650;text-transform:uppercase;letter-spacing:.55px;color:var(--color-fg-3);background:var(--color-bg-body);border-bottom:1px solid var(--color-border);white-space:nowrap}.logs-table td{padding:10px 14px;border-bottom:1px solid var(--color-border-light);vertical-align:middle}.logs-table tbody tr:not(.detail-row){cursor:pointer;transition:background .12s}.logs-table tbody tr:not(.detail-row):hover,.logs-table tbody tr.expanded{background:var(--color-primary-light)}.mono{font-family:var(--font-mono);font-size:11px}.muted{color:var(--color-fg-3)}.model{display:block;font-family:var(--font-mono);font-size:12px;color:var(--color-fg-0);max-width:300px;overflow:hidden;text-overflow:ellipsis}.provider,.token-out,.http-code{display:block;font-size:10px;color:var(--color-fg-3);margin-top:2px}.slow{color:var(--color-error)}.medium{color:var(--color-warning)}.cache-hit{display:inline-flex;align-items:center;gap:3px;font-size:10px;font-weight:650;padding:2px 7px;border-radius:10px;color:var(--color-info);background:var(--color-info-light)}.detail-row td{padding:0!important;background:var(--color-bg-body)}.detail-grid{display:grid;grid-template-columns:repeat(3,minmax(180px,1fr));gap:12px;padding:14px 18px;border-bottom:1px solid var(--color-border)}.detail-grid label{display:block;font-size:10px;text-transform:uppercase;letter-spacing:.45px;color:var(--color-fg-3);margin-bottom:4px}.detail-grid code{display:block;font:11px var(--font-mono);color:var(--color-fg-1);word-break:break-all}.error-detail{grid-column:1/-1}.error-detail code{color:var(--color-error);background:var(--color-error-light);padding:8px;border-radius:6px}.table-footer{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:11px 14px;background:var(--color-bg-body);border-top:1px solid var(--color-border)}.pagination{display:flex;align-items:center;gap:10px;font-size:11px;color:var(--color-fg-2)}.pagination button,.table-footer button{display:inline-flex;align-items:center;gap:5px}.size-select{width:110px;padding:5px 8px;font-size:11px}.error-banner{display:flex;align-items:center;gap:8px;padding:10px 14px;border-radius:8px;color:var(--color-error);background:var(--color-error-light);font-size:12px}.error-banner button{margin-left:auto;border:0;background:none;color:inherit;cursor:pointer}@keyframes spin{to{transform:rotate(360deg)}}@media(max-width:900px){.summary-grid{grid-template-columns:repeat(2,1fr)}.page-heading{flex-direction:column}.detail-grid{grid-template-columns:1fr}.table-footer{flex-wrap:wrap}}@media(max-width:560px){.summary-grid{grid-template-columns:1fr}.search-wrapper,.provider-input{width:100%;min-width:100%}.filter-meta{flex-direction:column;gap:4px}}
</style>
