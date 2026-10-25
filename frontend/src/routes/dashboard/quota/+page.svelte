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
  import {
    Layers, RefreshCw, Search, Clock, Cpu, Server, Activity, ShieldCheck, AlertCircle
  } from 'lucide-svelte';

  let loading = $state(true);
  let error = $state('');
  let quotaProviders = $state<any[]>([]);
  let refreshingConnId = $state<string | null>(null);
  let refreshingAllQuota = $state(false);
  let quotaSearch = $state('');
  let quotaFilter = $state<'all' | 'antigravity' | 'commandcode' | 'qoder' | 'other'>('all');

  function formatCountdown(resetTimeStr?: string): string {
    if (!resetTimeStr) return '—';
    try {
      const target = new Date(resetTimeStr).getTime();
      const diff = target - Date.now();
      if (diff <= 0) return 'Resetting now';
      const days = Math.floor(diff / (1000 * 60 * 60 * 24));
      const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
      const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
      if (days > 0) return `${days}d ${hours}h`;
      if (hours > 0) return `${hours}h ${minutes}m`;
      return `${minutes}m`;
    } catch {
      return resetTimeStr;
    }
  }

  function getQuotaColor(pct: number): { text: string; bg: string; border: string; pill: string } {
    if (pct > 70) {
      return {
        text: 'var(--color-success)',
        bg: 'var(--color-success)',
        border: 'rgba(16, 185, 129, 0.25)',
        pill: 'rgba(16, 185, 129, 0.12)'
      };
    }
    if (pct >= 30) {
      return {
        text: '#f59e0b',
        bg: '#f59e0b',
        border: 'rgba(245, 158, 11, 0.25)',
        pill: 'rgba(245, 158, 11, 0.12)'
      };
    }
    return {
      text: 'var(--color-error)',
      bg: 'var(--color-error)',
      border: 'rgba(239, 68, 68, 0.25)',
      pill: 'rgba(239, 68, 68, 0.12)'
    };
  }

  function getProviderBadge(prov?: string) {
    const p = (prov || '').toLowerCase();
    if (p.includes('antigravity')) return { label: 'AG', bg: '#2563eb', color: '#fff' };
    if (p.includes('commandcode')) return { label: 'CC', bg: '#0d9488', color: '#fff' };
    if (p.includes('qoder')) return { label: 'QD', bg: '#7c3aed', color: '#fff' };
    if (p.includes('openrouter')) return { label: 'OR', bg: '#ea580c', color: '#fff' };
    if (p.includes('deepseek')) return { label: 'DS', bg: '#0284c7', color: '#fff' };
    return { label: p ? p.slice(0, 2).toUpperCase() : 'AI', bg: 'var(--color-primary)', color: '#fff' };
  }

  async function loadQuotas() {
    loading = true;
    error = '';
    try {
      const res = await api.get<any>('/api/quota/providers');
      quotaProviders = res?.providers || [];
    } catch (e: any) {
      error = e.message || 'Failed to load live quota statistics';
    }
    loading = false;
  }

  async function refreshSingleQuota(connId: string) {
    refreshingConnId = connId;
    try {
      const res = await api.get<any>(`/api/usage/${connId}?refresh=1`);
      if (res) {
        quotaProviders = quotaProviders.map((p: any) => {
          if (p.connection_id === connId) {
            return { ...p, ...res };
          }
          return p;
        });
      }
    } catch (e: any) {
      console.error('Failed to refresh quota:', e);
    }
    refreshingConnId = null;
  }

  async function refreshAllQuotas() {
    refreshingAllQuota = true;
    try {
      const res = await api.get<any>('/api/quota/providers?refresh=1');
      if (res?.providers) {
        quotaProviders = res.providers;
      }
    } catch (e: any) {
      console.error('Failed to refresh all quotas:', e);
    }
    refreshingAllQuota = false;
  }

  const filteredQuotaProviders = $derived(
    (quotaProviders || []).filter((p: any) => {
      const prov = (p.provider || '').toLowerCase();
      const name = (p.name || '').toLowerCase();
      const q = quotaSearch.toLowerCase().trim();

      if (quotaFilter !== 'all') {
        if (quotaFilter === 'other') {
          if (['antigravity', 'commandcode', 'qoder'].includes(prov)) return false;
        } else if (prov !== quotaFilter) {
          return false;
        }
      }

      if (q) {
        return prov.includes(q) || name.includes(q) || (p.connection_id || '').toLowerCase().includes(q);
      }
      return true;
    })
  );

  const stats = $derived(() => {
    const list = quotaProviders || [];
    let healthyCount = 0;
    let totalLimits = 0;
    let sumPct = 0;
    let limitCount = 0;

    for (const p of list) {
      if (!p.error) healthyCount++;
      for (const g of p.groups || []) {
        for (const l of g.limits || []) {
          totalLimits++;
          if (typeof l.remainingPercentage === 'number') {
            sumPct += l.remainingPercentage;
            limitCount++;
          }
        }
      }
    }

    const avgPct = limitCount > 0 ? (sumPct / limitCount).toFixed(1) : '100';
    return {
      totalAccounts: list.length,
      healthyCount,
      totalLimits,
      avgPct
    };
  });

  onMount(() => {
    loadQuotas();
  });
</script>

<svelte:head>
  <title>Quota Tracker — Lintasan</title>
</svelte:head>

<TabNav tabs={__tabs} />

<div style="animation: fadeInUp 0.4s ease-out;">
  <!-- Header -->
  <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between mb-5">
    <div>
      <div class="flex items-center gap-2 mb-1">
        <h2 style="font-size: 20px; font-weight: 700; color: var(--color-fg-0); margin: 0;">Quota Tracker</h2>
        <span style="font-size: 10px; font-weight: 700; letter-spacing: 0.04em; padding: 2px 7px; border-radius: 12px; background: rgba(16, 185, 129, 0.12); color: var(--color-success); border: 1px solid rgba(16, 185, 129, 0.25);">
          LIVE UPSTREAM
        </span>
      </div>
      <p style="font-size: 13px; color: var(--color-fg-2); margin: 0;">
        Real-time upstream rolling windows, weekly caps, and credit balances across connected provider accounts (9Router parity).
      </p>
    </div>

    <div class="flex items-center gap-2 flex-wrap">
      <div style="position: relative;">
        <input
          type="text"
          placeholder="Search account or ID..."
          bind:value={quotaSearch}
          style="padding: 6px 12px 6px 30px; font-size: 12px; border-radius: 8px; border: 1px solid var(--color-border); background: var(--color-bg-card); color: var(--color-fg-0); width: 200px;"
        />
        <Search size={14} style="position: absolute; left: 10px; top: 50%; transform: translateY(-50%); color: var(--color-fg-3); pointer-events: none;" />
      </div>

      <button
        class="btn-primary flex items-center gap-1.5"
        style="padding: 6px 14px; font-size: 12px; font-weight: 600;"
        onclick={refreshAllQuotas}
        disabled={refreshingAllQuota}
        title="Force refresh live quota from all provider APIs"
      >
        <RefreshCw size={13} class={refreshingAllQuota ? 'spinning' : ''} />
        <span>Refresh All</span>
      </button>
    </div>
  </div>

  {#if loading}
    <div class="card flex items-center justify-center py-16">
      <Spinner size={32} />
    </div>
  {:else if error}
    <div class="card p-6 border-red-500/20 bg-red-500/5 text-center">
      <AlertCircle size={28} class="mx-auto mb-2 text-red-500" />
      <p class="text-sm text-red-400">{error}</p>
      <button class="btn-secondary mt-3" onclick={loadQuotas}>Retry</button>
    </div>
  {:else}
    <!-- Summary Metrics Grid -->
    {@const s = stats()}
    <div class="metric-grid mb-5">
      <div class="metric-card">
        <div class="metric-icon" style="color: var(--color-primary); background: rgba(59,130,246,0.1);"><Server size={18} /></div>
        <div class="metric-body">
          <span class="metric-label">CONNECTED ACCOUNTS</span>
          <strong class="metric-val">{s.totalAccounts}</strong>
        </div>
      </div>
      <div class="metric-card">
        <div class="metric-icon" style="color: #10b981; background: rgba(16,185,129,0.1);"><ShieldCheck size={18} /></div>
        <div class="metric-body">
          <span class="metric-label">HEALTHY CREDENTIALS</span>
          <strong class="metric-val">{s.healthyCount} / {s.totalAccounts}</strong>
        </div>
      </div>
      <div class="metric-card">
        <div class="metric-icon" style="color: #8b5cf6; background: rgba(139,92,246,0.1);"><Layers size={18} /></div>
        <div class="metric-body">
          <span class="metric-label">RATE WINDOWS & LIMITS</span>
          <strong class="metric-val">{s.totalLimits}</strong>
        </div>
      </div>
      <div class="metric-card">
        <div class="metric-icon" style="color: #f59e0b; background: rgba(245,158,11,0.1);"><Activity size={18} /></div>
        <div class="metric-body">
          <span class="metric-label">AVG REMAINING QUOTA</span>
          <strong class="metric-val">{s.avgPct}%</strong>
        </div>
      </div>
    </div>

    <!-- Filter Tabs -->
    <div class="flex items-center gap-1.5 mb-4 flex-wrap">
      <button
        class="filter-pill"
        class:active={quotaFilter === 'all'}
        onclick={() => quotaFilter = 'all'}
      >
        All Providers ({quotaProviders.length})
      </button>
      <button
        class="filter-pill"
        class:active={quotaFilter === 'antigravity'}
        onclick={() => quotaFilter = 'antigravity'}
      >
        Antigravity ({quotaProviders.filter(p => p.provider === 'antigravity').length})
      </button>
      <button
        class="filter-pill"
        class:active={quotaFilter === 'commandcode'}
        onclick={() => quotaFilter = 'commandcode'}
      >
        Command Code ({quotaProviders.filter(p => p.provider === 'commandcode').length})
      </button>
      <button
        class="filter-pill"
        class:active={quotaFilter === 'qoder'}
        onclick={() => quotaFilter = 'qoder'}
      >
        Qoder ({quotaProviders.filter(p => p.provider === 'qoder').length})
      </button>
      {#if quotaProviders.some(p => !['antigravity', 'commandcode', 'qoder'].includes(p.provider))}
        <button
          class="filter-pill"
          class:active={quotaFilter === 'other'}
          onclick={() => quotaFilter = 'other'}
        >
          Other Providers ({quotaProviders.filter(p => !['antigravity', 'commandcode', 'qoder'].includes(p.provider)).length})
        </button>
      {/if}
    </div>

    <!-- Provider Accounts List -->
    <div style="display: flex; flex-direction: column; gap: 16px;">
      {#if filteredQuotaProviders.length === 0}
        <div class="card" style="padding: 40px; text-align: center;">
          <EmptyState
            title="No matching provider accounts"
            description="Try adjusting your search query or filter tab."
          />
        </div>
      {:else}
        {#each filteredQuotaProviders as provider}
          {@const badge = getProviderBadge(provider.provider)}
          <div class="quota-account-card">
            <div class="quota-account-header">
              <div class="flex items-center gap-3">
                <span class="provider-avatar" style="background: {badge.bg}; color: {badge.color};">
                  {badge.label}
                </span>
                <div>
                  <div class="flex items-center gap-2 flex-wrap">
                    <strong style="font-size: 15px; color: var(--color-fg-0);">{provider.name || provider.provider}</strong>
                    <span class="plan-pill">{provider.plan ? provider.plan.toUpperCase() : 'STANDARD'}</span>
                    <span class="badge" style="font-size: 10px; padding: 2px 7px; background: var(--color-border-light); color: var(--color-fg-2); text-transform: uppercase;">
                      {provider.provider}
                    </span>
                  </div>
                  <small style="font-size: 11px; color: var(--color-fg-3);">
                    Connection: {provider.connection_id}
                    {#if provider.fetched_at}
                      · Last polled: {new Date(provider.fetched_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                    {/if}
                  </small>
                </div>
              </div>

              <div class="flex items-center gap-2">
                <button
                  class="btn-secondary flex items-center gap-1.5"
                  style="padding: 5px 12px; font-size: 11px; font-weight: 500;"
                  onclick={() => refreshSingleQuota(provider.connection_id)}
                  disabled={refreshingConnId === provider.connection_id}
                  title="Force refresh live quota from upstream"
                >
                  <RefreshCw size={12} class={refreshingConnId === provider.connection_id ? 'spinning' : ''} />
                  <span>Refresh</span>
                </button>
              </div>
            </div>

            {#if provider.error}
              <div style="padding: 12px 14px; background: rgba(239, 68, 68, 0.08); border: 1px solid rgba(239, 68, 68, 0.2); border-radius: 8px; color: var(--color-error); font-size: 12px; margin-top: 10px;">
                ⚠️ {provider.error}
              </div>
            {:else if provider.groups?.length}
              <div class="grid grid-cols-1 md:grid-cols-2 gap-3 mt-3">
                {#each provider.groups as group}
                  <div class="quota-group-card">
                    <div class="flex items-center justify-between mb-2 pb-2" style="border-bottom: 1px solid var(--color-border-light);">
                      <strong style="font-size: 13px; color: var(--color-fg-0);">{group.name}</strong>
                      <span style="font-size: 11px; color: var(--color-fg-3);">{group.limits?.length || 0} limits</span>
                    </div>
                    {#if group.description}
                      <p style="font-size: 11px; color: var(--color-fg-3); margin: 0 0 10px 0;">{group.description}</p>
                    {/if}

                    <div style="display: flex; flex-direction: column; gap: 12px;">
                      {#each group.limits as limit}
                        {@const colors = getQuotaColor(limit.remainingPercentage ?? 100)}
                        <div class="quota-limit-row">
                          <div class="flex items-center justify-between text-xs mb-1">
                            <span class="flex items-center gap-1.5" style="font-weight: 600; color: var(--color-fg-1);">
                              {limit.displayName}
                              {#if limit.exceeded}
                                <span style="background: rgba(239,68,68,0.12); color: #ef4444; border: 1px solid rgba(239,68,68,0.25); padding: 0 4px; border-radius: 4px; font-weight: 700; font-size: 9.5px;">
                                  EXCEEDED
                                </span>
                              {/if}
                            </span>
                            <span style="font-weight: 700; font-family: var(--font-mono); color: {colors.text}; background: {colors.pill}; padding: 1px 7px; border-radius: 6px; border: 1px solid {colors.border}; font-size: 11px;">
                              {(limit.remainingPercentage ?? 100).toFixed(1)}%
                            </span>
                          </div>

                          <!-- Progress Track -->
                          <div class="quota-progress-track">
                            <div
                              class="quota-progress-fill"
                              style="width: {Math.max(0, Math.min(100, limit.remainingPercentage ?? 100))}%; background: {colors.bg};"
                            ></div>
                          </div>

                          <div class="flex items-center justify-between text-[11px] mt-1" style="color: var(--color-fg-3);">
                            <div class="flex items-center gap-1.5 flex-wrap">
                              {#if limit.isCreditBalance || limit.currency === 'USD' || limit.window === 'credits'}
                                <span style="font-weight: 600; color: var(--color-fg-1);">
                                  ${(limit.remaining != null ? limit.remaining : Math.max(0, (limit.total || 0) - (limit.used || 0))).toFixed(2)} / ${Number(limit.total || 0).toFixed(2)} {limit.currency || 'USD'}
                                </span>
                                {#if limit.used > 0}
                                  <span style="color: var(--color-fg-3); font-size: 10.5px;">
                                    (Used ${Number(limit.used).toFixed(2)})
                                  </span>
                                {/if}
                              {:else if limit.total > 0}
                                <span>
                                  {Number(limit.used).toLocaleString()} / {Number(limit.total).toLocaleString()}
                                </span>
                              {:else if limit.description}
                                <span>{limit.description}</span>
                              {:else}
                                <span>Active</span>
                              {/if}
                            </div>
                            {#if limit.resetAt}
                              <span class="flex items-center gap-1 font-medium" style="color: var(--color-fg-2);">
                                <Clock size={11} /> {limit.resetAt.includes('T') ? 'Reset in ' + formatCountdown(limit.resetAt) : limit.resetAt}
                              </span>
                            {/if}
                          </div>
                        </div>
                      {/each}
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  {/if}
</div>

<style>
  .metric-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 14px;
  }
  .metric-card {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 16px 18px;
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: var(--radius, 10px);
  }
  .metric-icon {
    width: 38px;
    height: 38px;
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
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    color: var(--color-fg-3);
  }
  .metric-val {
    font-size: 18px;
    font-weight: 700;
    color: var(--color-fg-0);
    line-height: 1.1;
  }
  :global(.spinning) {
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }

  .quota-account-card {
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: 12px;
    padding: 18px 20px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.02);
  }
  .quota-account-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
    padding-bottom: 12px;
    border-bottom: 1px solid var(--color-border-light);
  }
  .provider-avatar {
    width: 34px;
    height: 34px;
    border-radius: 9px;
    display: grid;
    place-items: center;
    font-weight: 700;
    font-size: 13px;
  }
  .plan-pill {
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.05em;
    padding: 2px 7px;
    border-radius: 6px;
    background: rgba(99, 102, 241, 0.12);
    color: var(--color-primary);
    border: 1px solid rgba(99, 102, 241, 0.25);
  }
  .quota-group-card {
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: 10px;
    padding: 14px 16px;
  }
  .quota-limit-row {
    display: flex;
    flex-direction: column;
  }
  .quota-progress-track {
    height: 7px;
    background: var(--color-border-light, #e2e8f0);
    border-radius: 4px;
    overflow: hidden;
  }
  .quota-progress-fill {
    height: 100%;
    transition: width 0.3s ease, background 0.3s ease;
    border-radius: 4px;
  }
  .filter-pill {
    all: unset;
    cursor: pointer;
    font-size: 11px;
    font-weight: 500;
    padding: 4px 12px;
    border-radius: 14px;
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    color: var(--color-fg-2);
    transition: all 0.15s ease;
  }
  .filter-pill:hover {
    border-color: var(--color-primary);
    color: var(--color-primary);
  }
  .filter-pill.active {
    background: rgba(99, 102, 241, 0.12);
    border-color: rgba(99, 102, 241, 0.3);
    color: var(--color-primary);
    font-weight: 600;
  }
</style>
