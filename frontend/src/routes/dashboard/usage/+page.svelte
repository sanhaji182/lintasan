<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { BarChart3, ChartColumn, DollarSign, Database, Server, RefreshCw } from 'lucide-svelte/icons';

  let data = $state<any>(null);
  let quotaData = $state<any[]>([]);
  let loading = $state(true);
  let canvasEl: HTMLCanvasElement = $state()!;

  const COLORS = ['#3c50e0', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899', '#06b6d4'];

  onMount(async () => {
    try {
      const [usageRes, quotaRes] = await Promise.all([
        api.get<any>('/api/usage'),
        api.get<any>('/api/quota/stats').catch(() => null)
      ]);
      data = usageRes?.data || usageRes;
      quotaData = quotaRes?.data || [];
    } catch {}
    finally { loading = false; }
  });

  $effect(() => {
    if (data?.daily && canvasEl) drawChart();
  });

  function drawChart() {
    const ctx = canvasEl.getContext('2d');
    if (!ctx || !data?.daily) return;
    const dpr = window.devicePixelRatio || 1;
    const w = canvasEl.clientWidth;
    const h = 220;
    canvasEl.width = w * dpr;
    canvasEl.height = h * dpr;
    canvasEl.style.height = h + 'px';
    ctx.scale(dpr, dpr);

    ctx.clearRect(0, 0, w, h);
    const daily = data.daily.slice(-14);
    if (!daily.length) return;

    const maxVal = Math.max(...daily.map((d: any) => d.tokens || 0), 1);
    const barW = Math.max((w - 40) / daily.length - 6, 12);
    const padX = 20;

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
      const barH = ((d.tokens || 0) / maxVal) * (h - 50);
      const x = padX + i * ((w - 2 * padX) / daily.length) + 3;
      const y = h - 30 - barH;

      const grad = ctx.createLinearGradient(x, y, x, h - 30);
      grad.addColorStop(0, COLORS[0]);
      grad.addColorStop(1, COLORS[1]);
      ctx.fillStyle = grad;
      ctx.beginPath();
      ctx.roundRect(x, y, barW, barH, [3, 3, 0, 0]);
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

<div style="animation: fadeInUp 0.4s ease-out;">
  <h2 style="font-size: 18px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 20px;">Usage</h2>

  {#if loading}
    <Spinner />
  {:else if !data}
    <div class="card"><EmptyState icon={ChartColumn} title="No usage data" /></div>
  {:else}
    <!-- Summary stats -->
    <div class="card mb-5" style="padding: 0; overflow: hidden;">
      <div class="grid grid-cols-3" style="gap: 1px; background: var(--color-border);">
        {#each [
          { label: 'TOTAL TOKENS', value: (data.providers || []).reduce((s: number, p: any) => s + (p.tokens || 0), 0).toLocaleString() },
          { label: 'PROVIDERS', value: (data.providers || []).length },
          { label: 'MODELS', value: (data.models || []).length }
        ] as stat}
          <div class="text-center" style="padding: 16px; background: var(--color-bg-card);">
            <div style="font-size: 11px; font-weight: 500; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">{stat.label}</div>
            <div style="font-size: 20px; font-weight: 700; color: var(--color-fg-0); font-family: var(--font-mono);">{stat.value}</div>
          </div>
        {/each}
      </div>
    </div>

    <!-- Chart -->
    <div class="card mb-5">
      <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 16px;">Daily Token Usage</div>
      <canvas bind:this={canvasEl} style="width: 100%;"></canvas>
    </div>

    <!-- Provider breakdown -->
    {#if data.providers?.length}
      <div class="card mb-5">
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 16px;">Provider Breakdown</div>
        <div style="overflow-x: auto;">
          <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
            <thead>
              <tr>
                {#each ['Provider', 'Requests', 'Tokens'] as h}
                  <th style="text-align: left; padding: 12px 14px; font-size: 11px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.5px; color: var(--color-fg-3); border-bottom: 1px solid var(--color-border);">{h}</th>
                {/each}
              </tr>
            </thead>
            <tbody>
              {#each data.providers as p, i}
                <tr style="border-bottom: 1px solid var(--color-border-light);">
                  <td style="padding: 12px 14px;">
                    <div class="flex items-center gap-2">
                      <span class="w-2 h-2 rounded-full" style="background: {COLORS[i % COLORS.length]};"></span>
                      <span style="font-weight: 500; color: var(--color-fg-0);">{p.provider || p.name}</span>
                    </div>
                  </td>
                  <td style="padding: 12px 14px; font-family: var(--font-mono); font-size: 12px;">{p.requests?.toLocaleString() || 0}</td>
                  <td style="padding: 12px 14px; font-family: var(--font-mono); font-size: 12px;">{p.tokens?.toLocaleString() || 0}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}

    <!-- Model breakdown -->
    {#if data.models?.length}
      <div class="card mb-5">
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 16px;">Model Breakdown</div>
        <div style="overflow-x: auto;">
          <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
            <thead>
              <tr>
                {#each ['Model', 'Requests', 'Tokens'] as h}
                  <th style="text-align: left; padding: 12px 14px; font-size: 11px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.5px; color: var(--color-fg-3); border-bottom: 1px solid var(--color-border);">{h}</th>
                {/each}
              </tr>
            </thead>
            <tbody>
              {#each data.models as m}
                <tr style="border-bottom: 1px solid var(--color-border-light);">
                  <td style="padding: 12px 14px; font-weight: 500; color: var(--color-fg-0); font-size: 12px;">{m.model}</td>
                  <td style="padding: 12px 14px; font-family: var(--font-mono); font-size: 12px;">{m.requests?.toLocaleString() || 0}</td>
                  <td style="padding: 12px 14px; font-family: var(--font-mono); font-size: 12px;">{(m.tokens || 0).toLocaleString()}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}

    <!-- Quota Tracking -->
    {#if quotaData?.length}
      <div class="card mb-5">
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 16px;">Quota Tracking</div>
        <div style="overflow-x: auto;">
          <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
            <thead>
              <tr>
                {#each ['Provider', 'Input Tokens', 'Output Tokens', 'Cost Est.', 'Remaining', 'Next Reset'] as h}
                  <th style="text-align: left; padding: 12px 14px; font-size: 11px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.5px; color: var(--color-fg-3); border-bottom: 1px solid var(--color-border);">{h}</th>
                {/each}
              </tr>
            </thead>
            <tbody>
              {#each quotaData as q}
                <tr style="border-bottom: 1px solid var(--color-border-light);">
                  <td style="padding: 12px 14px; font-weight: 500; color: var(--color-fg-0);">{q.provider}</td>
                  <td style="padding: 12px 14px; font-family: var(--font-mono); font-size: 12px;">{(q.input_tokens || 0).toLocaleString()}</td>
                  <td style="padding: 12px 14px; font-family: var(--font-mono); font-size: 12px;">{(q.output_tokens || 0).toLocaleString()}</td>
                  <td style="padding: 12px 14px; font-family: var(--font-mono); font-size: 12px;">${(q.cost_estimate || 0).toFixed(4)}</td>
                  <td style="padding: 12px 14px;">
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
                  <td style="padding: 12px 14px; font-size: 12px; color: var(--color-fg-3);">
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
