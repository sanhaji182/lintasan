<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { TrendingUp, Zap, Database, Clock, Activity, Server, Cpu } from 'lucide-svelte/icons';

  interface LogEntry {
    id: string;
    model: string;
    provider: string;
    status: number;
    input_tokens: number;
    output_tokens: number;
    latency_ms: number;
    cached: number;
    created_at: string;
  }

  interface Stats {
    total_requests: number;
    cache_hit_rate: number;
    avg_latency: number;
    active_connections: number;
  }

  let stats = $state<Stats | null>(null);
  let logs = $state<LogEntry[]>([]);
  let loading = $state(true);

  const COLORS = ['#3c50e0', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16', '#f97316'];

  onMount(async () => {
    try {
      const [statsRes, logsRes] = await Promise.all([
        api.get<any>('/api/dashboard/stats').catch(() => null),
        api.get<{ data: LogEntry[] }>('/api/logs').catch(() => ({ data: [] }))
      ]);
      stats = statsRes?.data || statsRes || null;
      logs = logsRes?.data || [];
    } catch {}
    finally { loading = false; }
  });

  let providerBreakdown = $derived.by(() => {
    const map = new Map<string, { requests: number; tokens: number; totalLatency: number }>();
    for (const log of logs) {
      const p = log.provider || 'Unknown';
      const entry = map.get(p) || { requests: 0, tokens: 0, totalLatency: 0 };
      entry.requests++;
      entry.tokens += (log.input_tokens || 0) + (log.output_tokens || 0);
      entry.totalLatency += log.latency_ms || 0;
      map.set(p, entry);
    }
    return Array.from(map.entries()).map(([name, d]) => ({
      name,
      requests: d.requests,
      tokens: d.tokens,
      avgLatency: d.requests > 0 ? Math.round(d.totalLatency / d.requests) : 0
    })).sort((a, b) => b.requests - a.requests);
  });

  let modelBreakdown = $derived.by(() => {
    const map = new Map<string, { requests: number; inputTokens: number; outputTokens: number }>();
    for (const log of logs) {
      const m = log.model || 'Unknown';
      const entry = map.get(m) || { requests: 0, inputTokens: 0, outputTokens: 0 };
      entry.requests++;
      entry.inputTokens += log.input_tokens || 0;
      entry.outputTokens += log.output_tokens || 0;
      map.set(m, entry);
    }
    return Array.from(map.entries()).map(([name, d]) => ({
      name, ...d
    })).sort((a, b) => b.requests - a.requests);
  });

  let statusBreakdown = $derived.by(() => {
    let success = 0, errors = 0, cached = 0;
    for (const log of logs) {
      if (log.cached) cached++;
      else if (log.status >= 200 && log.status < 300) success++;
      else errors++;
    }
    return { success, errors, cached, total: logs.length };
  });

  let totalTokens = $derived(logs.reduce((s, l) => s + (l.input_tokens || 0) + (l.output_tokens || 0), 0));
  let avgLatency = $derived(logs.length > 0 ? Math.round(logs.reduce((s, l) => s + (l.latency_ms || 0), 0) / logs.length) : 0);

  function formatLatency(ms: number): string {
    if (ms >= 1000) return (ms / 1000).toFixed(1) + 's';
    return Math.round(ms) + 'ms';
  }
</script>

<div style="animation: fadeInUp 0.4s ease-out;">
  <h2 style="font-size: 18px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 20px;">Analytics</h2>

  {#if loading}
    <Spinner />
  {:else if logs.length === 0 && !stats}
    <div class="card"><EmptyState icon={TrendingUp} title="No analytics data" description="Analytics will appear once traffic flows through the gateway." /></div>
  {:else}
    <!-- Metric cards -->
    <div class="grid gap-4" style="grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); margin-bottom: 24px;">
      {#each [
        { icon: Activity, label: 'Total Requests', value: (stats?.total_requests ?? logs.length).toLocaleString(), color: 'var(--color-primary)' },
        { icon: Database, label: 'Total Tokens', value: totalTokens.toLocaleString(), color: 'var(--color-success)' },
        { icon: Zap, label: 'Cache Hit Rate', value: (stats?.cache_hit_rate ?? (logs.length > 0 ? Math.round((statusBreakdown.cached / logs.length) * 100) : 0)) + '%', color: 'var(--color-info)' },
        { icon: Clock, label: 'Avg Latency', value: formatLatency(stats?.avg_latency ?? avgLatency), color: 'var(--color-warning)' }
      ] as m}
        <div class="card" style="padding: 18px; position: relative; overflow: hidden;">
          <div style="position: absolute; top: 0; left: 0; right: 0; height: 3px; background: {m.color};"></div>
          <m.icon size={20} style="color: {m.color}; margin-bottom: 8px;" stroke-width={1.8} />
          <div style="font-size: 20px; font-weight: 700; font-family: var(--font-mono); color: var(--color-fg-0); letter-spacing: -0.3px;">{m.value}</div>
          <div style="font-size: 12px; font-weight: 500; color: var(--color-fg-3); margin-top: 2px;">{m.label}</div>
        </div>
      {/each}
    </div>

    <!-- Status breakdown -->
    <div class="card" style="margin-bottom: 24px;">
      <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 16px;">Request Status Breakdown</div>
      <div class="grid gap-3" style="grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));">
        {#each [
          { label: 'Success', value: statusBreakdown.success, color: 'var(--color-success)' },
          { label: 'Errors', value: statusBreakdown.errors, color: 'var(--color-error)' },
          { label: 'Cached', value: statusBreakdown.cached, color: 'var(--color-info)' },
          { label: 'Total', value: statusBreakdown.total, color: 'var(--color-primary)' }
        ] as item}
          {@const total = statusBreakdown.total || 1}
          <div>
            <div class="flex items-center justify-between" style="margin-bottom: 6px;">
              <span style="font-size: 12px; font-weight: 500; color: var(--color-fg-2);">{item.label}</span>
              <span style="font-size: 13px; font-weight: 700; font-family: var(--font-mono); color: var(--color-fg-0);">{item.value}</span>
            </div>
            <div style="height: 6px; background: var(--color-border); border-radius: 3px; overflow: hidden;">
              <div style="height: 100%; width: {(item.value / total) * 100}%; background: {item.color}; border-radius: 3px; transition: width 0.4s ease;"></div>
            </div>
          </div>
        {/each}
      </div>
    </div>

    <!-- Provider breakdown -->
    {#if providerBreakdown.length > 0}
      <div class="card" style="margin-bottom: 24px;">
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 16px;">
          <Server size={16} style="display: inline; vertical-align: -3px; margin-right: 6px; color: var(--color-primary);" />
          Requests by Provider
        </div>
        <div style="overflow-x: auto;">
          <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
            <thead>
              <tr>
                {#each ['Provider', 'Requests', 'Tokens', 'Avg Latency'] as h}
                  <th style="text-align: left; padding: 10px 14px; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; color: var(--color-fg-3); border-bottom: 1px solid var(--color-border); background: var(--color-bg-body);">{h}</th>
                {/each}
              </tr>
            </thead>
            <tbody>
              {#each providerBreakdown as p, i}
                <tr style="border-bottom: 1px solid var(--color-border-light);">
                  <td style="padding: 10px 14px;">
                    <div class="flex items-center gap-2">
                      <span class="w-2 h-2 rounded-full" style="background: {COLORS[i % COLORS.length]};"></span>
                      <span style="font-weight: 500; color: var(--color-fg-0);">{p.name}</span>
                    </div>
                  </td>
                  <td style="padding: 10px 14px; font-family: var(--font-mono); font-size: 12px;">{p.requests.toLocaleString()}</td>
                  <td style="padding: 10px 14px; font-family: var(--font-mono); font-size: 12px;">{p.tokens.toLocaleString()}</td>
                  <td style="padding: 10px 14px; font-family: var(--font-mono); font-size: 12px;">{formatLatency(p.avgLatency)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}

    <!-- Model breakdown -->
    {#if modelBreakdown.length > 0}
      <div class="card">
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 16px;">
          <Cpu size={16} style="display: inline; vertical-align: -3px; margin-right: 6px; color: var(--color-purple);" />
          Requests by Model
        </div>
        <div style="overflow-x: auto;">
          <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
            <thead>
              <tr>
                {#each ['Model', 'Requests', 'Tokens In', 'Tokens Out'] as h}
                  <th style="text-align: left; padding: 10px 14px; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; color: var(--color-fg-3); border-bottom: 1px solid var(--color-border); background: var(--color-bg-body);">{h}</th>
                {/each}
              </tr>
            </thead>
            <tbody>
              {#each modelBreakdown as m, i}
                <tr style="border-bottom: 1px solid var(--color-border-light);">
                  <td style="padding: 10px 14px; font-weight: 500; color: var(--color-fg-0); font-size: 12px; font-family: var(--font-mono);">{m.name}</td>
                  <td style="padding: 10px 14px; font-family: var(--font-mono); font-size: 12px;">{m.requests.toLocaleString()}</td>
                  <td style="padding: 10px 14px; font-family: var(--font-mono); font-size: 12px;">{m.inputTokens.toLocaleString()}</td>
                  <td style="padding: 10px 14px; font-family: var(--font-mono); font-size: 12px;">{m.outputTokens.toLocaleString()}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}
  {/if}
</div>
