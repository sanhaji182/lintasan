<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { showToast } from '$lib/toast';
  import { buildCallableCatalog, filterCallableModels, type CallableModel } from '$lib/workflow-consolidation';
  import { Search, Copy, Play, TestTube2, Boxes, RefreshCw, Cloud, Route, Server } from 'lucide-svelte';

  let rows = $state<CallableModel[]>([]);
  let loading = $state(true);
  let error = $state('');
  let query = $state('');
  let kind = $state<'all' | 'route' | 'cloud_agent' | 'provider'>('all');
  let testing = $state<string | null>(null);
  let testResults = $state<Record<string, { ok: boolean; message: string }>>({});

  const visible = $derived(filterCallableModels(rows, query).filter(row => kind === 'all' || row.kind === kind));
  const counts = $derived({
    all: rows.length,
    route: rows.filter(row => row.kind === 'route').length,
    cloud_agent: rows.filter(row => row.kind === 'cloud_agent').length,
    provider: rows.filter(row => row.kind === 'provider').length,
  });

  async function load() {
    loading = true; error = '';
    try {
      const [models, combos, aliases, connections] = await Promise.all([
        api.get<any>('/v1/models'), api.get<any>('/api/combos').catch(() => ({ data: [] })),
        api.get<any>('/api/aliases').catch(() => ({ data: {} })), api.get<any>('/api/connections').catch(() => ({ data: [] })),
      ]);
      rows = buildCallableCatalog({
        models: models?.data || [], combos: combos?.data || combos?.combos || [],
        aliases: aliases?.data || {}, connections: connections?.data || [],
      });
    } catch (e: any) { error = e.message || 'Could not load callable models'; }
    loading = false;
  }

  onMount(load);

  async function copyID(id: string) {
    await navigator.clipboard.writeText(id);
    showToast('Callable ID copied', 'success', 1800);
  }

  async function testModel(row: CallableModel) {
    if (!row.connectionId || row.kind === 'route') return;
    testing = row.id;
    try {
      const result = await api.post<any>('/api/models/test', { model_id: row.id, connection_id: row.connectionId });
      testResults[row.id] = { ok: !!result.success, message: result.message || result.status || (result.success ? 'Available' : 'Unavailable') };
    } catch (e: any) { testResults[row.id] = { ok: false, message: e.message || 'Test failed' }; }
    testing = null;
  }

  function fmtPrice(price: unknown): string {
    if (price == null) return 'Not reported';
    if (typeof price === 'string' || typeof price === 'number') return String(price);
    return Object.entries(price as Record<string, unknown>).map(([k, v]) => `${k}: ${v}`).join(' · ') || 'Not reported';
  }
</script>

<svelte:head><title>Models — Lintasan</title></svelte:head>

<div class="catalog-page">
  <header class="catalog-header">
    <div><div class="eyebrow">CALLABLE INVENTORY</div><h2>Models</h2><p>Everything you can call now: routes, Cloud Agents, and provider models. Metadata is shown only when reported by the source.</p></div>
    <button class="btn-secondary" onclick={load} disabled={loading}><RefreshCw size={14} /> Refresh</button>
  </header>

  <section class="catalog-tools" aria-label="Model catalog filters">
    <label class="search"><Search size={16} /><span class="sr-only">Search callable models</span><input bind:value={query} placeholder="Search ID, provider, account, or route…" /></label>
    <div class="filters" role="group" aria-label="Model type">
      {#each [['all','All'], ['route','Aliases & Combos'], ['cloud_agent','Cloud Agents'], ['provider','Provider Models']] as option}
        <button class:active={kind === option[0]} onclick={() => kind = option[0] as typeof kind}>{option[1]} <span>{counts[option[0] as keyof typeof counts]}</span></button>
      {/each}
    </div>
  </section>

  {#if loading}<div class="loading"><Spinner /></div>
  {:else if error}<div class="card"><EmptyState icon={Boxes} title="Models unavailable" description={error} action={load} actionLabel="Retry" /></div>
  {:else if visible.length === 0}<div class="card"><EmptyState icon={Search} title="No callable IDs match" description="Change the search or type filter." /></div>
  {:else}
    <div class="model-grid">
      {#each visible as row (row.id)}
        <article class="model-card">
          <div class="model-top">
            <div class="kind-icon" class:cloud={row.kind === 'cloud_agent'}>{#if row.kind === 'route'}<Route size={18} />{:else if row.kind === 'cloud_agent'}<Cloud size={18} />{:else}<Server size={18} />{/if}</div>
            <div class="identity"><code>{row.id}</code><span>{row.kind === 'route' ? 'Alias / combo' : row.kind === 'cloud_agent' ? 'Cloud Agent' : 'Provider model'}</span></div>
            <span class="health" data-health={testResults[row.id]?.ok ? 'healthy' : row.health}>{testResults[row.id] ? (testResults[row.id].ok ? 'verified' : 'failed') : row.health}</span>
          </div>
          <dl>
            <div><dt>Route / provider</dt><dd>{row.route || row.provider || 'Not reported'}</dd></div>
            <div><dt>Account</dt><dd>{row.account || 'Not reported'}</dd></div>
            <div><dt>Context</dt><dd>{row.contextWindow ? row.contextWindow.toLocaleString() + ' tokens' : 'Not reported'}</dd></div>
            <div><dt>Price</dt><dd>{fmtPrice(row.price)}</dd></div>
            <div><dt>Capabilities</dt><dd>{row.capabilities.length ? row.capabilities.join(', ') : (row.supportsStreaming === false ? 'Non-streaming' : 'Not reported')}</dd></div>
          </dl>
          {#if testResults[row.id]}<div class="test-result" class:failed={!testResults[row.id].ok}>{testResults[row.id].message}</div>{/if}
          <div class="actions">
            <button class="btn-secondary" onclick={() => copyID(row.id)}><Copy size={13} /> Copy ID</button>
            {#if row.connectionId && row.kind !== 'route'}<button class="btn-secondary" onclick={() => testModel(row)} disabled={testing === row.id}><TestTube2 size={13} /> {testing === row.id ? 'Testing…' : 'Safe test'}</button>{/if}
            <a class="btn-primary" href={`/dashboard/playground?model=${encodeURIComponent(row.id)}`}><Play size={13} /> Playground</a>
          </div>
        </article>
      {/each}
    </div>
  {/if}
</div>

<style>
  .catalog-page{display:flex;flex-direction:column;gap:18px}.catalog-header{display:flex;justify-content:space-between;align-items:flex-start;gap:16px}.catalog-header h2{font-size:24px;font-weight:750;color:var(--color-fg-0);margin:2px 0}.catalog-header p{font-size:13px;color:var(--color-fg-2);max-width:720px}.eyebrow{font-size:10px;font-weight:800;letter-spacing:.12em;color:var(--color-primary)}.catalog-tools{position:sticky;top:calc(var(--header-h) + 8px);z-index:15;background:color-mix(in srgb,var(--color-bg-body) 92%,transparent);backdrop-filter:blur(12px);padding:10px;border:1px solid var(--color-border);border-radius:12px;display:flex;gap:12px;flex-wrap:wrap}.search{display:flex;align-items:center;gap:8px;flex:1;min-width:240px;background:var(--color-bg-card);border:1px solid var(--color-border);border-radius:9px;padding:0 11px;color:var(--color-fg-3)}.search input{border:0;outline:0;background:transparent;color:var(--color-fg-0);padding:9px 0;width:100%;font-size:13px}.filters{display:flex;gap:5px;flex-wrap:wrap}.filters button{border:1px solid var(--color-border);background:var(--color-bg-card);color:var(--color-fg-2);padding:7px 10px;border-radius:8px;font-size:11px;cursor:pointer}.filters button.active{background:var(--color-primary-light);border-color:var(--color-primary);color:var(--color-primary)}.filters span{font-family:var(--font-mono);opacity:.75}.model-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(340px,1fr));gap:12px}.model-card{background:var(--color-bg-card);border:1px solid var(--color-border);border-radius:12px;padding:15px;display:flex;flex-direction:column;gap:13px}.model-top{display:flex;align-items:center;gap:10px}.kind-icon{width:34px;height:34px;border-radius:9px;background:var(--color-primary-light);color:var(--color-primary);display:grid;place-items:center}.kind-icon.cloud{background:var(--color-purple-light);color:var(--color-purple)}.identity{min-width:0;flex:1}.identity code{display:block;overflow:hidden;text-overflow:ellipsis;font-size:12px;font-weight:650;color:var(--color-fg-0)}.identity span{font-size:10px;color:var(--color-fg-3)}.health{font-size:10px;padding:3px 7px;border-radius:999px;background:var(--color-border-light);color:var(--color-fg-2)}.health[data-health=healthy],.health[data-health=verified]{background:var(--color-success-light);color:var(--color-success)}dl{display:grid;grid-template-columns:1fr 1fr;gap:9px;margin:0}dl div{min-width:0}dt{font-size:9px;text-transform:uppercase;letter-spacing:.06em;color:var(--color-fg-3)}dd{margin:2px 0 0;font-size:11px;color:var(--color-fg-1);overflow-wrap:anywhere}.actions{display:flex;gap:7px;flex-wrap:wrap;margin-top:auto}.actions a{text-decoration:none}.test-result{font-size:11px;padding:7px 9px;border-radius:7px;background:var(--color-success-light);color:var(--color-success)}.test-result.failed{background:var(--color-error-light);color:var(--color-error)}.loading{padding:60px;display:grid;place-items:center}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0)}@media(max-width:640px){.catalog-header{flex-direction:column}.catalog-tools{top:8px}.model-grid{grid-template-columns:1fr}dl{grid-template-columns:1fr}}
</style>
