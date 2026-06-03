<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  let summary = $state<any>(null);
  let history = $state<any[]>([]);
  let loading = $state(true);

  onMount(async () => {
    try {
      const [summaryData, historyData] = await Promise.all([
        api.get('/api/savings/summary'),
        api.get('/api/savings/history')
      ]);
      summary = summaryData;
      history = (historyData as any).history || [];
    } catch (e) {
      console.error('Failed to load savings:', e);
    }
    loading = false;
  });

  function formatCurrency(val: any) {
    return '$' + (val || 0).toFixed(2);
  }

  function formatNumber(val: any) {
    return (val || 0).toLocaleString();
  }
</script>

<div class="p-6 bg-gray-900 min-h-screen text-white">
  <h1 class="text-2xl font-bold mb-6">💰 Cost Savings Tracker</h1>
  <p class="text-gray-400 mb-6">Track how much you save with Lintasan</p>

  {#if loading}
    <div class="text-center py-8">Loading savings data...</div>
  {:else if summary}
    <!-- Big Numbers -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
      <div class="bg-gradient-to-br from-green-900 to-green-800 rounded-lg p-6">
        <div class="text-sm text-green-300">Total Savings</div>
        <div class="text-3xl font-bold mt-2">{formatCurrency(summary.total_savings)}</div>
      </div>
      <div class="bg-gray-800 rounded-lg p-6">
        <div class="text-sm text-gray-400">Total Requests</div>
        <div class="text-3xl font-bold mt-2">{formatNumber(summary.total_requests)}</div>
      </div>
      <div class="bg-gray-800 rounded-lg p-6">
        <div class="text-sm text-gray-400">Total Tokens</div>
        <div class="text-3xl font-bold mt-2">{formatNumber(summary.total_tokens)}</div>
      </div>
      <div class="bg-gray-800 rounded-lg p-6">
        <div class="text-sm text-gray-400">Avg per Request</div>
        <div class="text-3xl font-bold mt-2">
          {summary.total_requests > 0 ? formatCurrency(summary.total_savings / summary.total_requests) : '$0.00'}
        </div>
      </div>
    </div>

    <!-- Breakdown -->
    <div class="bg-gray-800 rounded-lg p-6 mb-6">
      <h2 class="text-lg font-semibold mb-4">📊 Savings Breakdown</h2>
      <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div class="text-center">
          <div class="text-2xl font-bold text-blue-400">{formatCurrency(summary.breakdown?.compression)}</div>
          <div class="text-sm text-gray-400 mt-1">Compression</div>
        </div>
        <div class="text-center">
          <div class="text-2xl font-bold text-purple-400">{formatCurrency(summary.breakdown?.routing)}</div>
          <div class="text-sm text-gray-400 mt-1">Smart Routing</div>
        </div>
        <div class="text-center">
          <div class="text-2xl font-bold text-yellow-400">{formatCurrency(summary.breakdown?.cache)}</div>
          <div class="text-sm text-gray-400 mt-1">Cache Hits</div>
        </div>
        <div class="text-center">
          <div class="text-2xl font-bold text-green-400">{formatCurrency(summary.breakdown?.free_tier)}</div>
          <div class="text-sm text-gray-400 mt-1">Free Tier</div>
        </div>
      </div>
    </div>

    <!-- History Table -->
    <div class="bg-gray-800 rounded-lg p-6">
      <h2 class="text-lg font-semibold mb-4">📈 Daily History</h2>
      {#if history.length === 0}
        <p class="text-gray-400">No history data yet. Start using Lintasan to track savings!</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-gray-400 border-b border-gray-700">
                <th class="text-left py-2">Date</th>
                <th class="text-right py-2">Requests</th>
                <th class="text-right py-2">Tokens</th>
                <th class="text-right py-2">Savings</th>
              </tr>
            </thead>
            <tbody>
              {#each history as day}
                <tr class="border-b border-gray-700 hover:bg-gray-700">
                  <td class="py-2">{day.date}</td>
                  <td class="text-right">{formatNumber(day.requests)}</td>
                  <td class="text-right">{formatNumber(day.tokens)}</td>
                  <td class="text-right text-green-400">{formatCurrency(day.savings)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {:else}
    <div class="text-center py-8 text-gray-400">Failed to load savings data</div>
  {/if}
</div>
