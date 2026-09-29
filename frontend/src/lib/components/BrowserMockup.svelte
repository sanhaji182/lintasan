<script lang="ts">
  import {
    LayoutDashboard, Activity, BarChart3, Settings,
    CircleCheck, TriangleAlert, CircleX, ArrowUpRight, RefreshCw
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

  // The metric tiles describe this Lintasan instance, so they must come from
  // live /health values rather than invented numbers. Until the caller has
  // them, show the real "unknown" state instead of a fake 0.
  let live = $derived(models > 0 && providers > 0);

  const statusTone = $derived(
    status.toLowerCase() === 'ok' ? 'live' : 'degraded'
  );

  const tabs = $derived<{ key: TabKey; label: string; icon: typeof LayoutDashboard }[]>([
    { key: 'overview', label: 'Overview', icon: LayoutDashboard },
    { key: 'activity', label: 'Activity', icon: Activity },
    { key: 'reports', label: 'Reports', icon: BarChart3 },
    { key: 'settings', label: 'Settings', icon: Settings }
  ]);

  let active = $state<TabKey>('overview');

  // Uptime is Go duration text like "43h47m29s". Show it verbatim in the
  // Overview tab; the other tabs have no live source, so they say so.
  const uptimeLabel = $derived(uptime ? uptime.replace(/\.\d+s$/, 's') : '—');
</script>

<!-- Browser window mockup. Purely presentational: the tabs switch panels in
     place, and every number shown is a live value the page already fetched. -->
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

      <span class="shrink-0 rounded-full border border-slate-200 bg-white px-2.5 py-1 text-[10px] font-bold tracking-wide text-slate-500">
        Live Demo
      </span>
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
        <span class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide"
              style="background: {statusTone === 'live' ? 'var(--color-success-light)' : 'var(--color-warning-light)'};
                     color: {statusTone === 'live' ? 'var(--color-success)' : 'var(--color-warning)'};">
          <span class="size-1.5 rounded-full" style="background: currentColor"></span>
          {statusTone === 'live' ? 'Active' : 'Degraded'}
        </span>
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
      <nav class="flex shrink-0 gap-1 overflow-x-auto border-b border-slate-200 bg-slate-50/60 p-2 sm:w-48 sm:flex-col sm:overflow-visible sm:border-b-0 sm:border-r">
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
            <div class="rounded-xl border border-slate-200 bg-white p-3">
              <p class="text-[10px] font-bold uppercase tracking-wide text-slate-400">Gateway</p>
              <p class="mt-1.5 text-xl font-extrabold tracking-tight text-slate-900"
                 style="color: {statusTone === 'live' ? 'var(--color-success)' : 'var(--color-warning)'}">
                {status}
              </p>
              <p class="mt-0.5 text-[10px] text-slate-500">uptime {uptimeLabel}</p>
            </div>

            <div class="rounded-xl border border-slate-200 bg-white p-3">
              <p class="text-[10px] font-bold uppercase tracking-wide text-slate-400">Models</p>
              <p class="mt-1.5 text-xl font-extrabold tracking-tight text-slate-900">{live ? models : '—'}</p>
              <p class="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-100">
                <span class="block h-full rounded-full" style="width:100%; background: var(--color-primary);"></span>
              </p>
            </div>

            <div class="rounded-xl border border-slate-200 bg-white p-3">
              <p class="text-[10px] font-bold uppercase tracking-wide text-slate-400">Providers</p>
              <p class="mt-1.5 text-xl font-extrabold tracking-tight text-slate-900">{live ? providers : '—'}</p>
              <p class="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-100">
                <span class="block h-full rounded-full" style="width:100%; background: var(--color-success);"></span>
              </p>
            </div>
          </div>

          <!-- Detail summary -->
          <div class="mt-3 rounded-xl border border-slate-200 bg-slate-50/50 p-3">
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
                <dd class="mt-0.5 text-[11px] text-slate-700">
                  {statusTone === 'live' ? 'Serving traffic' : 'Check logs'}
                </dd>
              </div>
            </dl>
          </div>

          <!-- What this tab can and cannot show -->
          <div class="mt-3 flex items-start gap-2 rounded-xl border border-slate-200 p-3">
            <CircleCheck size={15} class="mt-0.5 shrink-0" style="color: var(--color-success);" />
            <p class="text-[11px] leading-relaxed text-slate-500">
              Angka di atas dibaca langsung dari <code class="font-mono">/health</code> instance ini.
              Buka aplikasi untuk request log, cache hit rate, dan cost tracking lengkap.
            </p>
          </div>
        {:else if active === 'activity'}
          <div class="flex min-h-52 flex-col items-center justify-center rounded-xl border border-dashed border-slate-300 p-6 text-center">
            <Activity size={22} class="mb-2 text-slate-400" />
            <p class="text-[12px] font-semibold text-slate-700">Log request tidak ditampilkan di mockup</p>
            <p class="mt-1 max-w-sm text-[11px] leading-relaxed text-slate-500">
              Request log berisi data tenant nyata, jadi tidak di-hardcode di halaman publik.
            </p>
            <a href="/dashboard/logs" class="mt-3 inline-flex items-center gap-1.5 text-[11px] font-bold" style="color: var(--color-primary);">
              Buka log lengkap <ArrowUpRight size={12} />
            </a>
          </div>
        {:else if active === 'reports'}
          <div class="flex min-h-52 flex-col items-center justify-center rounded-xl border border-dashed border-slate-300 p-6 text-center">
            <BarChart3 size={22} class="mb-2 text-slate-400" />
            <p class="text-[12px] font-semibold text-slate-700">Laporan token & cost ada di dalam aplikasi</p>
            <p class="mt-1 max-w-sm text-[11px] leading-relaxed text-slate-500">
              Angka laporan bergantung periode dan tenant, jadi tidak ditampilkan sebagai angka statis di sini.
            </p>
            <a href="/dashboard/reports" class="mt-3 inline-flex items-center gap-1.5 text-[11px] font-bold" style="color: var(--color-primary);">
              Buka laporan <ArrowUpRight size={12} />
            </a>
          </div>
        {:else}
          <div class="flex min-h-52 flex-col items-center justify-center rounded-xl border border-dashed border-slate-300 p-6 text-center">
            <Settings size={22} class="mb-2 text-slate-400" />
            <p class="text-[12px] font-semibold text-slate-700">Konfigurasi tidak ditampilkan di mockup</p>
            <p class="mt-1 max-w-sm text-[11px] leading-relaxed text-slate-500">
              Halaman ini hanya interaktif untuk navigasi. Pengaturan asli ada di dashboard.
            </p>
            <a href="/dashboard/settings" class="mt-3 inline-flex items-center gap-1.5 text-[11px] font-bold" style="color: var(--color-primary);">
              Buka pengaturan <ArrowUpRight size={12} />
            </a>
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>
