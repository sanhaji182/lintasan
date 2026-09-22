<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import StatusBadge from '$lib/components/StatusBadge.svelte';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { showToast } from '$lib/toast';
  import { TestTube2, RefreshCw, CheckCircle2, AlertTriangle, TrendingDown, Coins, X, Settings, CloudOff, Plus, ChevronRight } from 'lucide-svelte';

  type QuotaBucket = { used?: number; total?: number; remaining?: number; unit?: string; percentage?: number };

  type Quota = {
    user_quota?: QuotaBucket;
    /** Add-on / bonus credits from promotions and rewards. Awarded separately from
     *  the plan allocation (e.g. the "Claim 100 Credits" reward lands here), and it
     *  expires 30 days from claim rather than with the plan. */
    addon_quota?: QuotaBucket;
    is_quota_exceeded?: boolean;
    reset_time?: string;
    expires_at?: number;
    fetched_at?: string;
    /** Upstream's own used/total ratio (0..1), carried for cross-checking. */
    total_usage_percentage?: number;
    /** Metered unit (observed: "credits"). */
    usage_type?: string;
    /** Entitlement class, e.g. "personal_professional_trial". */
    account_type?: string;
    /** Where upstream sends a user to raise quota. */
    upgrade_url?: string;
    /** Kept verbatim: per-provider bonus quota would appear here. Empty in every
     *  response observed so far. Rendered raw on purpose — the shape is unverified. */
    outer_providers?: unknown;
  };

  /** A bonus bucket is worth showing only once it carries something. */
  function hasBonus(conn: QoderConnection): boolean {
    const a = conn.quota?.addon_quota;
    if (!a) return false;
    return (a.remaining ?? 0) > 0 || (a.total ?? 0) > 0;
  }

  type ModelCostRow = {
    model_id: string;
    display_name: string;
    /** Multiplier in force right now. */
    factor: number;
    standard_factor: number;
    off_peak_factor: number;
    /** "vendor_table" | "discovered" | "unknown" — where the number came from. */
    factor_source: string;
    off_peak_active: boolean;
    /** Units of this model the connection's balance still buys at `factor`. */
    effective_credits?: number;
    /** Context limits. Absent means upstream did not state one — not zero. */
    max_input_tokens?: number;
    max_output_tokens?: number;
    promo_note?: string;
    promo_until?: string;
    /** True only for a live 0.0x promotion. */
    free_now?: boolean;
  };

  type ModelCostResponse = {
    success: boolean;
    window: {
      off_peak_now: boolean;
      off_peak_utc: string;
      off_peak_local: string;
      next_change: string;
      next_change_local: string;
      server_time_local: string;
    };
    note: string;
    data: { connection_id: string; name: string; credits_left: number; models: ModelCostRow[] }[];
  };

  type BulkAddRow = {
    credential: string;
    status: 'added' | 'duplicate' | 'invalid' | 'error';
    connection_id?: string;
    name?: string;
    account?: string;
    message?: string;
  };

  type BulkAddResponse = {
    success: boolean;
    validated: boolean;
    summary: {
      submitted: number;
      added: number;
      duplicates: number;
      invalid: number;
      failed: number;
      next_step?: string;
    };
    data: BulkAddRow[];
  };

  /** True when any account in the list carries bonus credits. */
  function anyBonus(): boolean {
    return connections.some(hasBonus);
  }

  /** Sum of bonus remaining across the pool. */
  function totalBonusRemaining(): number {
    return connections.reduce((sum, c) => sum + (c.quota?.addon_quota?.remaining ?? 0), 0);
  }

  /** Largest input context across the priced models, for the explainer note. Models
   *  that did not state a limit are skipped rather than counted as zero. */
  function widestContext(models: ModelCostRow[]): ModelCostRow | null {
    let best: ModelCostRow | null = null;
    for (const m of models) {
      if (!m.max_input_tokens) continue;
      if (!best || m.max_input_tokens > (best.max_input_tokens || 0)) best = m;
    }
    return best;
  }

  /**
   * Qoder sends expiresAt as epoch MILLISECONDS (e.g. 1790546012024). The previous
   * code multiplied by 1000, which rendered a date ~56,000 years out. Detect the
   * unit instead of assuming it.
   */
  function formatEpoch(v?: number): string {
    if (!v) return 'N/A';
    // Anything past ~year 33658 in seconds is really milliseconds.
    const ms = v > 1e11 ? v : v * 1000;
    return new Date(ms).toLocaleString();
  }

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
  /** Per-model Credit cost. null means "not fetched / unavailable", never "no models". */
  let modelCost = $state<ModelCostResponse | null>(null);
  let notEnabled = $state<string | null>(null);
  let refreshing = $state(false);
  let testInProgress = $state<string | null>(null);
  let testResults = $state<Record<string, TestResult>>({});
  let expandedRow = $state<string | null>(null);
  let showingConfig = $state(false);

  // Bulk-add state. Kept separate from `connections` because the panel works before
  // any connection exists.
  let showingAddPATs = $state(false);
  let bulkPats = $state('');
  let bulkValidating = $state(true);
  let bulkBusy = $state(false);
  let bulkResult = $state<BulkAddResponse | null>(null);

  // The cost table is reference material, not something an operator needs on screen
  // while working the connection list below it — so it collapses, and the choice is
  // remembered. Default collapsed: a returning visitor has already read the factors.
  const COST_PANEL_KEY = 'lintasan_qoder_cost_panel_collapsed';
  let costCollapsed = $state(true);

  function readCostPanelPref(): void {
    try {
      // Absent means first visit → stay collapsed (the default).
      costCollapsed = localStorage.getItem(COST_PANEL_KEY) !== '0';
    } catch {
      // Private mode / storage disabled: keep the in-memory default rather than
      // throwing on a cosmetic preference.
    }
  }

  function toggleCostPanel(): void {
    costCollapsed = !costCollapsed;
    try {
      localStorage.setItem(COST_PANEL_KEY, costCollapsed ? '1' : '0');
    } catch {
      // Non-fatal: the toggle still works for this session.
    }
  }
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

  /** Per-model Credits cost. The off-peak window makes a model 2.5x cheaper at some
   *  hours, so this is refreshed with the page rather than cached in the client. */
  async function fetchModelCost(): Promise<void> {
    try {
      const res = await api.get<ModelCostResponse>('/api/qoder/models');
      modelCost = res;
    } catch {
      // Leave null so the panel states it is unavailable instead of showing no models.
      modelCost = null;
    }
  }

  /** Every distinct model across the pool, with the best (lowest) factor in force.
   *  Deduplicated because accounts carry different entitlements and the same model
   *  would otherwise appear once per account. */
  function distinctModelCosts(): ModelCostRow[] {
    if (!modelCost) return [];
    const best = new Map<string, ModelCostRow>();
    for (const conn of modelCost.data || []) {
      for (const m of conn.models || []) {
        const prev = best.get(m.model_id);
        if (!prev || (m.factor > 0 && (prev.factor <= 0 || m.factor < prev.factor))) {
          best.set(m.model_id, m);
        }
      }
    }
    return [...best.values()].sort((a, b) => b.factor - a.factor);
  }

  function formatFactor(f: number): string {
    if (!f || f <= 0) return 'not reported';
    return `${f}x`;
  }

  /** 0.5x means one credit buys two units. That is the number an operator wants. */
  function effectiveUnits(credits: number, factor: number): string {
    if (!factor || factor <= 0) return '–';
    return formatCredit(Math.round(credits / factor));
  }

  /**
   * The models worth showing worked examples for.
   *
   * Picks the cheapest and the dearest model that actually have a factor, plus any
   * model with an off-peak discount — because the cheap/dear pair is what makes a
   * factor legible, and the discounted ones are where the timing decision lives.
   * Bounded to 4 so the note stays a note.
   */
  function explainerModels(models: ModelCostRow[]): ModelCostRow[] {
    const priced = models.filter((m) => m.factor > 0);
    if (priced.length === 0) return [];
    const cheapest = priced[priced.length - 1];
    const dearest = priced[0];
    const discounted = priced.filter(
      (m) => m.off_peak_factor > 0 && m.off_peak_factor < m.standard_factor,
    );
    const seen = new Set<string>();
    const out: ModelCostRow[] = [];
    for (const m of [discounted[0], cheapest, dearest, ...discounted.slice(1)]) {
      if (!m || seen.has(m.model_id)) continue;
      seen.add(m.model_id);
      out.push(m);
      if (out.length === 4) break;
    }
    return out;
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
      await Promise.all([fetchConnections(), fetchConfig(), fetchModelCost()]);
      showToast('✅ Data refreshed', 'success');
    } catch {
      showToast('❌ Refresh failed', 'error');
    } finally {
      refreshing = false;
    }
  }

  /** How many credentials the paste field currently holds. Mirrors the server's
   *  splitter so the button label and the submitted count cannot disagree. */
  function pastedCount(): number {
    const seen = new Set<string>();
    for (const t of bulkPats.split(/[\s,;"']+/)) {
      const v = t.trim();
      if (v) seen.add(v);
    }
    return seen.size;
  }

  async function submitBulkPats(): Promise<void> {
    if (bulkBusy || pastedCount() === 0) return;
    bulkBusy = true;
    bulkResult = null;
    try {
      const res = await api.post<BulkAddResponse>('/api/qoder/credentials', {
        pats: bulkPats,
        validate: bulkValidating,
      });
      bulkResult = res;
      const s = res.summary;
      if (s.added > 0) {
        // Optimistic copy, then refetch so the table reflects the server, not the
        // response — the two can differ if another operator added rows meanwhile.
        showToast(`✅ Added ${s.added} of ${s.submitted}`, 'success');
        await Promise.all([fetchConnections(), fetchModelCost()]);
        bulkPats = '';
      } else if (s.duplicates === s.submitted) {
        showToast('All credentials were already present', 'info');
      } else {
        showToast(`⚠️ Nothing added (${s.invalid} invalid, ${s.failed} failed)`, 'error');
      }
    } catch (err) {
      showToast(
        `❌ Bulk add failed: ${err instanceof Error ? err.message : 'unknown error'}`,
        'error',
      );
    } finally {
      bulkBusy = false;
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
    void fetchModelCost();
    readCostPanelPref();
  });
</script>

<div class="qd-page">
  <!-- Header with Config Toggle -->
  <div class="qd-head">
    <h1 class="qd-title">Qoder Connections</h1>
    <div class="qd-actions">
      <button class="qd-btn qd-btn-ghost" onclick={() => (showingAddPATs = !showingAddPATs)}>
        <Plus size={16} /> Add PATs
      </button>
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

  <!-- Bulk-add panel. Placed above the table because it is the first thing an
       operator needs when the pool is empty or needs topping up. -->
  {#if showingAddPATs}
    <section class="qd-panel">
      <h3 class="qd-panel-title"><Plus size={20} /> Add Qoder PATs</h3>
      <p class="qd-muted qd-sm">
        Paste Personal Access Tokens — one per line, or separated by commas, semicolons
        or spaces. Everything else about the connection (base URL, paths, auth header,
        format, priority) is derived, so a batch is identical to a single add.
      </p>

      <textarea
        class="qd-textarea"
        rows="6"
        placeholder={"pt-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\npt-yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy"}
        bind:value={bulkPats}
        disabled={bulkBusy}
      ></textarea>

      <div class="qd-addrow">
        <label class="qd-check">
          <input type="checkbox" bind:checked={bulkValidating} disabled={bulkBusy} />
          Validate each credential before adding
        </label>
        <span class="qd-muted qd-xs">
          {pastedCount()} credential{pastedCount() === 1 ? '' : 's'} pasted
        </span>
        <div class="qd-actions">
          <button
            class="qd-btn qd-btn-primary"
            onclick={submitBulkPats}
            disabled={bulkBusy || pastedCount() === 0}
          >
            {#if bulkBusy}
              <span class="qd-spin"><RefreshCw size={16} /></span> Adding…
            {:else}
              <Plus size={16} /> Add {pastedCount() || ''} PATs
            {/if}
          </button>
          <button
            class="qd-btn qd-btn-ghost"
            onclick={() => { bulkPats = ''; bulkResult = null; }}
            disabled={bulkBusy}
          >Clear</button>
        </div>
      </div>

      {#if bulkValidating}
        <div class="qd-note qd-muted">
          Validation runs the real session exchange, so a credential that passes here
          genuinely works — it is not a format check.
        </div>
      {:else}
        <div class="qd-notice qd-notice-warn">
          <AlertTriangle size={16} />
          <span>
            Validation is off. A credential that cannot authenticate will be added and
            will fail on every request instead of being reported now.
          </span>
        </div>
      {/if}

      {#if bulkResult}
        <div class="qd-added">
          <div class="qd-strong">
            {bulkResult.summary.added} added ·
            {bulkResult.summary.duplicates} duplicate ·
            {bulkResult.summary.invalid} invalid ·
            {bulkResult.summary.failed} failed
            <span class="qd-muted">of {bulkResult.summary.submitted} submitted</span>
          </div>
          <div class="qd-table-wrap">
            <table class="qd-table qd-sm">
              <thead>
                <tr><th>CREDENTIAL</th><th>STATUS</th><th>NAME</th><th>DETAIL</th></tr>
              </thead>
              <tbody>
                {#each bulkResult.data as r (r.credential)}
                  <tr>
                    <td class="qd-mono">{r.credential}</td>
                    <td>
                      {#if r.status === 'added'}
                        <StatusBadge status="success" />
                      {:else if r.status === 'duplicate'}
                        <StatusBadge status="pending" />
                      {:else}
                        <StatusBadge status="error" />
                      {/if}
                      <span class="qd-muted qd-xs">{r.status}</span>
                    </td>
                    <td class="qd-mono">{r.name || '–'}</td>
                    <td class="qd-muted">{r.message || '–'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
          {#if bulkResult.summary.added > 0}
            <div class="qd-note qd-muted">
              {bulkResult.summary.next_step}
            </div>
          {/if}
        </div>
      {/if}
    </section>
  {/if}

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

    {#if anyBonus()}
      <div class="qd-card qd-card-bonus">
        <div>
          <div class="qd-card-label">Bonus Credits</div>
          <div class="qd-card-value">{loading ? '–' : formatCredit(totalBonusRemaining())}</div>
          <div class="qd-card-sub">
            from promotions · separate from plan allocation
          </div>
        </div>
        <span class="qd-card-icon"><Coins size={40} /></span>
      </div>
    {/if}
  </div>

  <!-- Per-model Credit cost. Qoder prices Credits per model, and the multiplier
       changes with the time of day, so the window state matters as much as the
       factor itself. -->
  {#if modelCost}
    {@const models = distinctModelCosts()}
    {@const w = modelCost.window}
    <div class="qd-panel">
      <!-- The header is the toggle and stays visible when collapsed, so the window
           state and cheapest-model hint are readable without expanding. -->
      <button
        class="qd-panel-toggle qd-panel-head"
        onclick={toggleCostPanel}
        aria-expanded={!costCollapsed}
        aria-controls="qoder-cost-body"
      >
        <span class="qd-toggle-lead">
          <span class="qd-chevron" class:qd-chevron-open={!costCollapsed}>
            <ChevronRight size={16} />
          </span>
          <h2 class="qd-panel-title">Credit Cost per Model</h2>
        </span>
        <span class="qd-toggle-trail">
          {#if costCollapsed}
            <span class="qd-muted qd-xs">{models.length} models · {summary.total_remaining} credits left</span>
          {/if}
          <span class="qd-chip" class:qd-chip-off={w?.off_peak_now} class:qd-chip-on={!w?.off_peak_now}>
            {w?.off_peak_now ? 'OFF-PEAK NOW' : 'REGULAR HOURS'}
          </span>
        </span>
      </button>

      <div id="qoder-cost-body" class="qd-collapsible" class:qd-collapsed={costCollapsed}>
      <div class="qd-window">
        Off-peak <span class="qd-mono">{w?.off_peak_local || w?.off_peak_utc}</span>
        {#if w?.off_peak_now}
          · ends {w?.next_change_local}
        {:else}
          · starts {w?.next_change_local}
        {/if}
        · <span class="qd-muted">Qoder defines the window in UTC ({w?.off_peak_utc})</span>
      </div>

      {#if models.length === 0}
        <p class="qd-muted">
          No models discovered for these connections yet. Run a model sync, then refresh.
        </p>
      {:else}
        <div class="qd-table-wrap">
          <table class="qd-table">
            <thead>
              <tr>
                <th>MODEL</th>
                <th>FACTOR NOW</th>
                <th>REGULAR</th>
                <th>OFF-PEAK</th>
                <th>INPUT LIMIT</th>
                <th>FACTOR SOURCE</th>
              </tr>
            </thead>
            <tbody>
              {#each models as m (m.model_id)}
                <tr>
                  <td>
                    <span class="qd-strong">{m.display_name}</span>
                    <div class="qd-id">{m.model_id}</div>
                  </td>
                  <td>
                    {#if m.free_now}
                      <span class="qd-chip qd-chip-bonus">FREE NOW</span>
                    {:else if m.factor > 0}
                      <span class="qd-strong" class:qd-ok={m.off_peak_active}>{formatFactor(m.factor)}</span>
                    {:else}
                      <span class="qd-muted">not reported</span>
                    {/if}
                  </td>
                  <td>{formatFactor(m.standard_factor)}</td>
                  <td>
                    {#if m.off_peak_factor > 0 && m.off_peak_factor < m.standard_factor}
                      <span class="qd-ok">{formatFactor(m.off_peak_factor)}</span>
                    {:else}
                      <span class="qd-muted">{formatFactor(m.off_peak_factor)}</span>
                    {/if}
                  </td>
                  <td>
                    {#if m.max_input_tokens}
                      <span class="qd-strong" title="Maximum input context">{formatCredit(m.max_input_tokens)}</span>
                      {#if m.max_output_tokens}
                        <span class="qd-muted qd-xs">/ {formatCredit(m.max_output_tokens)} out</span>
                      {/if}
                    {:else}
                      <span class="qd-muted" title="Upstream did not state a limit for this model">not reported</span>
                    {/if}
                  </td>
                  <td class="qd-muted">{m.factor_source}</td>
                </tr>
                {#if m.promo_note}
                  <tr class="qd-row-detail">
                    <td colspan="6">
                      <div class="qd-note qd-muted">
                        Promotion: {m.promo_note}
                        {#if m.promo_until}· ends {m.promo_until}{/if}
                      </div>
                    </td>
                  </tr>
                {/if}
              {/each}
            </tbody>
          </table>
        </div>

        <div class="qd-note qd-muted">
          A factor of 0.5x means one Credit buys two units of that model; 2x means one
          Credit buys half a unit. With {formatCredit(summary.total_remaining)} Credits
          left in the pool, at the multiplier in force now:
          <ul class="qd-examples">
            {#each explainerModels(models) as m (m.model_id)}
              <li>
                <span class="qd-strong">{m.display_name}</span> at {formatFactor(m.factor)}
                → <span class="qd-strong">{effectiveUnits(summary.total_remaining, m.factor)}</span> units
                {#if m.off_peak_factor > 0 && m.off_peak_factor < m.standard_factor}
                  <span class="qd-muted">
                    ({effectiveUnits(summary.total_remaining, m.off_peak_factor)} if run off-peak)
                  </span>
                {/if}
              </li>
            {/each}
          </ul>
          {#if widestContext(models)}
            {@const wc = widestContext(models)}
            <div>
              Widest input context:
              <span class="qd-strong">{formatCredit(wc?.max_input_tokens || 0)}</span> tokens
              on <span class="qd-strong">{wc?.display_name}</span>.
            </div>
          {/if}
        </div>
      {/if}
      </div>
    </div>
  {/if}

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
                    {#if hasBonus(conn)}
                      <span class="qd-chip qd-chip-bonus" title="Add-on / bonus credits awarded by promotions">
                        +{formatCredit(conn.quota.addon_quota?.remaining || 0)} BONUS
                      </span>
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

                      {#if hasBonus(conn)}
                        <div class="qd-grid4 qd-sm qd-bonus-row">
                          <div>
                            <div class="qd-muted">Bonus — Used</div>
                            <div class="qd-strong">{formatCredit(conn.quota.addon_quota?.used || 0)}</div>
                          </div>
                          <div>
                            <div class="qd-muted">Bonus — Total</div>
                            <div class="qd-strong">{formatCredit(conn.quota.addon_quota?.total || 0)}</div>
                          </div>
                          <div>
                            <div class="qd-muted">Bonus — Remaining</div>
                            <div class="qd-strong qd-ok">{formatCredit(conn.quota.addon_quota?.remaining || 0)}</div>
                          </div>
                          <div>
                            <div class="qd-muted">Unit</div>
                            <div class="qd-mono">{conn.quota.addon_quota?.unit || conn.quota.usage_type || 'credits'}</div>
                          </div>
                        </div>
                        <div class="qd-note qd-muted">
                          Bonus credits come from promotions and rewards, not the plan allocation.
                          Each reward expires 30 days after it is claimed.
                        </div>
                      {:else}
                        <div class="qd-note qd-muted">
                          No add-on / bonus credits on this account yet. Promotion rewards (including
                          any daily claim) are credited here, separately from the plan allocation.
                        </div>
                      {/if}

                      {#if conn.quota.account_type || conn.quota.usage_type || conn.quota.upgrade_url}
                        <div class="qd-note qd-muted">
                          {#if conn.quota.account_type}Entitlement: <span class="qd-mono">{conn.quota.account_type}</span>{/if}
                          {#if conn.quota.usage_type} · metered in <span class="qd-mono">{conn.quota.usage_type}</span>{/if}
                          {#if conn.quota.upgrade_url}
                            · <a href={conn.quota.upgrade_url} target="_blank" rel="noopener">upgrade quota</a>
                          {/if}
                        </div>
                      {/if}

                      {#if Array.isArray(conn.quota.outer_providers) && conn.quota.outer_providers.length > 0}
                        <div class="qd-note qd-muted">
                          Per-provider quota reported by upstream:
                          <span class="qd-mono">{JSON.stringify(conn.quota.outer_providers)}</span>
                        </div>
                      {/if}
                      {#if conn.quota.expires_at}
                        <div class="qd-note qd-muted">
                          Expires: {formatEpoch(conn.quota.expires_at)}
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
  .qd-card-bonus { background: linear-gradient(90deg, #f59e0b, #b45309); }
  .qd-panel-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  /* The panel header doubles as a collapse toggle. Reset the button chrome so it still
     reads as a heading, and keep a focus ring because it is now interactive. */
  .qd-panel-toggle {
    width: 100%;
    background: none;
    border: 0;
    padding: 0;
    cursor: pointer;
    text-align: left;
    color: inherit;
    font: inherit;
  }
  .qd-panel-toggle:hover .qd-panel-title { text-decoration: underline; text-underline-offset: 3px; }
  .qd-panel-toggle:focus-visible { outline: 2px solid var(--color-primary, #3c50e0); outline-offset: 3px; border-radius: 6px; }
  .qd-toggle-lead { display: inline-flex; align-items: center; gap: 6px; }
  .qd-toggle-trail { display: inline-flex; align-items: center; gap: 8px; }
  /* .qd-chevron rotates to indicate state. ChevronRight -> down when open. */
  .qd-chevron { display: inline-flex; transition: transform 0.15s ease; color: var(--color-fg-3); }
  .qd-chevron-open { transform: rotate(90deg); }
  .qd-collapsible { display: block; }
  .qd-collapsed { display: none; }
  @media (prefers-reduced-motion: reduce) { .qd-chevron { transition: none; } }
  .qd-window { font-size: 12px; color: var(--color-fg-2); margin: 6px 0 12px; }
  .qd-textarea {
    width: 100%;
    box-sizing: border-box;
    font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 12px;
    line-height: 1.5;
    padding: 10px 12px;
    border-radius: 8px;
    /* Token-based so the field follows the theme; the paste is a PAT, and a
       light-only textarea in dark mode is where credentials get mistyped. */
    background: var(--color-bg-card);
    color: var(--color-fg-0);
    border: 1px solid var(--color-border);
    resize: vertical;
  }
  .qd-textarea:focus { outline: 2px solid var(--color-primary, #3c50e0); outline-offset: 1px; }
  .qd-addrow { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; margin-top: 10px; }
  .qd-check { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; color: var(--color-fg-1); }
  .qd-added { margin-top: 14px; display: flex; flex-direction: column; gap: 8px; }
  .qd-examples { margin: 6px 0 0; padding-left: 18px; }
  .qd-examples li { margin: 2px 0; }
  .qd-chip-off { background: rgba(34, 197, 94, 0.16); color: #15803d; border: 1px solid rgba(34, 197, 94, 0.45); }
  :global(html[data-theme='dark']) .qd-chip-off { color: #4ade80; }
  /* .qd-chip-bonus and .qd-chip-on are declared after the base .qd-chip rule — see the
     note there. Source order decides at equal specificity. */
  .qd-bonus-row { border-top: 1px dashed var(--color-border); padding-top: 10px; margin-top: 10px; }
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

  /* Chip variants MUST come after the base .qd-chip rule above. Equal specificity
     means source order decides, so an override declared earlier loses — which is
     exactly how "REGULAR HOURS" ended up rendering in the error palette. */
  /* Neutral, not red: a window state is not a fault. The red base style above is for
     the EXCEEDED chip it was designed for. */
  .qd-chip-on {
    background: var(--color-bg-body);
    color: var(--color-fg-2);
    border: 1px solid var(--color-border);
  }
  .qd-chip-bonus {
    background: rgba(245, 158, 11, 0.18);
    color: #b45309;
    border: 1px solid rgba(245, 158, 11, 0.45);
  }
  :global(html[data-theme='dark']) .qd-chip-bonus { color: #fbbf24; }

  .qd-foot {
    text-align: center;
    font-size: 11px;
    color: var(--color-fg-3);
  }

  @keyframes qd-spin {
    to { transform: rotate(360deg); }
  }
</style>
