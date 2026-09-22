<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import StatusBadge from '$lib/components/StatusBadge.svelte';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { showToast } from '$lib/toast';
  import { TestTube2, RefreshCw, CheckCircle2, AlertTriangle, TrendingDown, Coins, X, Settings, CloudOff } from 'lucide-svelte';

  type Quota = {
    user_quota?: { used?: number; total?: number; remaining?: number };
    is_quota_exceeded?: boolean;
    reset_time?: string;
    expires_at?: number;
    fetched_at?: string;
  };

  type QoderConnection = {
    connection_id: string;
    name: string;
    priority: number;
    models_count: number;
    quota?: Quota;
    error?: string;
  };

  type QoderQuotaResponse = {
    success: boolean;
    status?: string;
    message?: string;
    data?: QoderConnection[];
    summary?: {
      connections?: number;
      available?: number;
      errored?: number;
      total_remaining?: number;
      total_allocation?: number;
      fetched_at?: string;
    };
  };

  type QoderConfig = {
    success: boolean;
    enabled: boolean;
    template: boolean;
    region: string;
    idle_timeout_seconds: number;
    first_byte_timeout_seconds: number;
    hint?: string;
  };

  type TestResult = {
    status: 'testing' | 'ok' | 'error';
    latency_ms?: number;
    http_status?: number;
    message?: string;
    input_tokens?: number;
    output_tokens?: number;
    retryable?: boolean;
  };

  // State
  let connections = $state<QoderConnection[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let notEnabled = $state<string | null>(null);
  let refreshing = $state(false);
  let testInProgress = $state<string | null>(null);
  let testResults = $state<Record<string, TestResult>>({});
  let expandedRow = $state<string | null>(null);
  let showingConfig = $state(false);
  let lastUpdated = $state<Date | null>(null);

  // Summary mirrors the API's summary object (`/api/qoder/quota` returns
  // connections/available/errored/total_remaining/total_allocation).
  let summary = $state({
    connections: 0,
    available: 0,
    errored: 0,
    total_remaining: 0,
    total_allocation: 0,
  });

  let creditsPercentage = $derived(
    summary.total_allocation > 0
      ? Math.round((summary.total_remaining / summary.total_allocation) * 100)
      : 0,
  );

  let qoderConfig = $state<QoderConfig | null>(null);

  // The provider reports an explicit `not_enabled` status rather than an empty
  // list, so the UI can explain the disabled state instead of implying there are
  // no connections configured.
  async function fetchConnections(): Promise<boolean> {
    try {
      const data = await api.get<QoderQuotaResponse>('/api/qoder/quota');

      if (data.success === false) {
        notEnabled = data.message || 'The Qoder provider is not active.';
        connections = [];
        error = null;
        return false;
      }

      notEnabled = null;
      connections = data.data || [];

      if (data.summary) {
        summary = {
          connections: data.summary.connections ?? 0,
          available: data.summary.available ?? 0,
          errored: data.summary.errored ?? 0,
          total_remaining: data.summary.total_remaining ?? 0,
          total_allocation: data.summary.total_allocation ?? 0,
        };
      }

      lastUpdated = new Date();
      error = null;
      return true;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unknown error';
      return false;
    } finally {
      // Must clear here, not only on the success path: the summary cards and the
      // table are both gated on `loading`, so leaving it true renders a
      // permanent "Loading…" with blank cards even though the data has arrived.
      loading = false;
    }
  }

  async function fetchConfig(): Promise<void> {
    try {
      qoderConfig = await api.get<QoderConfig>('/api/qoder/config');
    } catch {
      // A failed probe must not render a confident "everything is off"; keep
      // null and let the template show the unknown state.
      qoderConfig = null;
    }
  }

  async function runTest(connectionId: string): Promise<void> {
    if (testInProgress === connectionId) return;

    testInProgress = connectionId;
    testResults[connectionId] = { status: 'testing', message: 'Testing…' };

    try {
      const result = await api.post<{
        success: boolean;
        latency_ms?: number;
        http_status?: number;
        message?: string;
        input_tokens?: number;
        output_tokens?: number;
        retryable?: boolean;
      }>('/api/models/test', {
        model_id: 'qmodel_38max',
        connection_id: connectionId,
      });

      testResults[connectionId] = {
        status: result.success ? 'ok' : 'error',
        latency_ms: result.latency_ms || 0,
        http_status: result.http_status || 0,
        message: result.message || (result.success ? 'Model responds' : 'Test failed'),
        input_tokens: result.input_tokens,
        output_tokens: result.output_tokens,
        retryable: result.retryable,
      };

      if (result.success) {
        showToast(`✅ Model test successful (${result.latency_ms || 0}ms)`, 'success');
      } else {
        showToast(`❌ Test failed: ${result.message || 'unknown error'}`, 'error');
      }
    } catch (err) {
      testResults[connectionId] = {
        status: 'error',
        message: `Connection error: ${err instanceof Error ? err.message : 'Unknown error'}`,
      };
      showToast('❌ Test request failed', 'error');
    } finally {
      testInProgress = null;
    }
  }

  async function refreshAll(): Promise<void> {
    refreshing = true;
    try {
      await Promise.all([fetchConnections(), fetchConfig()]);
      showToast('✅ Data refreshed', 'success');
    } catch {
      showToast('❌ Refresh failed', 'error');
    } finally {
      refreshing = false;
    }
  }

  function toggleExpanded(connectionId: string): void {
    expandedRow = expandedRow === connectionId ? null : connectionId;
  }

  function formatCredit(credits: number): string {
    return new Intl.NumberFormat('en-US', { maximumFractionDigits: 0 }).format(credits);
  }

  // StatusBadge maps its own keys; translate the tester's status vocabulary.
  function badgeStatus(status: TestResult['status']): string {
    switch (status) {
      case 'ok': return 'success';
      case 'testing': return 'pending';
      default: return 'error';
    }
  }

  onMount(() => {
    void fetchConnections();
    void fetchConfig();
  });
</script>

<div class="qd-page">
  <!-- Header with Config Toggle -->
  <div class="qd-head">
    <h1 class="qd-title">Qoder Connections</h1>
    <div class="qd-actions">
      <button class="qd-btn qd-btn-ghost" onclick={() => (showingConfig = !showingConfig)}>
        {#if showingConfig}
          <X size={16} /> Hide Config
        {:else}
          <Settings size={16} /> Show Config
        {/if}
      </button>
      <button class="qd-btn qd-btn-primary" onclick={refreshAll} disabled={refreshing || loading}>
        {#if refreshing}
          <span class="qd-spin"><RefreshCw size={16} /></span> Refreshing…
        {:else}
          <RefreshCw size={16} /> Refresh
        {/if}
      </button>
    </div>
  </div>

  <!-- Config Panel -->
  {#if showingConfig}
    <section class="qd-panel">
      <h3 class="qd-panel-title"><Settings size={20} /> Configuration</h3>
      {#if qoderConfig}
        <div class="qd-grid4">
          <div class="qd-tile">
            <div class="qd-muted">Enabled</div>
            <div class="qd-strong">{qoderConfig.enabled ? 'Yes ✓' : 'No ✗'}</div>
          </div>
          <div class="qd-tile">
            <div class="qd-muted">Template Provisioned</div>
            <div class="qd-strong">{qoderConfig.template ? 'Yes ✓' : 'No ✗'}</div>
          </div>
          <div class="qd-tile">
            <div class="qd-muted">Region</div>
            <div class="qd-strong">{qoderConfig.region}</div>
          </div>
          <div class="qd-tile">
            <div class="qd-muted">Idle Timeout</div>
            <div class="qd-strong">{qoderConfig.idle_timeout_seconds}s</div>
          </div>
        </div>
        {#if !qoderConfig.enabled}
          <div class="qd-notice qd-notice-warn">
            <AlertTriangle size={16} />
            <span>
              Provider is not active. Enable the <code>qoder_enabled</code> setting and
              provision the request template.
            </span>
          </div>
        {/if}
        {#if qoderConfig.hint}
          <div class="qd-hint">Hint: {qoderConfig.hint}</div>
        {/if}
      {:else}
        <div class="qd-notice qd-notice-muted">
          Could not read the Qoder configuration. Use Refresh to retry.
        </div>
      {/if}
    </section>
  {/if}

  <!-- Summary Cards -->
  <div class="qd-grid4">
    <div class="qd-card qd-card-blue">
      <div>
        <div class="qd-card-label">Total Connections</div>
        <div class="qd-card-value">{loading ? '–' : summary.connections}</div>
      </div>
      <span class="qd-card-icon"><TrendingDown size={40} /></span>
    </div>

    <div class="qd-card qd-card-green">
      <div>
        <div class="qd-card-label">Available</div>
        <div class="qd-card-value">{loading ? '–' : summary.available}</div>
      </div>
      <span class="qd-card-icon"><CheckCircle2 size={40} /></span>
    </div>

    <div class="qd-card qd-card-orange">
      <div>
        <div class="qd-card-label">Errored</div>
        <div class="qd-card-value">{loading ? '–' : summary.errored}</div>
      </div>
      <span class="qd-card-icon"><AlertTriangle size={40} /></span>
    </div>

    <div class="qd-card qd-card-purple">
      <div>
        <div class="qd-card-label">Credits Remaining</div>
        <div class="qd-card-value">{loading ? '–' : formatCredit(summary.total_remaining)}</div>
        <div class="qd-card-sub">
          {creditsPercentage}% of {formatCredit(summary.total_allocation)} allocated
        </div>
      </div>
      <span class="qd-card-icon"><Coins size={40} /></span>
    </div>
  </div>

  <!-- Main Table -->
  {#if loading}
    <div class="qd-panel">
      <Spinner />
      <p class="qd-loading-text">Loading Qoder connections…</p>
    </div>
  {:else if error}
    <div class="qd-notice qd-notice-error qd-center">
      <p><strong>Error:</strong> {error}</p>
      <button class="qd-btn qd-btn-primary" onclick={refreshAll}>Retry</button>
    </div>
  {:else if notEnabled}
    <div class="qd-notice qd-notice-warn qd-center">
      <CloudOff size={28} />
      <p><strong>Qoder provider is not active</strong></p>
      <p class="qd-sm">{notEnabled}</p>
      <p class="qd-xs">Enable the provider and provision the request template, then Refresh.</p>
    </div>
  {:else if connections.length === 0}
    <EmptyState
      icon={CloudOff}
      title="No Qoder connections found"
      description="Add a connection with the Qoder format on the Connections page."
    />
  {:else}
    <div class="qd-panel qd-panel-flush">
      <div class="qd-table-wrap">
        <table class="qd-table">
          <thead>
            <tr>
              <th>Connection</th>
              <th>Priority</th>
              <th>Models</th>
              <th>Credit Used</th>
              <th>Credit Left</th>
              <th>Status</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each connections as conn (conn.connection_id)}
              <tr class:qd-row-open={expandedRow === conn.connection_id}>
                <td>
                  <button
                    class="qd-expand"
                    onclick={() => toggleExpanded(conn.connection_id)}
                    aria-expanded={expandedRow === conn.connection_id}
                  >
                    {conn.name}
                    {#if expandedRow === conn.connection_id}▼{:else}▶{/if}
                  </button>
                  <div class="qd-id">{conn.connection_id.slice(0, 8)}…</div>
                </td>
                <td>{conn.priority}</td>
                <td>{conn.models_count}</td>

                {#if conn.quota}
                  <td>
                    <span class="qd-strong">{formatCredit(conn.quota.user_quota?.used || 0)}</span>
                    <span class="qd-sep"> / </span>
                    <span>{formatCredit(conn.quota.user_quota?.total || 0)}</span>
                  </td>
                  <td>
                    {#if conn.quota.user_quota?.remaining !== undefined}
                      <span
                        class="qd-strong"
                        class:qd-ok={conn.quota.user_quota.remaining > 0}
                        class:qd-bad={conn.quota.user_quota.remaining <= 0}
                      >{formatCredit(conn.quota.user_quota.remaining)}</span>
                    {:else}
                      –
                    {/if}
                    {#if conn.quota.is_quota_exceeded}
                      <span class="qd-chip">EXCEEDED</span>
                    {/if}
                  </td>
                {:else}
                  <td class="qd-muted">–</td>
                  <td class="qd-muted">–</td>
                {/if}

                <td>
                  {#if conn.error}
                    <StatusBadge status="error" />
                    <div class="qd-note">{conn.error}</div>
                  {:else if testResults[conn.connection_id]}
                    <StatusBadge status={badgeStatus(testResults[conn.connection_id].status)} />
                    {#if testResults[conn.connection_id].message}
                      <div class="qd-note">{testResults[conn.connection_id].message}</div>
                    {/if}
                  {:else}
                    <span class="qd-muted">Not tested</span>
                  {/if}
                </td>

                <td>
                  <button
                    class="qd-btn qd-btn-test"
                    onclick={() => runTest(conn.connection_id)}
                    disabled={testInProgress === conn.connection_id}
                  >
                    {#if testInProgress === conn.connection_id}
                      Testing…
                    {:else}
                      <TestTube2 size={14} /> Test Model
                    {/if}
                  </button>
                </td>
              </tr>

              {#if expandedRow === conn.connection_id && conn.quota}
                <tr class="qd-row-detail">
                  <td colspan="7">
                    <div class="qd-detail">
                      <h4 class="qd-panel-title">Detailed Credit Breakdown</h4>
                      <div class="qd-grid4 qd-sm">
                        <div>
                          <div class="qd-muted">Used</div>
                          <div class="qd-strong">{formatCredit(conn.quota.user_quota?.used || 0)}</div>
                        </div>
                        <div>
                          <div class="qd-muted">Total Allocation</div>
                          <div class="qd-strong">{formatCredit(conn.quota.user_quota?.total || 0)}</div>
                        </div>
                        <div>
                          <div class="qd-muted">Remaining</div>
                          <div class="qd-strong qd-ok">
                            {formatCredit(conn.quota.user_quota?.remaining || 0)}
                          </div>
                        </div>
                        <div>
                          <div class="qd-muted">Reset Time</div>
                          <div class="qd-mono">{conn.quota.reset_time || 'N/A'}</div>
                        </div>
                      </div>
                      {#if conn.quota.expires_at}
                        <div class="qd-note qd-muted">
                          Expires: {new Date(conn.quota.expires_at * 1000).toLocaleString()}
                        </div>
                      {/if}
                      {#if conn.quota.fetched_at}
                        <div class="qd-note qd-muted">
                          Last fetched: {new Date(conn.quota.fetched_at).toLocaleTimeString()}
                        </div>
                      {/if}
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}

  <!-- Footer info -->
  {#if lastUpdated}
    <div class="qd-foot">Last updated: {lastUpdated.toLocaleString()}</div>
  {/if}
</div>

<style>
  /* Every surface/text colour comes from the dashboard's design tokens
     (--color-bg-*, --color-fg-*, --color-border), so the page follows light and
     dark mode like the rest of the dashboard. Raw Tailwind palette utilities
     (bg-white, text-gray-900, border-gray-200) are light-only literals and
     produced white-on-white text in dark mode. */
  .qd-page {
    display: flex;
    flex-direction: column;
    gap: 24px;
  }

  .qd-head {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    align-items: center;
    justify-content: space-between;
  }
  .qd-title {
    font-size: 24px;
    font-weight: 700;
    color: var(--color-fg-0);
    margin: 0;
  }
  .qd-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .qd-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 14px;
    border-radius: 8px;
    border: 1px solid transparent;
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s, border-color 0.15s, opacity 0.15s;
  }
  .qd-btn:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .qd-btn-ghost {
    background: var(--color-bg-body);
    border-color: var(--color-border);
    color: var(--color-fg-1);
  }
  .qd-btn-ghost:hover:not(:disabled) {
    background: var(--color-bg-hover);
  }
  .qd-btn-primary {
    background: var(--color-primary);
    color: #fff;
  }
  .qd-btn-primary:hover:not(:disabled) {
    background: var(--color-primary-hover);
  }
  .qd-btn-test {
    background: var(--color-primary-light);
    color: var(--color-primary);
    border-color: color-mix(in srgb, var(--color-primary) 25%, transparent);
  }
  .qd-btn-test:hover:not(:disabled) {
    background: color-mix(in srgb, var(--color-primary) 18%, transparent);
  }
  .qd-spin {
    animation: qd-spin 1s linear infinite;
  }

  .qd-panel {
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: var(--radius, 12px);
    padding: 20px;
    box-shadow: var(--shadow, 0 1px 2px rgba(0, 0, 0, 0.04));
  }
  .qd-panel-flush {
    padding: 0;
    overflow: hidden;
  }
  .qd-panel-title {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 15px;
    font-weight: 600;
    color: var(--color-fg-0);
    margin: 0 0 14px;
  }

  .qd-grid4 {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 14px;
  }

  .qd-tile {
    background: var(--color-bg-body);
    border: 1px solid var(--color-border-light);
    border-radius: 10px;
    padding: 14px;
  }

  /* Summary cards keep their saturated gradients — white text reads correctly on
     them in both themes. */
  .qd-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    border-radius: 12px;
    padding: 18px;
    color: #fff;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
  }
  .qd-card-blue { background: linear-gradient(90deg, #3b82f6, #2563eb); }
  .qd-card-green { background: linear-gradient(90deg, #22c55e, #16a34a); }
  .qd-card-orange { background: linear-gradient(90deg, #f97316, #ea580c); }
  .qd-card-purple { background: linear-gradient(90deg, #a855f7, #7e22ce); }
  .qd-card-label { font-size: 13px; opacity: 0.9; }
  .qd-card-value { font-size: 30px; font-weight: 700; line-height: 1.15; }
  .qd-card-sub { font-size: 11px; opacity: 0.85; margin-top: 2px; }
  .qd-card-icon { opacity: 0.5; flex-shrink: 0; }

  .qd-notice {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    padding: 20px;
    border-radius: 10px;
    border: 1px solid var(--color-border);
    background: var(--color-bg-card);
    color: var(--color-fg-1);
    font-size: 13px;
  }
  .qd-notice-warn {
    background: var(--color-warning-light);
    border-color: color-mix(in srgb, var(--color-warning) 35%, transparent);
    color: var(--color-fg-1);
  }
  .qd-notice-error {
    background: var(--color-error-light);
    border-color: color-mix(in srgb, var(--color-error) 35%, transparent);
  }
  .qd-notice-muted { background: var(--color-bg-body); }
  .qd-center { text-align: center; }

  .qd-hint {
    margin-top: 12px;
    font-size: 12px;
    color: var(--color-fg-2);
  }
  .qd-loading-text {
    text-align: center;
    font-size: 13px;
    color: var(--color-fg-2);
    margin: 0 0 8px;
  }

  .qd-table-wrap { overflow-x: auto; }
  .qd-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  .qd-table thead th {
    text-align: left;
    padding: 10px 16px;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-fg-2);
    background: var(--color-bg-body);
    border-bottom: 1px solid var(--color-border);
    white-space: nowrap;
  }
  .qd-table tbody td {
    padding: 12px 16px;
    color: var(--color-fg-1);
    border-bottom: 1px solid var(--color-border-light);
    vertical-align: top;
  }
  .qd-table tbody tr:hover td { background: var(--color-bg-hover); }
  .qd-row-open td { background: var(--color-primary-light); }
  .qd-row-detail td { background: var(--color-bg-body); }

  .qd-detail {
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: 10px;
    padding: 14px;
  }

  .qd-expand {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-weight: 600;
    color: var(--color-primary);
    cursor: pointer;
    text-align: left;
  }
  .qd-expand:hover { text-decoration: underline; }

  .qd-id {
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: 11px;
    color: var(--color-fg-3);
    margin-top: 3px;
  }
  .qd-note { font-size: 11px; color: var(--color-fg-2); margin-top: 3px; }
  .qd-mono { font-family: var(--font-mono, ui-monospace, monospace); font-size: 12px; }
  .qd-muted { color: var(--color-fg-2); }
  .qd-strong { font-weight: 600; color: var(--color-fg-0); }
  .qd-ok { color: var(--color-success); }
  .qd-bad { color: var(--color-error); }
  .qd-sep { color: var(--color-fg-3); }
  .qd-sm { font-size: 12px; }
  .qd-xs { font-size: 11px; color: var(--color-fg-2); }

  .qd-chip {
    display: inline-flex;
    align-items: center;
    margin-left: 8px;
    padding: 1px 7px;
    border-radius: 999px;
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.03em;
    background: var(--color-error-light);
    color: var(--color-error);
  }

  .qd-foot {
    text-align: center;
    font-size: 11px;
    color: var(--color-fg-3);
  }

  @keyframes qd-spin {
    to { transform: rotate(360deg); }
  }
</style>
