<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { deriveOverview, type OverviewState } from '$lib/overview-state';
  import { Activity, ArrowRight, CheckCircle2, CircleAlert, Clock3, Gauge, Link2, RefreshCw, Route, Sparkles, Zap } from 'lucide-svelte';

  let loading = $state(true);
  let refreshing = $state(false);
  let sources = $state<any>(null);

  function unwrap<T>(result: PromiseSettledResult<any>, fallbackKey = 'data'): PromiseSettledResult<T> {
    if (result.status === 'rejected') return result;
    return { status: 'fulfilled', value: result.value?.[fallbackKey] ?? result.value };
  }

  async function load() {
    const results = await Promise.allSettled([
      fetch('/health').then(async response => { if (!response.ok) throw new Error('Health unavailable'); return response.json(); }),
      api.get('/api/dashboard/stats'),
      api.get('/api/logs'),
      api.get('/api/connections'),
    ]);
    sources = {
      health: unwrap(results[0], 'data'),
      stats: unwrap(results[1], 'data'),
      logs: unwrap(results[2], 'data'),
      connections: unwrap(results[3], 'data'),
    };
    loading = false;
    refreshing = false;
  }

  async function refresh() { refreshing = true; await load(); }
  onMount(load);

  const overview: OverviewState | null = $derived(sources ? deriveOverview(sources) : null);
  const metricCards = $derived(overview ? [
    { label: 'Requests', ...overview.requests, icon: Activity, tone: 'blue' },
    { label: 'Average latency', ...overview.latency, icon: Clock3, tone: 'amber' },
    { label: 'Recent tokens', ...overview.tokens, icon: Zap, tone: 'violet' },
    { label: 'Provider accounts', ...overview.providers, icon: Link2, tone: 'green' },
  ] : []);

  function formatTime(value?: string) {
    if (!value) return 'Time unavailable';
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : new Intl.RelativeTimeFormat('en', { numeric: 'auto' }).format(-Math.max(0, Math.round((Date.now() - date.getTime()) / 60000)), 'minute');
  }
</script>

<svelte:head><title>Command Center — Lintasan</title></svelte:head>

<section class="overview-shell" aria-labelledby="overview-title">
  <header class="hero">
    <div>
      <div class="eyebrow"><Sparkles size={14} /> Command Center</div>
      <h2 id="overview-title">Know what needs attention.</h2>
      <p>Live gateway signals, provider readiness, and the shortest path to your next action.</p>
    </div>
    <button class="btn-secondary refresh" onclick={refresh} disabled={loading || refreshing}>
      <span class:spinning={refreshing}><RefreshCw size={15} /></span> Refresh
    </button>
  </header>

  {#if loading}
    <div class="hero-status skeleton-block" aria-label="Loading gateway status"></div>
    <div class="metric-grid" aria-label="Loading metrics">{#each Array(4) as _}<div class="metric skeleton-block"></div>{/each}</div>
  {:else if overview}
    <article class="hero-status" class:attention={overview.gateway.tone !== 'success'}>
      <div class="status-icon">{#if overview.gateway.tone === 'success'}<CheckCircle2 size={22} />{:else}<CircleAlert size={22} />{/if}</div>
      <div><span class="status-kicker">Gateway status</span><strong>{overview.gateway.label}</strong><small>{overview.gateway.detail}</small></div>
      <a href="/dashboard/analytics">Open observability <ArrowRight size={14} /></a>
    </article>

    {#if overview.failedSources.length}
      <div class="source-warning" role="alert"><CircleAlert size={17} /><span><strong>Some live data is unavailable.</strong> {overview.failedSources.join(', ')} could not be loaded. Available panels remain current.</span></div>
    {/if}

    <div class="metric-grid">
      {#each metricCards as metric}
        {@const Icon = metric.icon}
        <article class="metric" data-tone={metric.tone}>
          <div class="metric-icon"><Icon size={18} /></div>
          <span>{metric.label}</span><strong>{metric.value}</strong><small>{metric.detail}</small>
        </article>
      {/each}
    </div>

    <div class="workspace-grid">
      <section class="panel activity-panel">
        <header><div><span class="section-kicker">Traffic</span><h3>Recent requests</h3></div><a href="/dashboard/analytics">View analytics <ArrowRight size={14} /></a></header>
        {#if overview.recentRequests.length}
          <div class="request-list">
            {#each overview.recentRequests.slice(0, 7) as request}
              <div class="request-row">
                <span class:failed={request.status != null && (request.status < 200 || request.status >= 300)} class="request-dot"></span>
                <div><strong>{request.model || 'Unknown model'}</strong><small>{request.provider || 'Provider unavailable'}</small></div>
                <span class="request-meta">{request.latency_ms != null ? `${request.latency_ms}ms` : 'Latency unavailable'}</span>
                <span class="request-time">{formatTime(request.created_at)}</span>
              </div>
            {/each}
          </div>
        {:else}
          <div class="empty"><Activity size={24} /><strong>No recorded requests</strong><span>Send a request from Playground or your application to see activity here.</span><a href="/dashboard/playground">Open Playground</a></div>
        {/if}
      </section>

      <aside class="right-rail">
        <section class="panel action-panel">
          <span class="section-kicker">Next steps</span><h3>Build your gateway</h3>
          <a class="quick-action" href="/dashboard/connections"><span><Link2 size={17} /><b>Manage connections</b><small>Add, test, and organize provider accounts.</small></span><ArrowRight size={15} /></a>
          <a class="quick-action" href="/dashboard/routing"><span><Route size={17} /><b>Shape routing</b><small>Control priorities, combos, and fallback.</small></span><ArrowRight size={15} /></a>
          <a class="quick-action" href="/dashboard/quickstart"><span><Gauge size={17} /><b>Open Quickstart</b><small>Connect a client with verified values.</small></span><ArrowRight size={15} /></a>
        </section>

        <section class="panel provider-panel">
          <header><div><span class="section-kicker">Providers</span><h3>Account readiness</h3></div><a href="/dashboard/connections">Manage</a></header>
          {#if overview.connections.length}
            {#each overview.connections.slice(0, 5) as connection}
              <div class="provider-row"><span class:active={connection.is_active} class="provider-dot"></span><div><strong>{connection.name}</strong><small>{connection.format || 'Format unavailable'}</small></div><span>{connection.is_active ? 'Active' : 'Inactive'}</span></div>
            {/each}
          {:else}
            <div class="empty compact"><Link2 size={22} /><strong>No provider accounts</strong><a href="/dashboard/connections">Add a connection</a></div>
          {/if}
        </section>
      </aside>
    </div>
  {/if}
</section>

<style>
  .overview-shell { max-width:1440px; margin:0 auto; }
  .hero { display:flex; align-items:flex-end; justify-content:space-between; gap:24px; margin-bottom:24px; }
  .eyebrow,.section-kicker,.status-kicker { display:flex; align-items:center; gap:6px; color:var(--color-primary); font-size:11px; font-weight:700; letter-spacing:.08em; text-transform:uppercase; }
  h2 { margin:7px 0 4px; color:var(--color-fg-0); font-size:clamp(26px,3vw,36px); font-weight:650; letter-spacing:-.04em; line-height:1.1; }
  .hero p { margin:0; color:var(--color-fg-2); font-size:14px; }
  .refresh { min-height:40px; display:flex; align-items:center; gap:7px; }
  .spinning { animation:spin .8s linear infinite; }
  .hero-status { display:grid; grid-template-columns:auto 1fr auto; align-items:center; gap:14px; min-height:98px; margin-bottom:16px; padding:20px; border:1px solid color-mix(in srgb,var(--color-success) 24%,var(--color-border)); border-radius:var(--radius-lg); background:linear-gradient(120deg,var(--color-success-light),var(--color-bg-card) 62%); }
  .hero-status.attention { border-color:var(--color-border); background:var(--color-bg-card); }
  .status-icon { width:46px; height:46px; display:grid; place-items:center; border-radius:13px; color:var(--color-success); background:var(--color-bg-card); border:1px solid var(--color-border); }
  .attention .status-icon { color:var(--color-warning); }
  .hero-status strong,.hero-status small { display:block; }.hero-status strong { margin-top:2px; font-size:20px; }.hero-status small { color:var(--color-fg-2); font-size:12px; }
  .hero-status a,.panel header a,.empty a { display:inline-flex; align-items:center; gap:5px; color:var(--color-primary); font-size:12px; font-weight:650; text-decoration:none; }
  .source-warning { display:flex; gap:10px; margin-bottom:16px; padding:12px 14px; border:1px solid color-mix(in srgb,var(--color-warning) 28%,var(--color-border)); border-radius:10px; color:var(--color-fg-1); background:var(--color-warning-light); font-size:12px; }
  .metric-grid { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:12px; margin-bottom:16px; }
  .metric { min-height:146px; padding:17px; border:1px solid var(--color-border); border-radius:var(--radius); background:var(--color-bg-card); box-shadow:var(--shadow-sm); }
  .metric-icon { width:36px;height:36px;display:grid;place-items:center;margin-bottom:15px;border-radius:10px;color:var(--tone,var(--color-primary));background:color-mix(in srgb,var(--tone,var(--color-primary)) 10%,transparent); }
  .metric[data-tone="amber"]{--tone:var(--color-warning)}.metric[data-tone="violet"]{--tone:var(--color-purple)}.metric[data-tone="green"]{--tone:var(--color-success)}
  .metric>span,.metric>small { display:block;color:var(--color-fg-3);font-size:11px; }.metric>strong{display:block;margin:3px 0;font:650 24px var(--font-sans);letter-spacing:-.03em}.metric>small{color:var(--color-fg-2)}
  .workspace-grid { display:grid; grid-template-columns:minmax(0,1.6fr) minmax(300px,.75fr); gap:16px; }
  .right-rail { display:grid; gap:16px; align-content:start; }.panel { overflow:hidden; border:1px solid var(--color-border); border-radius:var(--radius); background:var(--color-bg-card); box-shadow:var(--shadow-sm); }.panel>header,.action-panel { padding:18px; }.panel>header{display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid var(--color-border-light)}
  h3 { margin:2px 0 0; font-size:14px; font-weight:650; }.request-row { display:grid; grid-template-columns:auto minmax(0,1fr) auto auto; align-items:center; gap:11px; min-height:58px; padding:9px 18px; border-bottom:1px solid var(--color-border-light); }.request-row:last-child{border-bottom:0}.request-row strong,.provider-row strong{display:block;font-size:12px}.request-row small,.provider-row small{display:block;color:var(--color-fg-3);font-size:11px}.request-dot,.provider-dot{width:8px;height:8px;border-radius:50%;background:var(--color-success)}.request-dot.failed,.provider-dot:not(.active){background:var(--color-error)}.request-meta{font:11px var(--font-mono);color:var(--color-fg-2)}.request-time{color:var(--color-fg-3);font-size:11px}
  .action-panel h3 { margin-bottom:12px; }.quick-action { display:flex;align-items:center;justify-content:space-between;gap:10px;padding:12px 0;border-top:1px solid var(--color-border-light);color:var(--color-fg-1);text-decoration:none}.quick-action>span{display:grid;grid-template-columns:auto 1fr;column-gap:9px}.quick-action svg{grid-row:1/3;color:var(--color-primary)}.quick-action b{font-size:12px}.quick-action small{color:var(--color-fg-3);font-size:11px}.provider-row{display:flex;align-items:center;gap:10px;padding:11px 18px;border-bottom:1px solid var(--color-border-light)}.provider-row>div{flex:1}.provider-row>span:last-child{color:var(--color-fg-3);font-size:11px}
  .empty{min-height:220px;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:7px;padding:24px;color:var(--color-fg-3);text-align:center}.empty span{max-width:340px;font-size:12px}.empty strong{color:var(--color-fg-1);font-size:13px}.empty.compact{min-height:150px}.skeleton-block{min-height:98px;background:var(--color-border-light);animation:shimmer 1.4s infinite}
  @media(max-width:1050px){.metric-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.workspace-grid{grid-template-columns:1fr}}
  @media(max-width:600px){.hero{align-items:flex-start}.hero p{max-width:280px}.refresh{width:44px;min-height:44px;flex-shrink:0;padding:0;justify-content:center;font-size:0}.hero-status{grid-template-columns:auto 1fr}.hero-status>a{grid-column:2}.metric-grid{grid-template-columns:1fr 1fr}.metric{min-height:130px}.request-row{grid-template-columns:auto minmax(0,1fr) auto}.request-time{display:none}}
</style>
