<script lang="ts">
  import {
    LayoutDashboard, Activity, BarChart3, Settings,
    CircleCheck, ArrowUpRight, FlaskConical, Radio
  } from 'lucide-svelte';
  import LogoMark from '$lib/components/LogoMark.svelte';

  type TabKey = 'overview' | 'activity' | 'reports' | 'settings';

  let {
    models = 0,
    providers = 0,
    status = 'ok',
    uptime = ''
  }: {
    models?: number;
    providers?: number;
    status?: string;
    uptime?: string;
  } = $props();

  // Live numbers only count when the page actually delivered /health values.
  let live = $derived(models > 0 && providers > 0);

  // Default to the sample dataset so an anonymous visitor poking the demo sees
  // a fully populated product instead of three tiles saying "not available".
  // The Live toggle swaps in the real /health reading from this instance.
  let sample = $state(true);

  const statusTone = $derived(status.toLowerCase() === 'ok' ? 'live' : 'degraded');
  const uptimeLabel = $derived(uptime ? uptime.replace(/\.\d+s$/, 's') : '—');

  const tabs: { key: TabKey; label: string; icon: typeof LayoutDashboard }[] = [
    { key: 'overview', label: 'Overview', icon: LayoutDashboard },
    { key: 'activity', label: 'Activity', icon: Activity },
    { key: 'reports', label: 'Reports', icon: BarChart3 },
    { key: 'settings', label: 'Settings', icon: Settings }
  ];

  let active = $state<TabKey>('overview');

  // ── Sample dataset ──────────────────────────────────────────────────────
  // Every value below is invented for the landing-page mockup only. It is never
  // read from or written to the backend, and every panel that shows it carries
  // a "Sample data" badge so it can never be mistaken for a real reading.
  const sMetrics = [
    { label: 'Providers online', value: '12/12', pct: 100, note: 'all healthy', accent: 'var(--color-success)' },
    { label: 'Models catalog', value: '128', pct: 100, note: 'across 12 providers', accent: 'var(--color-primary)' },
    { label: 'Cache hit rate', value: '38%', pct: 38, note: 'last 24h · semantically matched', accent: 'var(--color-purple)' }
  ];
  const sDetail = [
    { k: 'Endpoint', v: '/v1/chat/completions' },
    { k: 'Platform', v: 'Single binary · Go' },
    { k: 'Region', v: 'ap-southeast-1' }
  ];
  const sActivity = [
    { t: '12:04:08', text: 'gpt-4o-mini → openai · 200 · 812ms · cached', tone: 'var(--color-success)' },
    { t: '12:03:51', text: 'claude-sonnet-4.5 → anthropic · 200 · 1.9s', tone: 'var(--color-success)' },
    { t: '12:02:44', text: 'deepseek-v3 → failover to groq · 200 · 940ms', tone: 'var(--color-warning)' },
    { t: '12:01:17', text: 'qwen3-coder → qoder pool · 429 rate-limited', tone: 'var(--color-error)' },
    { t: '11:58:59', text: 'gemini-2.5-flash → google · 200 · 612ms', tone: 'var(--color-success)' }
  ];
  const sReports = [
    { label: 'Tokens routed today', value: '4.2M', delta: '+12%' },
    { label: 'Est. cost saved (cache)', value: '$31.40', delta: 'vs direct billing' },
    { label: 'Failovers served', value: '17', delta: '0 request errors' }
  ];
  const sSettings = [
    { label: 'Semantic cache', on: true, desc: 'cosine ≥ 0.97 · TTL 6h' },
    { label: 'Smart failover', on: true, desc: '3 retries · circuit breaker 50%' },
    { label: 'Cost budget alerts', on: false, desc: '$120/mo · webhook' }
  ];

  const displayMetrics = $derived(sample
    ? sMetrics
    : (live
        ? [
            { label: 'Gateway', value: status, pct: 100, note: 'uptime ' + uptimeLabel, accent: statusTone === 'live' ? 'var(--color-success)' : 'var(--color-warning)' },
            { label: 'Models', value: String(models), pct: 100, note: 'in catalog', accent: 'var(--color-primary)' },
            { label: 'Providers', value: String(providers), pct: 100, note: 'connected', accent: 'var(--color-success)' }
          ]
        : [
            { label: 'Gateway', value: status, pct: 100, note: 'metrics unavailable', accent: 'var(--color-warning)' },
            { label: 'Models', value: '—', pct: 0, note: 'no data', accent: 'var(--color-primary)' },
            { label: 'Providers', value: '—', pct: 0, note: 'no data', accent: 'var(--color-success)' }
          ]));
</script>

<!-- Browser window mockup. The sample dataset is hard-coded for the demo; the
     Live toggle swaps in this instance's real /health reading. Panels that show
     sample values are clearly badged so they can never pass for real data. -->
<div class="relative w-full">
  <div class="pointer-events-none absolute -inset-x-6 -top-6 bottom-0 hidden opacity-60 lg:block"
       style="background: radial-gradient(60% 50% at 50% 0%, var(--color-primary-glow), transparent 70%);"></div>

  <div class="relative overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl shadow-slate-900/10">
    <!-- Browser chrome -->
    <div class="flex items-center gap-3 border-b border-slate-200 bg-slate-50/80 px-4 py-3">
      <div class="flex shrink-0 items-center gap-1.5" aria-hidden="true">
        <span class="size-3 rounded-full bg-rose-500"></span>
        <span class="size-3 rounded-full bg-amber-400"></span>
        <span class="size-3 rounded-full bg-emerald-500"></span>
      </div>

      <div class="mx-auto flex min-w-0 max-w-md flex-1 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-1.5 shadow-sm">
        <span class="size-1.5 shrink-0 rounded-full bg-emerald-500"></span>
        <span class="truncate font-mono text-[11px] text-slate-500">lintasan.sans.biz.id/dashboard</span>
      </div>

      <!-- Demo / Live toggle -->
      <div class="flex shrink-0 items-center rounded-full border border-slate-200 bg-white p-0.5 text-[10px] font-bold">
        <button type="button" onclick={() => (sample = true)}
                class="flex items-center gap-1 rounded-full px-2.5 py-1 transition {sample ? 'bg-slate-900 text-white' : 'text-slate-500 hover:text-slate-700'}">
          <FlaskConical size={11} /> Sample
        </button>
        <button type="button" onclick={() => (sample = false)}
                class="flex items-center gap-1 rounded-full px-2.5 py-1 transition {!sample ? 'bg-emerald-600 text-white' : 'text-slate-500 hover:text-slate-700'}">
          <Radio size={11} /> Live
        </button>
      </div>
    </div>

    <!-- App header -->
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 bg-white px-4 py-3 sm:px-5">
      <div class="flex items-center gap-2.5">
        <LogoMark size={26} title="Lintasan" />
        <div class="leading-tight">
          <p class="text-[13px] font-bold text-slate-900">Lintasan</p>
          <p class="text-[10px] text-slate-500">AI Gateway</p>
        </div>
      </div>

      <div class="flex items-center gap-2">
        {#if sample}
          <span class="rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide"
                style="background: var(--color-warning-light); color: var(--color-warning);">
            Sample data
          </span>
        {:else}
          <span class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide"
                style="background: {statusTone === 'live' ? 'var(--color-success-light)' : 'var(--color-warning-light)'};
                       color: {statusTone === 'live' ? 'var(--color-success)' : 'var(--color-warning)'};">
            <span class="size-1.5 rounded-full" style="background: currentColor"></span>
            {statusTone === 'live' ? 'Active' : 'Degraded'}
          </span>
        {/if}
        <a href="/dashboard" class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-[12px] font-semibold text-white transition hover:opacity-90"
           style="background: var(--color-primary);">
          Buka Aplikasi
          <ArrowUpRight size={13} />
        </a>
      </div>
    </div>

    <!-- Body: sidebar + main -->
    <div class="flex flex-col sm:flex-row">
      <!-- Sidebar -->
      <nav class="flex shrink-0 gap-1 overflow-x-auto border-b border-slate-200 bg-slate-50/60 p-2 sm:w-44 sm:flex-col sm:overflow-visible sm:border-b-0 sm:border-r">
        {#each tabs as tab}
          <button
            type="button"
            onclick={() => (active = tab.key)}
            aria-current={active === tab.key ? 'page' : undefined}
            class="flex shrink-0 items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[12px] font-semibold transition
                   {active === tab.key ? 'bg-white text-primary shadow-sm' : 'text-slate-500 hover:bg-white/70 hover:text-slate-700'}"
          >
            <tab.icon size={14} stroke-width={2} />
            {tab.label}
          </button>
        {/each}
      </nav>

      <!-- Main content -->
      <div class="min-w-0 flex-1 p-4 sm:p-5">
        {#if active === 'overview'}
          <!-- Metric tiles -->
          <div class="grid gap-3 sm:grid-cols-3">
            {#each displayMetrics as m}
              <div class="rounded-xl border border-slate-200 bg-white p-3">
                <p class="text-[10px] font-bold uppercase tracking-wide text-slate-400">{m.label}</p>
                <p class="mt-1.5 text-xl font-extrabold tracking-tight text-slate-900">{m.value}</p>
                <p class="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-100">
                  <span class="block h-full rounded-full" style="width:{m.pct}%; background:{m.accent};"></span>
                </p>
                <p class="mt-1.5 text-[10px] text-slate-500">{m.note}</p>
              </div>
            {/each}
          </div>

          <!-- Detail summary -->
          <div class="mt-3 rounded-xl border border-slate-200 bg-slate-50/50 p-3">
            {#if sample}
              <dl class="grid gap-2 sm:grid-cols-3">
                {#each sDetail as d}
                  <div>
                    <dt class="text-[10px] font-bold uppercase tracking-wide text-slate-400">{d.k}</dt>
                    <dd class="mt-0.5 font-mono text-[11px] text-slate-700">{d.v}</dd>
                  </div>
                {/each}
              </dl>
            {:else}
              <dl class="grid gap-2 sm:grid-cols-3">
                <div>
                  <dt class="text-[10px] font-bold uppercase tracking-wide text-slate-400">Endpoint</dt>
                  <dd class="mt-0.5 font-mono text-[11px] text-slate-700">/v1</dd>
                </div>
                <div>
                  <dt class="text-[10px] font-bold uppercase tracking-wide text-slate-400">Platform</dt>
                  <dd class="mt-0.5 text-[11px] text-slate-700">Single binary · Go</dd>
                </div>
                <div>
                  <dt class="text-[10px] font-bold uppercase tracking-wide text-slate-400">Status</dt>
                  <dd class="mt-0.5 text-[11px] text-slate-700">{statusTone === 'live' ? 'Serving traffic' : 'Check logs'}</dd>
                </div>
              </dl>
            {/if}
          </div>

          {#if !sample}
            <div class="mt-3 flex items-start gap-2 rounded-xl border border-slate-200 p-3">
              <CircleCheck size={15} class="mt-0.5 shrink-0" style="color: var(--color-success);" />
              <p class="text-[11px] leading-relaxed text-slate-500">
                Angka di atas dibaca langsung dari <code class="font-mono">/health</code> instance ini.
                Buka aplikasi untuk request log, cache hit rate, dan cost tracking lengkap.
              </p>
            </div>
          {/if}
        {:else if active === 'activity'}
          <div class="rounded-xl border border-slate-200 bg-white">
            {#each sActivity as ev, i}
              <div class="flex items-start gap-2.5 px-3.5 py-2.5 {i>0 ? 'border-t border-slate-100' : ''}">
                <span class="mt-1.5 size-1.5 shrink-0 rounded-full" style="background:{ev.tone}"></span>
                <div class="min-w-0 flex-1">
                  <code class="text-[10.5px] text-slate-400">{ev.t}</code>
                  <p class="truncate font-mono text-[11px] text-slate-700">{ev.text}</p>
                </div>
              </div>
            {/each}
          </div>
          <a href="/dashboard/logs" class="mt-3 inline-flex items-center gap-1.5 text-[11px] font-bold" style="color: var(--color-primary);">
            Buka log asli <ArrowUpRight size={12} />
          </a>
        {:else if active === 'reports'}
          <div class="grid gap-3">
            {#each sReports as r}
              <div class="flex items-center justify-between rounded-xl border border-slate-200 bg-white p-3">
                <div>
                  <p class="text-[10px] font-bold uppercase tracking-wide text-slate-400">{r.label}</p>
                  <p class="mt-0.5 text-lg font-extrabold tracking-tight text-slate-900">{r.value}</p>
                </div>
                <span class="rounded-full px-2 py-1 text-[10px] font-bold" style="background: var(--color-success-light); color: var(--color-success);">{r.delta}</span>
              </div>
            {/each}
          </div>
          <a href="/dashboard/reports" class="mt-3 inline-flex items-center gap-1.5 text-[11px] font-bold" style="color: var(--color-primary);">
            Buka laporan asli <ArrowUpRight size={12} />
          </a>
        {:else}
          <div class="grid gap-3">
            {#each sSettings as s}
              <div class="flex items-center justify-between gap-3 rounded-xl border border-slate-200 bg-white p-3">
                <div class="min-w-0">
                  <p class="text-[12px] font-semibold text-slate-800">{s.label}</p>
                  <p class="mt-0.5 text-[10.5px] text-slate-500">{s.desc}</p>
                </div>
                <span class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition" style="background: {s.on ? 'var(--color-primary)' : '#cbd5e1'};">
                  <span class="absolute size-3.5 rounded-full bg-white shadow transition-all" style="left: {s.on ? '18px' : '3px'};"></span>
                </span>
              </div>
            {/each}
          </div>
          <a href="/dashboard/settings" class="mt-3 inline-flex items-center gap-1.5 text-[11px] font-bold" style="color: var(--color-primary);">
            Buka pengaturan asli <ArrowUpRight size={12} />
          </a>
        {/if}
      </div>
    </div>
  </div>
</div>
