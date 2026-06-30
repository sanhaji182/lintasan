<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { DollarSign, TrendingDown, Zap, Database, Leaf, RefreshCw, AlertCircle } from 'lucide-svelte/icons';

  let summary = $state<any>(null);
  let history = $state<any[]>([]);
  let loading = $state(true);
  let error = $state('');

  const breakdownColors: Record<string, string> = {
    compression: 'var(--color-primary)',
    routing: 'var(--color-purple, #8b5cf6)',
    cache: 'var(--color-warning, #f59e0b)',
    free_tier: 'var(--color-success)'
  };

  const breakdownIcons: Record<string, typeof DollarSign> = {
    compression: Zap,
    routing: RefreshCw,
    cache: Database,
    free_tier: Leaf
  };

  const breakdownLabels: Record<string, string> = {
    compression: 'Compression',
    routing: 'Smart Routing',
    cache: 'Cache Hits',
    free_tier: 'Free Tier'
  };

  async function loadSavings() {
    loading = true;
    error = '';
    try {
      const [summaryData, historyData] = await Promise.all([
        api.get('/api/savings/summary'),
        api.get('/api/savings/history')
      ]);
      summary = summaryData;
      history = (historyData as any).history || [];
    } catch (e: any) {
      error = e.message || 'Failed to load savings data';
    }
    loading = false;
  }

  onMount(loadSavings);

  function formatCurrency(val: any) {
    return '$' + (val || 0).toFixed(2);
  }

  function formatNumber(val: any) {
    return (val || 0).toLocaleString();
  }
</script>

<div style="animation: fadeInUp 0.4s ease-out;">
  <h2 style="font-size: 18px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 4px;">Cost Savings</h2>
  <p style="font-size: 13px; color: var(--color-fg-3); margin-bottom: 20px;">Track how much you save with Lintasan's smart routing, compression, and caching.</p>

  {#if loading}
    <Spinner />
  {:else if error}
    <div class="card"><EmptyState icon={AlertCircle} title="Failed to load savings" description={error} action={loadSavings} actionLabel="Retry" /></div>
  {:else if !summary}
    <div class="card"><EmptyState icon={DollarSign} title="No savings data" description="Savings will appear once traffic flows through the gateway." /></div>
  {:else}
    <!-- Summary stat cards -->
    <div class="card mb-5" style="padding: 0; overflow: hidden;">
      <div class="grid grid-cols-4" style="gap: 1px; background: var(--color-border);">
        {#each [
          { label: 'TOTAL SAVINGS', value: formatCurrency(summary.total_savings), color: 'var(--color-success)' },
          { label: 'TOTAL REQUESTS', value: formatNumber(summary.total_requests), color: 'var(--color-primary)' },
          { label: 'TOTAL TOKENS', value: formatNumber(summary.total_tokens), color: 'var(--color-info, #3b82f6)' },
          { label: 'AVG / REQUEST', value: summary.total_requests > 0 ? formatCurrency(summary.total_savings / summary.total_requests) : '$0.00', color: 'var(--color-warning, #f59e0b)' }
        ] as stat}
          <div class="text-center" style="padding: 16px 20px; background: var(--color-bg-card);">
            <div style="font-size: 11px; font-weight: 500; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 4px;">{stat.label}</div>
            <div style="font-size: 20px; font-weight: 700; color: {stat.color}; font-family: var(--font-mono);">{stat.value}</div>
          </div>
        {/each}
      </div>
    </div>

    <!-- Savings Breakdown -->
    {#if summary.breakdown}
      <div class="card mb-5">
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 16px;">Savings Breakdown</div>
        <div class="grid gap-4" style="grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));">
          {#each Object.entries(summary.breakdown) as [key, value]}
            {@const color = breakdownColors[key] || 'var(--color-fg-2)'}
            {@const Icon = breakdownIcons[key] || DollarSign}
            {@const label = breakdownLabels[key] || key}
            <div class="card" style="padding: 16px; position: relative; overflow: hidden;">
              <div style="position: absolute; top: 0; left: 0; right: 0; height: 3px; background: {color};"></div>
              <Icon size={18} style="color: {color}; margin-bottom: 8px;" stroke-width={1.8} />
              <div style="font-size: 20px; font-weight: 700; font-family: var(--font-mono); color: var(--color-fg-0);">{formatCurrency(value)}</div>
              <div style="font-size: 12px; font-weight: 500; color: var(--color-fg-3); margin-top: 2px;">{label}</div>
            </div>
          {/each}
        </div>
      </div>
    {/if}

    <!-- History Table -->
    <div class="card">
      <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 16px;">
        <TrendingDown size={16} style="display: inline; vertical-align: -3px; margin-right: 6px; color: var(--color-primary);" />
        Daily Savings History
      </div>
      {#if history.length === 0}
        <EmptyState icon={DollarSign} title="No history yet" description="Start using Lintasan to track savings over time." />
      {:else}
        <div style="overflow-x: auto;">
          <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
            <thead>
              <tr>
                {#each ['Date', 'Requests', 'Tokens', 'Savings'] as h}
                  <th style="text-align: {h === 'Date' ? 'left' : 'right'}; padding: 12px 14px; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; color: var(--color-fg-3); border-bottom: 1px solid var(--color-border); background: var(--color-bg-body);">{h}</th>
                {/each}
              </tr>
            </thead>
            <tbody>
              {#each history as day}
                <tr style="border-bottom: 1px solid var(--color-border-light);">
                  <td style="padding: 10px 14px; font-weight: 500; color: var(--color-fg-0);">{day.date}</td>
                  <td style="padding: 10px 14px; text-align: right; font-family: var(--font-mono); font-size: 12px;">{formatNumber(day.requests)}</td>
                  <td style="padding: 10px 14px; text-align: right; font-family: var(--font-mono); font-size: 12px;">{formatNumber(day.tokens)}</td>
                  <td style="padding: 10px 14px; text-align: right; font-family: var(--font-mono); font-size: 12px; color: var(--color-success); font-weight: 600;">{formatCurrency(day.savings)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}
</div>
