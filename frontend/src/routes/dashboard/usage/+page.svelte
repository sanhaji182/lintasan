<script lang="ts">
  import TabNav from '$lib/components/TabNav.svelte';
  const __tabs = [
    { label: 'Requests', path: '/dashboard/analytics' },
    { label: 'Usage', path: '/dashboard/usage' },
    { label: 'Quota Tracker', path: '/dashboard/quota' },
    { label: 'Savings', path: '/dashboard/savings' },
    { label: 'Logs', path: '/dashboard/logs' },
    { label: 'Metrics', path: '/dashboard/observability' }
  ];
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { BarChart3, ChartColumn, DollarSign, Database, Server, RefreshCw, AlertCircle, Search, Activity, Zap, Cpu, Layers } from 'lucide-svelte';

  let data = $state<any>(null);
  let quotaData = $state<any[]>([]);
  let loading = $state(true);
  let error = $state('');
  let canvasEl: HTMLCanvasElement = $state()!;
  let period = $state<'today' | '7d' | '30d' | 'all'>('all');
  let metricMode = $state<'tokens' | 'requests'>('tokens');
  let providerSearch = $state('');
  let modelSearch = $state('');

  const COLORS = ['#3c50e0', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899', '#06b6d4', '#14b8a6', '#6366f1'];

  async function loadUsage() {
    loading = true;
    error = '';
    try {
      const [usageRes, quotaRes] = await Promise.all([
        api.get<any>('/api/usage'),
        api.get<any>('/api/quota/stats').catch(() => null)
      ]);
      data = usageRes?.data || usageRes;
      quotaData = quotaRes?.data?.by_connection || quotaRes?.data || [];
    } catch (e: any) {
      error = e.message || 'Failed to load usage data';
    } finally {
      loading = false;
    }
  }

  onMount(loadUsage);

  const totalTokens = $derived(
    (data?.providers || []).reduce((s: number, p: any) => s + (p.tokens || 0), 0)
  );
  const totalRequests = $derived(
    (data?.providers || []).reduce((s: number, p: any) => s + (p.requests || 0), 0)
  );

  const filteredDaily = $derived(() => {
    if (!data?.daily) return [];
    const list = [...data.daily].sort((a, b) => a.date.localeCompare(b.date));
    if (period === 'today') return list.slice(-1);
    if (period === '7d') return list.slice(-7);
    if (period === '30d') return list.slice(-30);
    return list.slice(-14);
  });

  const filteredProviders = $derived(() => {
    if (!data?.providers) return [];
    if (!providerSearch.trim()) return data.providers;
    const q = providerSearch.toLowerCase();
    return data.providers.filter((p: any) => (p.provider || p.name || '').toLowerCase().includes(q));
  });

  const filteredModels = $derived(() => {
    if (!data?.models) return [];
    if (!modelSearch.trim()) return data.models;
    const q = modelSearch.toLowerCase();
    return data.models.filter((m: any) => (m.model || '').toLowerCase().includes(q));
  });

  $effect(() => {
    const list = filteredDaily();
    if (list.length && canvasEl) drawChart();
  });

  function drawChart() {
    const ctx = canvasEl.getContext('2d');
    if (!ctx) return;
    const daily = filteredDaily();
    if (!daily.length) return;

    const dpr = window.devicePixelRatio || 1;
    const w = canvasEl.clientWidth;
    const h = 220;
    canvasEl.width = w * dpr;
    canvasEl.height = h * dpr;
    canvasEl.style.height = h + 'px';
    ctx.scale(dpr, dpr);

    ctx.clearRect(0, 0, w, h);

    const isToken = metricMode === 'tokens';
    const values = daily.map((d: any) => (isToken ? (d.tokens || 0) : (d.requests || 0)));
    const maxVal = Math.max(...values, 1);
    const barW = Math.max((w - 40) / daily.length - 8, 12);
    const padX = 24;

    // Grid lines
    ctx.strokeStyle = getComputedStyle(document.documentElement).getPropertyValue('--color-border').trim() || '#e2e8f0';
    ctx.lineWidth = 0.5;
    for (let i = 0; i <= 4; i++) {
      const y = 20 + (h - 50) * (1 - i / 4);
      ctx.beginPath();
      ctx.moveTo(padX, y);
      ctx.lineTo(w - padX, y);
      ctx.stroke();
    }

    // Bars
    daily.forEach((d: any, i: number) => {
      const val = isToken ? (d.tokens || 0) : (d.requests || 0);
      const barH = (val / maxVal) * (h - 55);
      const x = padX + i * ((w - 2 * padX) / daily.length) + 4;
      const y = h - 30 - barH;

      const grad = ctx.createLinearGradient(x, y, x, h - 30);
      grad.addColorStop(0, isToken ? '#3c50e0' : '#10b981');
      grad.addColorStop(1, isToken ? '#6366f1' : '#34d399');
      ctx.fillStyle = grad;
      ctx.beginPath();
      ctx.roundRect(x, y, barW, barH, [4, 4, 0, 0]);
      ctx.fill();

      // Date label
      ctx.fillStyle = getComputedStyle(document.documentElement).getPropertyValue('--color-fg-3').trim() || '#94a3b8';
      ctx.font = '10px Inter';
      ctx.textAlign = 'center';
      const label = d.date ? d.date.slice(5) : '';
      ctx.fillText(label, x + barW / 2, h - 12);
    });
  }
</script>

<svelte:head>
  <title>Usage & Quota — Lintasan</title>
</svelte:head>

<TabNav tabs={__tabs} />

<div style="animation: fadeInUp 0.4s ease-out;">
  <!-- Header with Period Controls (ala 9router) -->
  <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between mb-5">
    <div>
      <h2 style="font-size: 20px; font-weight: 700; color: var(--color-fg-0); margin: 0 0 4px 0;">Usage & Quotas</h2>
      <p style="font-size: 13px; color: var(--color-fg-2); margin: 0;">Comprehensive breakdown of tokens, request volume, and connected accounts.</p>
    </div>
    <div class="flex items-center gap-2">
      <div style="display: inline-flex; border: 1px solid var(--color-border); border-radius: 8px; overflow: hidden; background: var(--color-bg-card);">
        {#each [
          { key: 'today', label: 'Today' },
          { key: '7d', label: '7D' },
          { key: '30d', label: '30D' },
          { key: 'all', label: 'All' }
        ] as p}
          <button
            class="period-btn"
            class:active={period === p.key}
            onclick={() => { period = p.key as any; }}
          >
            {p.label}
          </button>
        {/each}
      </div>
      <button class="btn-secondary flex items-center gap-1.5" onclick={loadUsage} disabled={loading} style="padding: 6px 12px; font-size: 12px;">
        <RefreshCw size={13} class={loading ? 'spinning' : ''} /> Refresh
      </button>
    </div>
  </div>

  {#if loading}
    <Spinner />
  {:else if error}
    <div class="card"><EmptyState icon={AlertCircle} title="Failed to load usage" description={error} action={loadUsage} actionLabel="Retry" /></div>
  {:else if !data}
    <div class="card"><EmptyState icon={ChartColumn} title="No usage data recorded yet" description="Send requests through the proxy router to see live usage metrics." actionHref="/dashboard/playground" actionLabel="Open Playground" /></div>
  {:else}
    <!-- Summary stats cards (4 cards) -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 mb-5">
      <div class="metric-card">
        <div class="metric-icon" style="color: #3c50e0; background: rgba(60,80,224,0.1);"><Activity size={18} /></div>
        <div class="metric-body">
          <span class="metric-label">TOTAL REQUESTS</span>
          <strong class="metric-val">{totalRequests.toLocaleString()}</strong>
        </div>
      </div>
      <div class="metric-card">
        <div class="metric-icon" style="color: #8b5cf6; background: rgba(139,92,246,0.1);"><Zap size={18} /></div>
        <div class="metric-body">
          <span class="metric-label">TOTAL TOKENS</span>
          <strong class="metric-val">{totalTokens.toLocaleString()}</strong>
        </div>
      </div>
      <div class="metric-card">
        <div class="metric-icon" style="color: #10b981; background: rgba(16,185,129,0.1);"><Server size={18} /></div>
        <div class="metric-body">
          <span class="metric-label">ACTIVE PROVIDERS</span>
          <strong class="metric-val">{(data.providers || []).length}</strong>
        </div>
      </div>
      <div class="metric-card">
        <div class="metric-icon" style="color: #f59e0b; background: rgba(245,158,11,0.1);"><Cpu size={18} /></div>
        <div class="metric-body">
          <span class="metric-label">MODELS IN USE</span>
          <strong class="metric-val">{(data.models || []).length}</strong>
        </div>
      </div>
    </div>

    <!-- Chart Panel -->
    <div class="card mb-5" style="padding: 18px 20px;">
      <div class="flex items-center justify-between mb-4 flex-wrap gap-2">
        <div class="flex items-center gap-2">
          <BarChart3 size={16} style="color: var(--color-primary);" />
          <h3 style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin: 0;">Daily Usage Trend</h3>
        </div>
        <div class="flex items-center gap-2">
          <div style="display: inline-flex; border: 1px solid var(--color-border); border-radius: 6px; overflow: hidden; font-size: 11px;">
            <button
              class="metric-toggle-btn"
              class:active={metricMode === 'tokens'}
              onclick={() => { metricMode = 'tokens'; }}
            >
              Tokens
            </button>
            <button
              class="metric-toggle-btn"
              class:active={metricMode === 'requests'}
              onclick={() => { metricMode = 'requests'; }}
            >
              Requests
            </button>
          </div>
        </div>
      </div>
      <canvas bind:this={canvasEl} style="width: 100%;"></canvas>
    </div>

    <!-- Grid: Provider & Model Breakdowns -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-5 mb-5">
      <!-- Provider Breakdown Table -->
      <div class="card" style="padding: 18px 20px;">
        <div class="flex items-center justify-between mb-3 flex-wrap gap-2">
          <div class="flex items-center gap-2">
            <Server size={15} style="color: var(--color-primary);" />
            <h3 style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin: 0;">Usage by Provider / Account</h3>
          </div>
          <div class="relative" style="width: 160px;">
            <Search size={12} class="absolute" style="left: 7px; top: 50%; transform: translateY(-50%); color: var(--color-fg-3);" />
            <input
              class="input-field"
              placeholder="Filter..."
              bind:value={providerSearch}
              style="padding-left: 24px; font-size: 11px; padding-top: 3px; padding-bottom: 3px;"
            />
          </div>
        </div>
        <div style="overflow-x: auto; max-height: 420px;">
          <table style="width: 100%; border-collapse: collapse; font-size: 12px;">
            <thead>
              <tr style="border-bottom: 1px solid var(--color-border);">
                <th style="text-align: left; padding: 8px 10px; font-size: 10px; font-weight: 600; text-transform: uppercase; color: var(--color-fg-3);">Provider</th>
                <th style="text-align: right; padding: 8px 10px; font-size: 10px; font-weight: 600; text-transform: uppercase; color: var(--color-fg-3);">Reqs</th>
                <th style="text-align: right; padding: 8px 10px; font-size: 10px; font-weight: 600; text-transform: uppercase; color: var(--color-fg-3);">Tokens</th>
                <th style="text-align: right; padding: 8px 10px; font-size: 10px; font-weight: 600; text-transform: uppercase; color: var(--color-fg-3); width: 70px;">Share</th>
              </tr>
            </thead>
            <tbody>
              {#each filteredProviders() as p, i}
                {@const share = totalTokens > 0 ? (p.tokens || 0) / totalTokens * 100 : 0}
                <tr style="border-bottom: 1px solid var(--color-border-light);">
                  <td style="padding: 9px 10px;">
                    <div class="flex items-center gap-1.5">
                      <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {COLORS[i % COLORS.length]};"></span>
                      <span style="font-weight: 500; color: var(--color-fg-0); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 150px;" title={p.provider || p.name}>
                        {p.provider || p.name}
                      </span>
                    </div>
                  </td>
                  <td style="text-align: right; padding: 9px 10px; font-family: var(--font-mono); font-size: 11px; color: var(--color-fg-2);">
                    {(p.requests || 0).toLocaleString()}
                  </td>
                  <td style="text-align: right; padding: 9px 10px; font-family: var(--font-mono); font-size: 11px; font-weight: 600; color: var(--color-fg-0);">
                    {(p.tokens || 0).toLocaleString()}
                  </td>
                  <td style="text-align: right; padding: 9px 10px;">
                    <div style="display: flex; align-items: center; justify-content: flex-end; gap: 6px;">
                      <div style="width: 36px; height: 4px; background: var(--color-border); border-radius: 2px; overflow: hidden;">
                        <div style="width: {Math.min(100, Math.round(share))}%; height: 100%; background: var(--color-primary); border-radius: 2px;"></div>
                      </div>
                      <span style="font-size: 10px; font-family: var(--font-mono); color: var(--color-fg-3); width: 28px; text-align: right;">{share.toFixed(0)}%</span>
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>

      <!-- Model Breakdown Table -->
      <div class="card" style="padding: 18px 20px;">
        <div class="flex items-center justify-between mb-3 flex-wrap gap-2">
          <div class="flex items-center gap-2">
            <Cpu size={15} style="color: var(--color-primary);" />
            <h3 style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin: 0;">Usage by Model</h3>
          </div>
          <div class="relative" style="width: 160px;">
            <Search size={12} class="absolute" style="left: 7px; top: 50%; transform: translateY(-50%); color: var(--color-fg-3);" />
            <input
              class="input-field"
              placeholder="Filter..."
              bind:value={modelSearch}
              style="padding-left: 24px; font-size: 11px; padding-top: 3px; padding-bottom: 3px;"
            />
          </div>
        </div>
        <div style="overflow-x: auto; max-height: 420px;">
          <table style="width: 100%; border-collapse: collapse; font-size: 12px;">
            <thead>
              <tr style="border-bottom: 1px solid var(--color-border);">
                <th style="text-align: left; padding: 8px 10px; font-size: 10px; font-weight: 600; text-transform: uppercase; color: var(--color-fg-3);">Model</th>
                <th style="text-align: right; padding: 8px 10px; font-size: 10px; font-weight: 600; text-transform: uppercase; color: var(--color-fg-3);">Reqs</th>
                <th style="text-align: right; padding: 8px 10px; font-size: 10px; font-weight: 600; text-transform: uppercase; color: var(--color-fg-3);">Tokens</th>
                <th style="text-align: right; padding: 8px 10px; font-size: 10px; font-weight: 600; text-transform: uppercase; color: var(--color-fg-3); width: 70px;">Share</th>
              </tr>
            </thead>
            <tbody>
              {#each filteredModels() as m}
                {@const share = totalTokens > 0 ? (m.tokens || 0) / totalTokens * 100 : 0}
                <tr style="border-bottom: 1px solid var(--color-border-light);">
                  <td style="padding: 9px 10px; font-weight: 500; color: var(--color-fg-0); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 160px;" title={m.model}>
                    {m.model}
                  </td>
                  <td style="text-align: right; padding: 9px 10px; font-family: var(--font-mono); font-size: 11px; color: var(--color-fg-2);">
                    {(m.requests || 0).toLocaleString()}
                  </td>
                  <td style="text-align: right; padding: 9px 10px; font-family: var(--font-mono); font-size: 11px; font-weight: 600; color: var(--color-fg-0);">
                    {(m.tokens || 0).toLocaleString()}
                  </td>
                  <td style="text-align: right; padding: 9px 10px;">
                    <div style="display: flex; align-items: center; justify-content: flex-end; gap: 6px;">
                      <div style="width: 36px; height: 4px; background: var(--color-border); border-radius: 2px; overflow: hidden;">
                        <div style="width: {Math.min(100, Math.round(share))}%; height: 100%; background: #10b981; border-radius: 2px;"></div>
                      </div>
                      <span style="font-size: 10px; font-family: var(--font-mono); color: var(--color-fg-3); width: 28px; text-align: right;">{share.toFixed(0)}%</span>
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Quota Tracking -->
    {#if quotaData?.length}
      <div class="card mb-5" style="padding: 18px 20px;">
        <div class="flex items-center gap-2 mb-3">
          <Layers size={16} style="color: var(--color-primary);" />
          <h3 style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin: 0;">Provider Quota Tracking</h3>
        </div>
        <div style="overflow-x: auto;">
          <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
            <thead>
              <tr style="border-bottom: 1px solid var(--color-border);">
                {#each ['Provider', 'Input Tokens', 'Output Tokens', 'Cost Est.', 'Remaining', 'Next Reset'] as h}
                  <th style="text-align: left; padding: 10px 12px; font-size: 11px; font-weight: 600; text-transform: uppercase; color: var(--color-fg-3);">{h}</th>
                {/each}
              </tr>
            </thead>
            <tbody>
              {#each quotaData as q}
                <tr style="border-bottom: 1px solid var(--color-border-light);">
                  <td style="padding: 10px 12px; font-weight: 500; color: var(--color-fg-0);">{q.provider}</td>
                  <td style="padding: 10px 12px; font-family: var(--font-mono); font-size: 12px;">{(q.input_tokens || 0).toLocaleString()}</td>
                  <td style="padding: 10px 12px; font-family: var(--font-mono); font-size: 12px;">{(q.output_tokens || 0).toLocaleString()}</td>
                  <td style="padding: 10px 12px; font-family: var(--font-mono); font-size: 12px;">${(q.cost_estimate || 0).toFixed(4)}</td>
                  <td style="padding: 10px 12px;">
                    {#if q.remaining?.percent_remaining !== undefined}
                      <div style="display: flex; align-items: center; gap: 8px;">
                        <div style="flex: 1; height: 6px; background: var(--color-border); border-radius: 3px; overflow: hidden; max-width: 100px;">
                          <div style="width: {Math.max(0, Math.min(100, q.remaining.percent_remaining))}%; height: 100%; background: {q.remaining.percent_remaining > 20 ? 'var(--color-success)' : 'var(--color-error)'}; border-radius: 3px;"></div>
                        </div>
                        <span style="font-size: 11px; color: var(--color-fg-3); font-family: var(--font-mono);">{q.remaining.percent_remaining?.toFixed(0)}%</span>
                      </div>
                    {:else}
                      <span style="font-size: 12px; color: var(--color-fg-3);">Unlimited</span>
                    {/if}
                  </td>
                  <td style="padding: 10px 12px; font-size: 12px; color: var(--color-fg-3);">
                    {q.next_reset ? new Date(q.next_reset).toLocaleString() : '—'}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}
  {/if}
</div>

<style>
  .period-btn {
    padding: 5px 12px;
    font-size: 12px;
    font-weight: 500;
    color: var(--color-fg-2);
    background: transparent;
    border: none;
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }
  .period-btn:hover {
    color: var(--color-fg-0);
  }
  .period-btn.active {
    background: var(--color-primary);
    color: #ffffff;
    font-weight: 600;
  }
  .metric-toggle-btn {
    padding: 3px 8px;
    border: none;
    background: transparent;
    cursor: pointer;
    color: var(--color-fg-2);
  }
  .metric-toggle-btn.active {
    background: var(--color-primary);
    color: #ffffff;
    font-weight: 600;
  }
  .metric-card {
    padding: 16px 18px;
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    display: flex;
    align-items: center;
    gap: 14px;
    box-shadow: var(--shadow-sm);
  }
  .metric-icon {
    width: 40px;
    height: 40px;
    border-radius: 10px;
    display: grid;
    place-items: center;
    flex-shrink: 0;
  }
  .metric-body {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .metric-label {
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.05em;
    color: var(--color-fg-3);
  }
  .metric-val {
    font-size: 20px;
    font-weight: 700;
    color: var(--color-fg-0);
    font-family: var(--font-mono);
    line-height: 1.1;
  }
  :global(.spinning) {
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
</style>
