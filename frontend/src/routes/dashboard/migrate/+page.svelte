<script lang="ts">
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import {
    Upload, ArrowRight, CircleCheck, CircleAlert, Ban,
    HeartCrack, Layers, KeyRound, Info, RotateCcw
  } from 'lucide-svelte/icons';

  // Mirrors internal/migrate.Connection / .Combo / .Summary.
  interface Conn {
    name: string;
    base_url: string;
    format: string;
    priority: number;
    is_active: boolean;
    portability: string;
    health: string;
    reason?: string;
    source_provider: string;
    has_key: boolean;
    prefix?: string;
  }
  interface Combo {
    name: string;
    models: string[];
    partial: boolean;
    skipped_models?: string[];
  }
  interface Provider {
    name: string;
    base_url: string;
    format: string;
    prefix?: string;
    accounts: number;
    healthy: number;
  }
  interface Summary {
    healthy: number;
    unusable: number;
    blocked: number;
    blocked_by_reason: Record<string, number>;
    combos: number;
    combos_partial: number;
  }
  interface Preview {
    source: string;
    summary: Summary;
    selected: Conn[];
    healthy: Conn[];
    unusable: Conn[];
    blocked: Conn[];
    combos: Combo[];
    providers: Provider[];
    warnings: string[];
  }

  let file = $state<File | null>(null);
  let preview = $state<Preview | null>(null);
  let result = $state<any>(null);
  let busy = $state(false);
  let error = $state('');
  let includeUnusable = $state(false);
  let importCombos = $state(true);
  let dragging = $state(false);

  // Recomputed client-side so toggling "include unusable" updates the count
  // without a round trip. The server recomputes the same thing on import, so
  // this is a display aid, never the source of truth.
  const willImport = $derived(
    preview ? (includeUnusable ? preview.healthy.length + preview.unusable.length : preview.healthy.length) : 0
  );

  async function runPreview(f: File) {
    busy = true;
    error = '';
    result = null;
    preview = null;
    try {
      const text = await f.text();
      const res = await api.post<{ data: Preview }>('/api/migrate/preview', JSON.parse(text));
      preview = res.data;
    } catch (e: any) {
      error = e?.message || 'Could not read this file';
    } finally {
      busy = false;
    }
  }

  async function onPick(event: Event) {
    const input = event.target as HTMLInputElement;
    const f = input.files?.[0];
    if (f) { file = f; await runPreview(f); }
  }

  async function onDrop(e: DragEvent) {
    e.preventDefault();
    dragging = false;
    const f = e.dataTransfer?.files?.[0];
    if (f) { file = f; await runPreview(f); }
  }

  // Importing only the endpoints is a separate request, not a flag on the
  // account import: the two produce different results and conflating them in
  // one button would make it unclear which one just ran.
  async function importProvidersOnly() {
    if (!file) return;
    busy = true;
    error = '';
    try {
      const text = await file.text();
      const res = await api.post<{ data: any }>('/api/migrate/providers', JSON.parse(text));
      result = { ...res.data, providers_only: true };
    } catch (e: any) {
      error = e?.message || 'Import failed';
    } finally {
      busy = false;
    }
  }

  async function confirmImport() {
    if (!file) return;
    busy = true;
    error = '';
    try {
      const text = await file.text();
      const qs = new URLSearchParams();
      if (includeUnusable) qs.set('include_unusable', 'true');
      if (!importCombos) qs.set('skip_combos', 'true');
      const res = await api.post<{ data: any }>(`/api/migrate/import?${qs}`, JSON.parse(text));
      result = res.data;
    } catch (e: any) {
      error = e?.message || 'Import failed';
    } finally {
      busy = false;
    }
  }

  function reset() {
    file = null; preview = null; result = null; error = ''; includeUnusable = false;
  }

  function portLabel(p: string): string {
    return p === 'direct' ? 'own endpoint' : p === 'preset' ? 'via preset' : p;
  }
</script>

<svelte:head><title>Migrate — Lintasan</title></svelte:head>

<div class="page">
  <header class="head">
    <div>
      <h1>Migrate from another router</h1>
      <p class="sub">
        Import your connections from a 9router backup. Nothing is written until you confirm.
      </p>
    </div>
    {#if preview || result}
      <button class="ghost" onclick={reset}><RotateCcw size={15} /> Start over</button>
    {/if}
  </header>

  {#if error}
    <div class="alert err"><CircleAlert size={16} /> {error}</div>
  {/if}

  <!-- STEP 1: pick a file -->
  {#if !preview && !result}
    <div
      class="drop"
      class:over={dragging}
      role="button"
      tabindex="0"
      ondragover={(e) => { e.preventDefault(); dragging = true; }}
      ondragleave={() => (dragging = false)}
      ondrop={onDrop}
    >
      {#if busy}
        <Spinner />
        <p>Reading your export…</p>
      {:else}
        <Upload size={26} />
        <p class="drop-main">Drop your <code>9router-backup-*.json</code> here</p>
        <p class="drop-sub">or</p>
        <label class="btn">
          Choose file
          <input type="file" accept="application/json,.json" onchange={onPick} hidden />
        </label>
        <p class="note">
          <KeyRound size={13} />
          Your file is parsed in memory and never saved to disk. API keys stay on this server.
        </p>
      {/if}
    </div>
  {/if}

  <!-- STEP 2: preview -->
  {#if preview && !result}
    <div class="cards">
      <div class="card ok">
        <CircleCheck size={18} />
        <div class="n">{preview.summary.healthy}</div>
        <div class="l">ready to import</div>
      </div>
      <div class="card warn">
        <HeartCrack size={18} />
        <div class="n">{preview.summary.unusable}</div>
        <div class="l">already failing</div>
      </div>
      <div class="card bad">
        <Ban size={18} />
        <div class="n">{preview.summary.blocked}</div>
        <div class="l">can't be migrated</div>
      </div>
      <div class="card neutral">
        <Layers size={18} />
        <div class="n">{preview.summary.combos}</div>
        <div class="l">combos</div>
      </div>
    </div>

    {#each preview.warnings as w}
      <div class="alert info"><Info size={15} /> {w}</div>
    {/each}

    <section class="panel">
      <h2>Will be imported ({willImport})</h2>
      <table>
        <thead>
          <tr><th>Name</th><th>Endpoint</th><th>Source</th><th>Key</th></tr>
        </thead>
        <tbody>
          {#each (includeUnusable ? [...preview.healthy, ...preview.unusable] : preview.healthy) as c}
            <tr class:dim={c.health !== 'ok'}>
              <td>{c.name}</td>
              <td><code>{c.base_url}</code></td>
              <td><span class="tag">{portLabel(c.portability)}</span></td>
              <td>{c.has_key ? '✓' : '—'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </section>

    {#if preview.blocked.length}
      <section class="panel muted">
        <h2>Cannot be migrated ({preview.blocked.length})</h2>
        <p class="explain">
          These use a login-based (OAuth) flow or a provider Lintasan has no endpoint for.
          Knowing the URL isn't enough — they need a dedicated adapter. Keep using your
          current router for these.
        </p>
        <div class="chips">
          {#each [...new Set(preview.blocked.map((b) => b.source_provider))] as p}
            <span class="chip">{p}</span>
          {/each}
        </div>
      </section>
    {/if}

    {#if preview.combos.length}
      <section class="panel">
        <h2>Combos ({preview.combos.length})</h2>
        <ul class="combos">
          {#each preview.combos as c}
            <li>
              <strong>{c.name}</strong>
              <span class="count">{c.models.length} models</span>
              {#if c.partial}
                <span class="partial">
                  {c.skipped_models?.length} skipped (not migratable)
                </span>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <section class="confirm">
      <label class="opt">
        <input type="checkbox" bind:checked={importCombos} />
        Import combos too
      </label>
      {#if preview.summary.unusable > 0}
        <label class="opt">
          <input type="checkbox" bind:checked={includeUnusable} />
          Also import the {preview.summary.unusable} connection(s) that were already failing
        </label>
      {/if}
      <button class="btn primary" onclick={confirmImport} disabled={busy || willImport === 0}>
        {#if busy}<Spinner />{:else}Import {willImport} connection(s) <ArrowRight size={16} />{/if}
      </button>
      {#if preview.providers?.length}
        <div class="alt">
          <span class="alt-or">or</span>
          <button class="btn ghost" onclick={importProvidersOnly} disabled={busy}>
            Import the {preview.providers.length} provider(s) only
          </button>
          <p class="explain">
            Adds the endpoints as presets so you can create connections with your own keys.
            No accounts and no API keys are copied.
          </p>
        </div>
      {/if}
    </section>
  {/if}

  <!-- STEP 3: done -->
  {#if result}
    <section class="panel done">
      <h2><CircleCheck size={18} /> Import complete</h2>
      {#if result.providers_only}
        <ul class="result">
          <li><strong>{result.presets_imported}</strong> provider(s) added</li>
          {#if result.presets_skipped}
            <li><strong>{result.presets_skipped}</strong> skipped (already present)</li>
          {/if}
          <li><strong>{result.accounts_ignored}</strong> account(s) ignored, as requested</li>
        </ul>
      {:else}
        <ul class="result">
          <li><strong>{result.connections_imported}</strong> connection(s) added</li>
          {#if result.connections_skipped}
            <li><strong>{result.connections_skipped}</strong> skipped (already present)</li>
          {/if}
          {#if result.combos_imported}
            <li><strong>{result.combos_imported}</strong> combo(s) added</li>
          {/if}
          {#if result.combos_skipped}
            <li><strong>{result.combos_skipped}</strong> combo(s) skipped (name already used)</li>
          {/if}
        </ul>
      {/if}
      {#each result.skipped_reasons || [] as r}
        <p class="explain">{r}</p>
      {/each}
      {#if result.providers_only}
        <a class="btn primary" href="/dashboard/providers">View providers <ArrowRight size={16} /></a>
      {:else}
        <a class="btn primary" href="/dashboard/connections">View connections <ArrowRight size={16} /></a>
      {/if}
    </section>
  {/if}
</div>

<style>
  .page { padding: 1.5rem; max-width: 1080px; margin: 0 auto; display: flex; flex-direction: column; gap: 1rem; }
  .head { display: flex; justify-content: space-between; align-items: flex-start; gap: 1rem; }
  h1 { font-size: 1.35rem; font-weight: 650; margin: 0; }
  .sub { color: var(--text-muted, #94a3b8); margin: .3rem 0 0; font-size: .9rem; }

  .drop {
    border: 1.5px dashed var(--border, #334155); border-radius: 14px;
    padding: 2.5rem 1.5rem; text-align: center;
    display: flex; flex-direction: column; align-items: center; gap: .6rem;
    transition: border-color .15s, background .15s;
  }
  .drop.over { border-color: var(--accent, #8b5cf6); background: rgba(139, 92, 246, .06); }
  .drop-main { margin: .3rem 0 0; font-size: 1rem; }
  .drop-sub { color: var(--text-muted, #94a3b8); font-size: .85rem; margin: 0; }
  .note {
    display: flex; align-items: center; gap: .4rem; margin: .8rem 0 0;
    font-size: .8rem; color: var(--text-muted, #94a3b8);
  }

  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: .75rem; }
  .card {
    border: 1px solid var(--border, #334155); border-radius: 12px;
    padding: .9rem 1rem; display: flex; flex-direction: column; gap: .15rem;
  }
  .card .n { font-size: 1.5rem; font-weight: 650; }
  .card .l { font-size: .78rem; color: var(--text-muted, #94a3b8); }
  .card.ok { border-left: 3px solid #22c55e; }
  .card.warn { border-left: 3px solid #f59e0b; }
  .card.bad { border-left: 3px solid #ef4444; }
  .card.neutral { border-left: 3px solid #64748b; }

  .alert {
    display: flex; align-items: center; gap: .5rem;
    padding: .6rem .8rem; border-radius: 9px; font-size: .85rem;
  }
  .alert.err { background: rgba(239, 68, 68, .12); color: #fca5a5; }
  .alert.info { background: rgba(100, 116, 139, .12); color: var(--text-muted, #94a3b8); }

  .panel { border: 1px solid var(--border, #334155); border-radius: 12px; padding: 1rem; }
  .panel h2 { font-size: .95rem; font-weight: 600; margin: 0 0 .7rem; display: flex; align-items: center; gap: .4rem; }
  .panel.muted { opacity: .85; }
  .explain { font-size: .82rem; color: var(--text-muted, #94a3b8); margin: 0 0 .6rem; line-height: 1.5; }

  table { width: 100%; border-collapse: collapse; font-size: .84rem; }
  th { text-align: left; font-weight: 500; color: var(--text-muted, #94a3b8); padding: .35rem .5rem; }
  td { padding: .4rem .5rem; border-top: 1px solid var(--border, #1e293b); }
  tr.dim { opacity: .55; }
  code { font-size: .8rem; color: var(--text-muted, #94a3b8); }
  .tag { font-size: .72rem; padding: .12rem .45rem; border-radius: 5px; background: rgba(139, 92, 246, .15); }

  .chips { display: flex; flex-wrap: wrap; gap: .35rem; }
  .chip { font-size: .75rem; padding: .2rem .5rem; border-radius: 6px; background: rgba(239, 68, 68, .12); color: #fca5a5; }

  .combos { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: .35rem; }
  .combos li { display: flex; align-items: center; gap: .6rem; font-size: .85rem; }
  .count { color: var(--text-muted, #94a3b8); font-size: .8rem; }
  .partial { font-size: .75rem; color: #fbbf24; }

  .confirm { display: flex; flex-direction: column; gap: .6rem; padding-top: .3rem; }
  .opt { display: flex; align-items: center; gap: .5rem; font-size: .85rem; }

  .btn {
    display: inline-flex; align-items: center; gap: .4rem; cursor: pointer;
    padding: .5rem .9rem; border-radius: 9px; font-size: .87rem;
    border: 1px solid var(--border, #334155); background: transparent; color: inherit;
    text-decoration: none; width: fit-content;
  }
  .btn.primary { background: var(--accent, #8b5cf6); border-color: transparent; color: #fff; }
  .btn:disabled { opacity: .5; cursor: not-allowed; }
  .ghost { background: none; border: none; color: var(--text-muted, #94a3b8); cursor: pointer; display: flex; align-items: center; gap: .3rem; font-size: .82rem; }

  .alt { display: flex; flex-direction: column; gap: .3rem; margin-top: .4rem; }
  .alt-or { font-size: .75rem; color: var(--text-muted, #64748b); text-transform: uppercase; letter-spacing: .05em; }
  .alt .btn.ghost { text-decoration: underline; padding-left: 0; }

  .done h2 { color: #22c55e; }
  .result { list-style: none; padding: 0; margin: 0 0 .8rem; display: flex; flex-direction: column; gap: .3rem; font-size: .88rem; }
</style>
