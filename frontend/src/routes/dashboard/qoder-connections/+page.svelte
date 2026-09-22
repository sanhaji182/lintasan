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
  // `total_connections` is derived from `connections` rather than duplicated.
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

  // Config state
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

<div class="space-y-6">
  <!-- Header with Config Toggle -->
  <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
    <h1 class="text-2xl font-bold text-gray-900">Qoder Connections</h1>
    <div class="flex flex-wrap gap-2">
      <button
        onclick={() => (showingConfig = !showingConfig)}
        class="px-4 py-2 bg-gray-100 hover:bg-gray-200 rounded-lg text-sm font-medium transition-colors flex items-center gap-2"
      >
        {#if showingConfig}
          <X size={16} /> Hide Config
        {:else}
          <Settings size={16} /> Show Config
        {/if}
      </button>
      <button
        onclick={refreshAll}
        disabled={refreshing || loading}
        class="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:bg-blue-300 text-white rounded-lg text-sm font-medium transition-colors flex items-center gap-2"
      >
        {#if refreshing}
          <RefreshCw class="animate-spin" size={16} />
          Refreshing…
        {:else}
          <RefreshCw size={16} /> Refresh
        {/if}
      </button>
    </div>
  </div>

  <!-- Config Panel -->
  {#if showingConfig}
    <div class="bg-white border border-gray-200 rounded-lg p-6 shadow-sm">
      <h3 class="text-lg font-semibold mb-4 flex items-center gap-2">
        <Settings size={20} /> Configuration
      </h3>
      {#if qoderConfig}
        <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
          <div class="bg-gray-50 rounded p-4">
            <div class="text-sm text-gray-600">Enabled</div>
            <div class="font-semibold">{qoderConfig.enabled ? 'Yes ✓' : 'No ✗'}</div>
          </div>
          <div class="bg-gray-50 rounded p-4">
            <div class="text-sm text-gray-600">Template Provisioned</div>
            <div class="font-semibold">{qoderConfig.template ? 'Yes ✓' : 'No ✗'}</div>
          </div>
          <div class="bg-gray-50 rounded p-4">
            <div class="text-sm text-gray-600">Region</div>
            <div class="font-semibold">{qoderConfig.region}</div>
          </div>
          <div class="bg-gray-50 rounded p-4">
            <div class="text-sm text-gray-600">Idle Timeout</div>
            <div class="font-semibold">{qoderConfig.idle_timeout_seconds}s</div>
          </div>
        </div>
        {#if !qoderConfig.enabled}
          <div class="mt-4 p-4 bg-yellow-50 border border-yellow-200 rounded-lg text-sm text-yellow-800">
            <AlertTriangle class="inline-block mr-2" size={16} />
            Provider is not active. Enable the <code>qoder_enabled</code> setting and provision the request template.
          </div>
        {/if}
        {#if qoderConfig.hint}
          <div class="mt-3 text-sm text-gray-600">Hint: {qoderConfig.hint}</div>
        {/if}
      {:else}
        <div class="p-4 bg-gray-50 border border-gray-200 rounded-lg text-sm text-gray-600">
          Could not read the Qoder configuration. Use Refresh to retry.
        </div>
      {/if}
    </div>
  {/if}

  <!-- Summary Cards -->
  <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-4">
    <div class="bg-gradient-to-r from-blue-500 to-blue-600 rounded-lg p-5 text-white shadow-md">
      <div class="flex items-center justify-between">
        <div>
          <div class="text-sm opacity-90">Total Connections</div>
          <div class="text-3xl font-bold mt-2">{loading ? '–' : summary.connections}</div>
        </div>
        <TrendingDown size={40} class="opacity-50" />
      </div>
    </div>

    <div class="bg-gradient-to-r from-green-500 to-green-600 rounded-lg p-5 text-white shadow-md">
      <div class="flex items-center justify-between">
        <div>
          <div class="text-sm opacity-90">Available</div>
          <div class="text-3xl font-bold mt-2">{loading ? '–' : summary.available}</div>
        </div>
        <CheckCircle2 size={40} class="opacity-50" />
      </div>
    </div>

    <div class="bg-gradient-to-r from-orange-500 to-orange-600 rounded-lg p-5 text-white shadow-md">
      <div class="flex items-center justify-between">
        <div>
          <div class="text-sm opacity-90">Errored</div>
          <div class="text-3xl font-bold mt-2">{loading ? '–' : summary.errored}</div>
        </div>
        <AlertTriangle size={40} class="opacity-50" />
      </div>
    </div>

    <div class="bg-gradient-to-r from-purple-500 to-purple-600 rounded-lg p-5 text-white shadow-md">
      <div class="flex items-center justify-between">
        <div>
          <div class="text-sm opacity-90">Credits Remaining</div>
          <div class="text-3xl font-bold mt-2">{loading ? '–' : formatCredit(summary.total_remaining)}</div>
          <div class="text-xs mt-1 opacity-80">
            {creditsPercentage}% of {formatCredit(summary.total_allocation)} allocated
          </div>
        </div>
        <Coins size={40} class="opacity-50" />
      </div>
    </div>
  </div>

  <!-- Main Table -->
  {#if loading}
    <div class="bg-white border border-gray-200 rounded-lg">
      <Spinner />
      <p class="pb-6 text-center text-sm text-gray-500">Loading Qoder connections…</p>
    </div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 rounded-lg p-6 text-center">
      <p class="text-red-800 font-medium">Error: {error}</p>
      <button onclick={refreshAll} class="mt-4 px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700">
        Retry
      </button>
    </div>
  {:else if notEnabled}
    <div class="bg-yellow-50 border border-yellow-200 rounded-lg p-6 text-center">
      <CloudOff class="inline-block mb-2 text-yellow-700" size={28} />
      <p class="text-yellow-900 font-medium">Qoder provider is not active</p>
      <p class="mt-1 text-sm text-yellow-800">{notEnabled}</p>
      <p class="mt-1 text-xs text-yellow-700">
        Enable the provider and provision the request template, then Refresh.
      </p>
    </div>
  {:else if connections.length === 0}
    <EmptyState
      icon={CloudOff}
      title="No Qoder connections found"
      description="Add a connection with the Qoder format on the Connections page."
    />
  {:else}
    <div class="bg-white border border-gray-200 rounded-lg overflow-x-auto shadow-sm">
      <table class="min-w-full divide-y divide-gray-200">
        <thead class="bg-gray-50">
          <tr>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Connection</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Priority</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Models</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Credit Used</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Credit Left</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
          </tr>
        </thead>
        <tbody class="bg-white divide-y divide-gray-200">
          {#each connections as conn (conn.connection_id)}
            <tr class={expandedRow === conn.connection_id ? 'bg-blue-50' : ''}>
              <td class="px-6 py-4">
                <button
                  onclick={() => toggleExpanded(conn.connection_id)}
                  class="font-medium text-blue-600 hover:text-blue-800"
                  aria-expanded={expandedRow === conn.connection_id}
                >
                  {conn.name}
                  {#if expandedRow === conn.connection_id}▼{:else}▶{/if}
                </button>
                <div class="text-xs text-gray-500 mt-1 font-mono">{conn.connection_id.slice(0, 8)}…</div>
              </td>
              <td class="px-6 py-4 text-sm text-gray-700">{conn.priority}</td>
              <td class="px-6 py-4 text-sm text-gray-700">{conn.models_count}</td>

              {#if conn.quota}
                <td class="px-6 py-4 text-sm">
                  <span class="font-medium">{formatCredit(conn.quota.user_quota?.used || 0)}</span>
                  <span class="text-gray-400"> / </span>
                  <span>{formatCredit(conn.quota.user_quota?.total || 0)}</span>
                </td>
                <td class="px-6 py-4 text-sm">
                  {#if conn.quota.user_quota?.remaining !== undefined}
                    <span class="font-semibold {conn.quota.user_quota.remaining <= 0 ? 'text-red-600' : 'text-green-600'}">
                      {formatCredit(conn.quota.user_quota.remaining)}
                    </span>
                  {:else}
                    –
                  {/if}
                  {#if conn.quota.is_quota_exceeded}
                    <span class="ml-2 inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-red-100 text-red-800">
                      EXCEEDED
                    </span>
                  {/if}
                </td>
              {:else}
                <td class="px-6 py-4 text-sm text-gray-500">–</td>
                <td class="px-6 py-4 text-sm text-gray-500">–</td>
              {/if}

              <td class="px-6 py-4">
                {#if conn.error}
                  <StatusBadge status="error" />
                  <div class="text-xs text-gray-500 mt-1">{conn.error}</div>
                {:else if testResults[conn.connection_id]}
                  <StatusBadge status={badgeStatus(testResults[conn.connection_id].status)} />
                  {#if testResults[conn.connection_id].message}
                    <div class="text-xs text-gray-500 mt-1">{testResults[conn.connection_id].message}</div>
                  {/if}
                {:else}
                  <span class="text-gray-400 text-sm">Not tested</span>
                {/if}
              </td>

              <td class="px-6 py-4">
                <button
                  onclick={() => runTest(conn.connection_id)}
                  disabled={testInProgress === conn.connection_id}
                  class="px-3 py-1 bg-indigo-100 text-indigo-700 rounded hover:bg-indigo-200 disabled:opacity-50 transition-colors text-sm font-medium inline-flex items-center gap-1"
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
              <tr class="bg-gray-50">
                <td colspan="7" class="px-6 py-4">
                  <div class="bg-white border border-gray-200 rounded-lg p-4">
                    <h4 class="font-semibold text-sm mb-3">Detailed Credit Breakdown</h4>
                    <div class="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                      <div>
                        <div class="text-gray-600">Used</div>
                        <div class="font-semibold">{formatCredit(conn.quota.user_quota?.used || 0)}</div>
                      </div>
                      <div>
                        <div class="text-gray-600">Total Allocation</div>
                        <div class="font-semibold">{formatCredit(conn.quota.user_quota?.total || 0)}</div>
                      </div>
                      <div>
                        <div class="text-gray-600">Remaining</div>
                        <div class="font-semibold text-green-600">{formatCredit(conn.quota.user_quota?.remaining || 0)}</div>
                      </div>
                      <div>
                        <div class="text-gray-600">Reset Time</div>
                        <div class="font-mono text-xs">{conn.quota.reset_time || 'N/A'}</div>
                      </div>
                    </div>
                    {#if conn.quota.expires_at}
                      <div class="mt-3 text-xs text-gray-500">
                        Expires: {new Date(conn.quota.expires_at * 1000).toLocaleString()}
                      </div>
                    {/if}
                    {#if conn.quota.fetched_at}
                      <div class="mt-1 text-xs text-gray-400">
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
  {/if}

  <!-- Footer info -->
  {#if lastUpdated}
    <div class="mt-4 text-xs text-gray-500 text-center">
      Last updated: {lastUpdated.toLocaleString()}
    </div>
  {/if}
</div>
