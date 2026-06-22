<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import {
    Activity, Search, Database, Cpu, Server, TriangleAlert,
    CheckCircle2, Gauge, RefreshCw, Layers, Zap
  } from 'lucide-svelte';

  // ── Types ──────────────────────────────────────────────────────────────
  interface SearchMetrics {
    calls: number;
    hits: number;
    empty_exits: number;
    rows_scanned: number;
    capped_scans: number;
    max_scan_rows: number;
  }
  interface MemoryStats {
    total_memories: number;
    available: boolean;
    backend: string;
    avg_score?: number;
    search?: SearchMetrics;
  }
  interface ProcStats {
    goroutines: number | null;
    heapAlloc: number | null;
    rss: number | null;
  }
  interface HttpSeries {
    endpoint: string;
    statusClass: string;
    count: number;
    sumSeconds: number;
  }
  // Proxy RESPONSE cache (exact + semantic) hit/miss — distinct from the
  // semantic-SEARCH counters above. null = counters absent from /metrics.
  interface CacheStats {
    hits: number;
    misses: number;
  }

  let loading = $state(true);
  let refreshing = $state(false);
  let memStats = $state<MemoryStats | null>(null);
  let search = $state<SearchMetrics | null>(null);
  let proc = $state<ProcStats>({ goroutines: null, heapAlloc: null, rss: null });
  let httpSeries = $state<HttpSeries[]>([]);
  let cache = $state<CacheStats | null>(null);
  let metricsAvailable = $state(true);
  let lastUpdated = $state<Date | null>(null);
  let timer: ReturnType<typeof setInterval> | null = null;

  // ── Prometheus text parser (minimal) ───────────────────────────────────
  // Parses the exposition format we serve at /metrics. We only need a handful
  // of families, so this stays small. Labels are parsed into a flat object.
  function parseProm(text: string): { name: string; labels: Record<string, string>; value: number }[] {
    const out: { name: string; labels: Record<string, string>; value: number }[] = [];
    for (const raw of text.split('\n')) {
      const line = raw.trim();
      if (!line || line.startsWith('#')) continue;
      const sp = line.lastIndexOf(' ');
      if (sp < 0) continue;
      const metric = line.slice(0, sp);
      const valStr = line.slice(sp + 1);
      const value = valStr === '+Inf' ? Infinity : parseFloat(valStr);
      if (Number.isNaN(value)) continue;
      const brace = metric.indexOf('{');
      let name = metric;
      const labels: Record<string, string> = {};
      if (brace >= 0 && metric.endsWith('}')) {
        name = metric.slice(0, brace);
        const inner = metric.slice(brace + 1, -1);
        // split on commas not inside quotes
        let cur = '', inQ = false;
        const pairs: string[] = [];
        for (const ch of inner) {
          if (ch === '"') inQ = !inQ;
          if (ch === ',' && !inQ) { pairs.push(cur); cur = ''; }
          else cur += ch;
        }
        if (cur) pairs.push(cur);
        for (const p of pairs) {
          const eq = p.indexOf('=');
          if (eq < 0) continue;
          const k = p.slice(0, eq).trim();
          let v = p.slice(eq + 1).trim();
          if (v.startsWith('"') && v.endsWith('"')) v = v.slice(1, -1);
          labels[k] = v;
        }
      }
      out.push({ name, labels, value });
    }
    return out;
  }

  async function loadAll() {
    // /v1/memory/stats already carries the search counters as JSON — primary
    // source so the page works even if /metrics is disabled.
    try {
      memStats = await api.get<MemoryStats>('/v1/memory/stats');
      search = memStats?.search ?? null;
    } catch { memStats = null; search = null; }

    // /metrics for runtime + http families. Unauthenticated (like /health) but
    // we send the token anyway via api conventions; raw text fetch here.
    try {
      const res = await api.raw('/metrics');
      if (!res.ok) { metricsAvailable = false; return; }
      const text = await res.text();
      const samples = parseProm(text);
      metricsAvailable = true;

      const get1 = (n: string) => samples.find(s => s.name === n)?.value ?? null;
      proc = {
        goroutines: get1('lintasan_process_goroutines'),
        heapAlloc: get1('lintasan_process_heap_alloc_bytes'),
        rss: get1('lintasan_process_resident_memory_bytes')
      };

      // Proxy response-cache hit/miss (operational question #3). Only populate
      // when at least one counter is present so the card degrades to N/A
      // rather than showing a misleading 0% when /metrics omits them.
      const cHits = get1('lintasan_cache_hits_total');
      const cMisses = get1('lintasan_cache_misses_total');
      cache = (cHits == null && cMisses == null)
        ? null
        : { hits: cHits ?? 0, misses: cMisses ?? 0 };

      // HTTP families: aggregate requests_total + duration sum/count per series.
      const seriesMap = new Map<string, HttpSeries>();
      for (const s of samples) {
        const ep = s.labels.endpoint, sc = s.labels.status_class;
        if (!ep || !sc) continue;
        const key = ep + '|' + sc;
        const cur = seriesMap.get(key) || { endpoint: ep, statusClass: sc, count: 0, sumSeconds: 0 };
        if (s.name === 'lintasan_http_requests_total') cur.count = s.value;
        if (s.name === 'lintasan_http_request_duration_seconds_sum') cur.sumSeconds = s.value;
        seriesMap.set(key, cur);
      }
      httpSeries = Array.from(seriesMap.values())
        .filter(s => s.count > 0)
        .sort((a, b) => b.count - a.count);

      // If /metrics carried search counters and /v1/memory/stats didn't, fall
      // back to the metrics source so the hot-path card is never empty.
      if (!search) {
        const sm: SearchMetrics = {
          calls: get1('lintasan_memory_search_calls_total') ?? 0,
          hits: get1('lintasan_memory_search_hits_total') ?? 0,
          empty_exits: get1('lintasan_memory_search_empty_exits_total') ?? 0,
          rows_scanned: get1('lintasan_memory_search_scanned_rows_total') ?? 0,
          capped_scans: get1('lintasan_memory_search_capped_total') ?? 0,
          max_scan_rows: get1('lintasan_memory_search_max_scan_rows') ?? 0
        };
        if (sm.calls > 0 || sm.max_scan_rows > 0) search = sm;
      }
    } catch {
      metricsAvailable = false;
    }
    lastUpdated = new Date();
  }

  async function refresh() {
    refreshing = true;
    await loadAll();
    refreshing = false;
  }

  onMount(async () => {
    await loadAll();
    loading = false;
    // Light auto-refresh so the page reflects live counters (read-only).
    timer = setInterval(loadAll, 15000);
  });
  onDestroy(() => { if (timer) clearInterval(timer); });

  // ── Derived hot-path health ────────────────────────────────────────────
  const hitRate = $derived(search && search.calls > 0
    ? Math.round((search.hits / search.calls) * 100) : 0);
  const emptyRate = $derived(search && search.calls > 0
    ? Math.round((search.empty_exits / search.calls) * 100) : 0);
  // avg rows scanned per *scanning* call (calls that weren't empty-exits).
  const scanningCalls = $derived(search ? Math.max(0, search.calls - search.empty_exits) : 0);
  const avgScanned = $derived(search && scanningCalls > 0
    ? Math.round(search.rows_scanned / scanningCalls) : 0);
  const scanLoadPct = $derived(search && search.max_scan_rows > 0
    ? Math.min(100, Math.round((avgScanned / search.max_scan_rows) * 100)) : 0);

  // Proxy response-cache hit rate (operational question #3). Distinct from the
  // semantic-SEARCH "Hit Rate" card (search.hits/search.calls): this measures
  // how often a request was served from the exact/semantic RESPONSE cache
  // instead of going upstream. N/A until at least one cache-eligible request.
  const cacheTotal = $derived(cache ? cache.hits + cache.misses : 0);
  const cacheHitRate = $derived(cache && cacheTotal > 0
    ? Math.round((cache.hits / cacheTotal) * 100) : null);

  // Warning state: the H3-regression early warning the user asked for.
  //  - capped scans > 0  → search is hitting the cap (store grew past safe size)
  //  - avg scanned per call > 70% of cap → trending hot, ANN index worth it
  const warnLevel = $derived.by(() => {
    if (!search) return 'none';
    if (search.capped_scans > 0) return 'critical';
    if (search.max_scan_rows > 0 && avgScanned >= 0.7 * search.max_scan_rows) return 'warn';
    return 'ok';
  });

  function fmtBytes(n: number | null): string {
    if (n == null) return '—';
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1024 * 1024 * 1024) return (n / (1024 * 1024)).toFixed(1) + ' MB';
    return (n / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
  }
  function fmtLatency(s: number, count: number): string {
    if (!count) return '—';
    const ms = (s / count) * 1000;
    if (ms >= 1000) return (ms / 1000).toFixed(2) + 's';
    return ms.toFixed(1) + 'ms';
  }
  function statusColor(sc: string): string {
    if (sc === '2xx') return 'var(--color-success)';
    if (sc === '3xx') return 'var(--color-info)';
    if (sc === '4xx') return 'var(--color-warning)';
    if (sc === '5xx') return 'var(--color-error)';
    return 'var(--color-fg-3)';
  }
</script>

<div style="animation: fadeInUp 0.4s ease-out;">
  <div class="flex items-center justify-between" style="margin-bottom: 20px;">
    <div>
      <h2 style="font-size: 18px; font-weight: 600; color: var(--color-fg-0); display: flex; align-items: center; gap: 8px;">
        <Activity size={20} style="color: var(--color-primary);" /> Observability
      </h2>
      <p style="font-size: 13px; color: var(--color-fg-2); margin-top: 2px;">
        Search hot-path health, process runtime, and HTTP traffic. Scrape-friendly at <code style="font-family: var(--font-mono); font-size: 12px;">/metrics</code>.
      </p>
    </div>
    <button class="btn-secondary flex items-center gap-2" onclick={refresh} disabled={refreshing}>
      <RefreshCw size={15} style={refreshing ? 'animation: spin 0.8s linear infinite;' : ''} />
      {refreshing ? 'Refreshing' : 'Refresh'}
    </button>
  </div>

  {#if loading}
    <div class="flex justify-center" style="padding: 60px 0;"><Spinner /></div>
  {:else}
    <!-- H3 early-warning banner -->
    {#if warnLevel === 'critical'}
      <div class="warn-banner critical">
        <TriangleAlert size={20} />
        <div>
          <div class="wb-title">Semantic search scanning hot — consider an ANN index</div>
          <div class="wb-body">
            {search?.capped_scans.toLocaleString()} search{search?.capped_scans === 1 ? '' : 'es'} hit the
            <strong>MaxScanRows={search?.max_scan_rows.toLocaleString()}</strong> cap. The H3 O(n) scan is
            being throttled — the store has grown past the safe brute-force size. Time to build a real
            approximate-nearest-neighbour index.
          </div>
        </div>
      </div>
    {:else if warnLevel === 'warn'}
      <div class="warn-banner warn">
        <TriangleAlert size={20} />
        <div>
          <div class="wb-title">Search scan load trending up</div>
          <div class="wb-body">
            Average rows scanned per query ({avgScanned.toLocaleString()}) is {scanLoadPct}% of the
            MaxScanRows cap ({search?.max_scan_rows.toLocaleString()}). Watch this — if it keeps climbing,
            the H3 brute-force scan will start hitting the cap.
          </div>
        </div>
      </div>
    {:else if warnLevel === 'ok'}
      <div class="warn-banner ok">
        <CheckCircle2 size={18} />
        <div class="wb-body" style="margin:0;">
          Search hot path is healthy — no capped scans, scan load {scanLoadPct}% of cap. H3 regression not detected.
        </div>
      </div>
    {/if}

    <!-- Search hot-path metric cards -->
    <div style="font-size: 13px; font-weight: 600; color: var(--color-fg-1); margin: 20px 0 12px; display: flex; align-items: center; gap: 6px;">
      <Search size={15} style="color: var(--color-primary);" /> Semantic Search Hot Path (H3)
    </div>
    {#if !search}
      <div class="card" style="padding: 18px; color: var(--color-fg-2); font-size: 13px;">
        No search metrics yet. They populate once a vector memory search runs, or when a memory backend is connected.
      </div>
    {:else}
      <div class="grid gap-4" style="grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); margin-bottom: 12px;">
        {#each [
          { icon: Search, label: 'Search Calls', value: search.calls.toLocaleString(), color: 'var(--color-primary)' },
          { icon: CheckCircle2, label: 'Hit Rate', value: hitRate + '%', color: 'var(--color-success)' },
          { icon: Layers, label: 'Empty-Exit Rate', value: emptyRate + '%', color: 'var(--color-info)' },
          { icon: Database, label: 'Rows Scanned', value: search.rows_scanned.toLocaleString(), color: 'var(--color-purple)' },
          { icon: TriangleAlert, label: 'Capped Scans', value: search.capped_scans.toLocaleString(), color: search.capped_scans > 0 ? 'var(--color-error)' : 'var(--color-fg-3)' },
          { icon: Gauge, label: 'Max Scan Rows', value: search.max_scan_rows === 0 ? '∞' : search.max_scan_rows.toLocaleString(), color: 'var(--color-warning)' }
        ] as m}
          <div class="card" style="padding: 16px; position: relative; overflow: hidden;">
            <div style="position: absolute; top: 0; left: 0; right: 0; height: 3px; background: {m.color};"></div>
            <m.icon size={18} style="color: {m.color}; margin-bottom: 6px;" stroke-width={1.8} />
            <div style="font-size: 19px; font-weight: 700; font-family: var(--font-mono); color: var(--color-fg-0); letter-spacing: -0.3px;">{m.value}</div>
            <div style="font-size: 11px; font-weight: 500; color: var(--color-fg-3); margin-top: 2px;">{m.label}</div>
          </div>
        {/each}
      </div>

      <!-- Scan-load bar -->
      <div class="card" style="margin-bottom: 24px;">
        <div class="flex items-center justify-between" style="margin-bottom: 8px;">
          <span style="font-size: 12px; font-weight: 600; color: var(--color-fg-1);">Avg scan load per query</span>
          <span style="font-size: 12px; font-family: var(--font-mono); color: var(--color-fg-0);">
            {avgScanned.toLocaleString()} / {search.max_scan_rows === 0 ? '∞' : search.max_scan_rows.toLocaleString()} rows
          </span>
        </div>
        <div style="height: 8px; background: var(--color-border); border-radius: 4px; overflow: hidden;">
          <div style="height: 100%; width: {scanLoadPct}%; border-radius: 4px; transition: width 0.4s ease; background: {warnLevel === 'critical' ? 'var(--color-error)' : warnLevel === 'warn' ? 'var(--color-warning)' : 'var(--color-success)'};"></div>
        </div>
        <div style="font-size: 11px; color: var(--color-fg-3); margin-top: 6px;">
          Backend: <strong style="color: var(--color-fg-1);">{(memStats?.backend || 'none').toUpperCase()}</strong>
          · {memStats?.total_memories?.toLocaleString() ?? 0} memories stored
        </div>
      </div>
    {/if}

    <!-- Proxy response cache (exact + semantic) — operational question #3 -->
    <div style="font-size: 13px; font-weight: 600; color: var(--color-fg-1); margin: 0 0 12px; display: flex; align-items: center; gap: 6px;">
      <Zap size={15} style="color: var(--color-warning);" /> Response Cache (Proxy)
    </div>
    <div class="grid gap-4" style="grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); margin-bottom: 24px;">
      {#each [
        { icon: Zap, label: 'Cache Hit Rate', value: cacheHitRate == null ? 'N/A' : cacheHitRate + '%', color: 'var(--color-success)' },
        { icon: CheckCircle2, label: 'Cache Hits', value: cache ? cache.hits.toLocaleString() : 'N/A', color: 'var(--color-primary)' },
        { icon: Layers, label: 'Cache Misses', value: cache ? cache.misses.toLocaleString() : 'N/A', color: 'var(--color-fg-3)' }
      ] as m}
        <div class="card" style="padding: 16px; position: relative; overflow: hidden;">
          <div style="position: absolute; top: 0; left: 0; right: 0; height: 3px; background: {m.color};"></div>
          <m.icon size={18} style="color: {m.color}; margin-bottom: 6px;" stroke-width={1.8} />
          <div style="font-size: 19px; font-weight: 700; font-family: var(--font-mono); color: var(--color-fg-0); letter-spacing: -0.3px;">{m.value}</div>
          <div style="font-size: 11px; font-weight: 500; color: var(--color-fg-3); margin-top: 2px;">{m.label}</div>
        </div>
      {/each}
    </div>
    <div style="font-size: 11px; color: var(--color-fg-3); margin: -16px 0 24px;">
      Exact + semantic RESPONSE cache served vs. upstream. Distinct from the semantic-search Hit Rate above (memory-context lookups).
      {#if cacheHitRate == null}<span> Counters populate once a cache-eligible request runs.</span>{/if}
    </div>

    <!-- Process runtime -->
    <div style="font-size: 13px; font-weight: 600; color: var(--color-fg-1); margin: 0 0 12px; display: flex; align-items: center; gap: 6px;">
      <Cpu size={15} style="color: var(--color-purple);" /> Process Runtime
    </div>
    {#if !metricsAvailable}
      <div class="card" style="padding: 16px; color: var(--color-fg-2); font-size: 13px;">
        <code style="font-family: var(--font-mono);">/metrics</code> is disabled (LINTASAN_METRICS_ENABLED=false) or unreachable. Runtime + HTTP panels are unavailable.
      </div>
    {:else}
      <div class="grid gap-4" style="grid-template-columns: repeat(auto-fit, minmax(170px, 1fr)); margin-bottom: 24px;">
        {#each [
          { icon: Activity, label: 'Goroutines', value: proc.goroutines?.toLocaleString() ?? '—', color: 'var(--color-info)' },
          { icon: Database, label: 'Heap Alloc', value: fmtBytes(proc.heapAlloc), color: 'var(--color-success)' },
          { icon: Server, label: 'Resident (RSS)', value: fmtBytes(proc.rss), color: 'var(--color-purple)' }
        ] as m}
          <div class="card" style="padding: 16px; position: relative; overflow: hidden;">
            <div style="position: absolute; top: 0; left: 0; right: 0; height: 3px; background: {m.color};"></div>
            <m.icon size={18} style="color: {m.color}; margin-bottom: 6px;" stroke-width={1.8} />
            <div style="font-size: 19px; font-weight: 700; font-family: var(--font-mono); color: var(--color-fg-0); letter-spacing: -0.3px;">{m.value}</div>
            <div style="font-size: 11px; font-weight: 500; color: var(--color-fg-3); margin-top: 2px;">{m.label}</div>
          </div>
        {/each}
      </div>

      <!-- HTTP traffic by endpoint group -->
      <div style="font-size: 13px; font-weight: 600; color: var(--color-fg-1); margin: 0 0 12px; display: flex; align-items: center; gap: 6px;">
        <Server size={15} style="color: var(--color-primary);" /> HTTP Traffic by Endpoint Group
      </div>
      {#if httpSeries.length === 0}
        <div class="card" style="padding: 16px; color: var(--color-fg-2); font-size: 13px;">
          No HTTP requests recorded yet. Series appear after traffic flows through the gateway.
        </div>
      {:else}
        <div class="card">
          <div style="overflow-x: auto;">
            <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
              <thead>
                <tr>
                  {#each ['Endpoint Group', 'Status', 'Requests', 'Avg Latency'] as h}
                    <th style="text-align: left; padding: 10px 14px; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; color: var(--color-fg-3); border-bottom: 1px solid var(--color-border); background: var(--color-bg-body);">{h}</th>
                  {/each}
                </tr>
              </thead>
              <tbody>
                {#each httpSeries as s}
                  <tr style="border-bottom: 1px solid var(--color-border-light);">
                    <td style="padding: 10px 14px; font-family: var(--font-mono); font-size: 12px; color: var(--color-fg-0);">{s.endpoint}</td>
                    <td style="padding: 10px 14px;">
                      <span style="font-size: 11px; font-weight: 600; padding: 2px 8px; border-radius: 4px; color: {statusColor(s.statusClass)}; background: color-mix(in srgb, {statusColor(s.statusClass)} 12%, transparent);">{s.statusClass}</span>
                    </td>
                    <td style="padding: 10px 14px; font-family: var(--font-mono); font-size: 12px;">{s.count.toLocaleString()}</td>
                    <td style="padding: 10px 14px; font-family: var(--font-mono); font-size: 12px;">{fmtLatency(s.sumSeconds, s.count)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>
      {/if}
    {/if}

    {#if lastUpdated}
      <div style="font-size: 11px; color: var(--color-fg-3); margin-top: 16px; text-align: right;">
        Auto-refreshes every 15s · last updated {lastUpdated.toLocaleTimeString()}
      </div>
    {/if}
  {/if}
</div>

<style>
  .warn-banner {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 14px 16px;
    border-radius: 10px;
    border: 1px solid;
    margin-bottom: 4px;
  }
  .warn-banner.critical {
    background: color-mix(in srgb, var(--color-error) 8%, var(--color-bg-card));
    border-color: color-mix(in srgb, var(--color-error) 35%, transparent);
    color: var(--color-error);
  }
  .warn-banner.warn {
    background: color-mix(in srgb, var(--color-warning) 8%, var(--color-bg-card));
    border-color: color-mix(in srgb, var(--color-warning) 35%, transparent);
    color: var(--color-warning);
  }
  .warn-banner.ok {
    background: color-mix(in srgb, var(--color-success) 8%, var(--color-bg-card));
    border-color: color-mix(in srgb, var(--color-success) 30%, transparent);
    color: var(--color-success);
    align-items: center;
  }
  .wb-title { font-size: 13px; font-weight: 700; margin-bottom: 3px; }
  .wb-body { font-size: 12.5px; line-height: 1.5; color: var(--color-fg-1); }
  .warn-banner.ok .wb-body { color: var(--color-fg-1); }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
