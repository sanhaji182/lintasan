<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import { FlaskConical, ShieldAlert, ExternalLink, Trash2 } from 'lucide-svelte';

  interface CatalogEntry {
    id: string;
    name: string;
    flow: string;
    implementation: string;
    deprecated?: boolean;
    notes?: string;
  }

  interface OAuthStatus {
    enabled: boolean;
    experimental?: boolean;
    catalog?: CatalogEntry[];
    disclaimer?: string;
    public_base?: string;
    hint?: string;
    source?: string;
  }

  interface OAuthSession {
    id: string;
    provider: string;
    status: string;
    expires_at?: string;
    created_at?: string;
  }

  let status = $state<OAuthStatus | null>(null);
  let sessions = $state<OAuthSession[]>([]);
  let loading = $state(true);
  let actionLoading = $state('');
  let acknowledge = $state(false);
  let selectedProvider = $state('xai');
  let deviceInfo = $state<any>(null);
  let pollSessionId = $state('');
  let lastRedirect = $state('');
  let error = $state('');
  let cursorToken = $state('');
  let cursorMachineId = $state('');

  const catalog = $derived(status?.catalog ?? []);
  const readyProviders = $derived(catalog.filter((p) => p.implementation === 'ready'));
  const importOnly = $derived(catalog.filter((p) => p.implementation === 'import_only'));

  async function load() {
    loading = true;
    error = '';
    try {
      const st = await api.get<OAuthStatus>('/api/oauth/status');
      status = st;
      if (st.enabled) {
        const sess = await api.get<{ data: OAuthSession[] }>('/api/oauth/sessions');
        sessions = sess.data ?? [];
      } else {
        sessions = [];
      }
    } catch (e: any) {
      error = e?.message || 'Failed to load OAuth IDE status';
    } finally {
      loading = false;
    }
  }

  async function authorize() {
    if (!acknowledge) {
      error = 'Acknowledge the risks before continuing.';
      return;
    }
    actionLoading = 'authorize';
    error = '';
    lastRedirect = '';
    try {
      const res = await api.post<any>('/api/oauth/authorize', {
        provider: selectedProvider,
        acknowledge_risk: true,
      });
      pollSessionId = res.session_id || '';
      if (res.flow === 'device_code' && res.device) {
        deviceInfo = res.device;
        lastRedirect = '';
      } else {
        deviceInfo = null;
        lastRedirect = res.redirect_url || '';
        if (lastRedirect) {
          window.open(lastRedirect, '_blank', 'noopener,noreferrer');
        }
      }
      await load();
    } catch (e: any) {
      error = e?.message || 'Authorize failed';
    } finally {
      actionLoading = '';
    }
  }

  async function pollDevice() {
    if (!pollSessionId) return;
    actionLoading = 'poll';
    error = '';
    try {
      const res = await api.post<any>(`/api/oauth/device/poll?session_id=${encodeURIComponent(pollSessionId)}`, {});
      if (res.status === 'active') {
        deviceInfo = null;
        await load();
      } else {
        error = res.hint || 'Still pending — complete device login';
      }
    } catch (e: any) {
      error = e?.message || 'Poll failed';
    } finally {
      actionLoading = '';
    }
  }

  async function importCursor() {
    if (!acknowledge) {
      error = 'Acknowledge the risks before continuing.';
      return;
    }
    actionLoading = 'cursor-import';
    error = '';
    try {
      await api.post('/api/oauth/cursor/import', {
        accessToken: cursorToken,
        machineId: cursorMachineId,
        acknowledge_risk: true,
      });
      cursorToken = '';
      cursorMachineId = '';
      await load();
    } catch (e: any) {
      error = e?.message || 'Cursor import failed';
    } finally {
      actionLoading = '';
    }
  }

  async function revoke(id: string) {
    actionLoading = id;
    try {
      await api.delete(`/api/oauth/sessions/${id}`);
      await load();
    } catch (e: any) {
      error = e?.message || 'Revoke failed';
    } finally {
      actionLoading = '';
    }
  }

  onMount(load);
</script>

<svelte:head><title>OAuth IDE (Experimental) — Lintasan</title></svelte:head>

<div class="page">
  <header class="page-header">
    <div class="title-row">
      <FlaskConical size={28} stroke-width={1.6} />
      <div>
        <h1>OAuth IDE <span class="badge">Experimental</span></h1>
        <p class="subtitle">Lab feature — personal BYO subscription only. Default OFF on the server.</p>
      </div>
    </div>
  </header>

  {#if loading}
    <Spinner />
  {:else}
    {#if error}
      <div class="alert error">{error}</div>
    {/if}

    <section class="card warn-card">
      <ShieldAlert size={20} />
      <div>
        <strong>ToS &amp; risk</strong>
        <pre class="disclaimer">{status?.disclaimer ?? 'Upstream providers may prohibit third-party OAuth routing. Account suspension is possible. Not a substitute for official API keys.'}</pre>
      </div>
    </section>

    {#if catalog.length > 0}
      <section class="card">
        <h2>9router OAuth catalog (8)</h2>
        <p class="muted small">{status?.source}</p>
        <ul class="catalog">
          {#each catalog as p}
            <li>
              <strong>{p.name}</strong> <code>{p.id}</code>
              <span class="pill">{p.implementation}</span>
              <span class="pill muted">{p.flow}</span>
              {#if p.deprecated}<span class="pill warn">deprecated</span>{/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if !status?.enabled}
      <section class="card">
        <p><strong>Disabled on this instance.</strong></p>
        <p class="muted">Enable only on private/lab hosts:</p>
        <code class="block">LINTASAN_OAUTH_IDE_ENABLED=true</code>
        <code class="block">LINTASAN_OAUTH_PUBLIC_BASE_URL=https://your-host</code>
        <p class="muted small">Per provider: <code>LINTASAN_OAUTH_IDE_COPILOT_CLIENT_ID</code>, <code>_CLIENT_SECRET</code>, and for non-GitHub providers <code>_TOKEN_URL</code>.</p>
      </section>
    {:else}
      <section class="card">
        <p class="muted small">Public base: <code>{status.public_base}</code></p>
        <p class="muted small">{status.hint}</p>

        <label class="ack">
          <input type="checkbox" bind:checked={acknowledge} />
          I understand this is experimental, for my own account, and may violate upstream terms.
        </label>

        <div class="row">
          <select bind:value={selectedProvider}>
            {#each catalog as p}
              <option value={p.id} disabled={p.implementation !== 'ready'}>
                {p.name} ({p.implementation})
              </option>
            {/each}
          </select>
          <button class="btn primary" disabled={!acknowledge || actionLoading === 'authorize' || readyProviders.length === 0} onclick={authorize}>
            {actionLoading === 'authorize' ? 'Starting…' : 'Authorize (admin)'}
          </button>
        </div>
        {#if deviceInfo}
          <div class="device-box">
            <p><strong>Device login</strong> ({selectedProvider})</p>
            <p>Code: <code class="user-code">{deviceInfo.user_code}</code></p>
            <p><a href={deviceInfo.verification_uri_complete || deviceInfo.verification_uri} target="_blank" rel="noopener noreferrer">Open verification page <ExternalLink size={14} /></a></p>
            <button class="btn primary" disabled={actionLoading === 'poll'} onclick={pollDevice}>
              {actionLoading === 'poll' ? 'Polling…' : 'Poll for completion'}
            </button>
          </div>
        {/if}
        {#if lastRedirect}
          <p class="muted small">Opened: <a href={lastRedirect} target="_blank" rel="noopener noreferrer">provider login <ExternalLink size={14} /></a></p>
        {/if}

        {#if importOnly.length > 0}
          <div class="device-box" style="margin-top: 1rem;">
            <p><strong>Cursor — import token</strong></p>
            <p class="muted small">From <code>state.vscdb</code>: <code>cursorAuth/accessToken</code> + <code>storage.serviceMachineId</code></p>
            <textarea class="import-ta" placeholder="accessToken" bind:value={cursorToken} rows="2"></textarea>
            <input class="import-in" placeholder="machineId" bind:value={cursorMachineId} />
            <button class="btn primary" disabled={!acknowledge || actionLoading === 'cursor-import'} onclick={importCursor}>
              {actionLoading === 'cursor-import' ? 'Importing…' : 'Import Cursor session'}
            </button>
          </div>
        {/if}
      </section>

      <section class="card">
        <h2>Sessions</h2>
        {#if sessions.length === 0}
          <p class="muted">No OAuth sessions yet.</p>
        {:else}
          <ul class="session-list">
            {#each sessions as s}
              <li>
                <span><strong>{s.provider}</strong> — {s.status}</span>
                <span class="muted mono">{s.id}</span>
                <button class="btn ghost" disabled={actionLoading === s.id} onclick={() => revoke(s.id)}>
                  <Trash2 size={16} /> Revoke
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/if}
  {/if}
</div>

<style>
  .page { max-width: 720px; margin: 0 auto; padding: 1.5rem; }
  .title-row { display: flex; gap: 1rem; align-items: flex-start; }
  h1 { font-size: 1.35rem; margin: 0; }
  .badge {
    font-size: 0.7rem; vertical-align: middle;
    background: rgba(139, 92, 246, 0.15); color: #a78bfa;
    border: 1px solid rgba(139, 92, 246, 0.35);
    padding: 2px 8px; border-radius: 6px; margin-left: 8px;
  }
  .subtitle { color: var(--text-muted, #94a3b8); margin: 0.25rem 0 0; font-size: 0.9rem; }
  .card { background: var(--surface, #1e293b); border: 1px solid var(--border, #334155); border-radius: 12px; padding: 1.25rem; margin-bottom: 1rem; }
  .warn-card { display: flex; gap: 12px; border-color: rgba(234, 179, 8, 0.35); background: rgba(234, 179, 8, 0.06); }
  .disclaimer { white-space: pre-wrap; font-size: 0.8rem; margin: 0.5rem 0 0; font-family: inherit; color: var(--text-muted); }
  .ack { display: flex; gap: 8px; align-items: flex-start; margin: 1rem 0; font-size: 0.9rem; }
  .row { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
  select, .btn { padding: 0.5rem 0.75rem; border-radius: 8px; border: 1px solid var(--border); }
  .btn.primary { background: #3b82f6; color: #fff; border: none; cursor: pointer; }
  .btn.primary:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn.ghost { background: transparent; cursor: pointer; display: inline-flex; gap: 4px; align-items: center; }
  .session-list { list-style: none; padding: 0; margin: 0; }
  .session-list li { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; padding: 0.5rem 0; border-bottom: 1px solid var(--border); }
  .catalog { list-style: none; padding: 0; margin: 0; font-size: 0.85rem; }
  .catalog li { padding: 0.35rem 0; border-bottom: 1px solid var(--border); display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
  .pill { font-size: 0.7rem; padding: 2px 6px; border-radius: 4px; background: #334155; }
  .pill.warn { background: rgba(234, 179, 8, 0.2); color: #eab308; }
  .device-box { margin-top: 1rem; padding: 1rem; border: 1px dashed var(--border); border-radius: 8px; }
  .user-code { font-size: 1.25rem; letter-spacing: 0.1em; }
  .import-ta, .import-in { width: 100%; margin: 0.35rem 0; padding: 0.5rem; border-radius: 8px; border: 1px solid var(--border); background: #0f172a; font-family: ui-monospace, monospace; font-size: 0.8rem; }
  .mono { font-family: ui-monospace, monospace; font-size: 0.75rem; }
  .muted { color: var(--text-muted, #94a3b8); }
  .small { font-size: 0.85rem; }
  .block { display: block; margin: 0.35rem 0; padding: 0.35rem 0.5rem; background: #0f172a; border-radius: 6px; font-size: 0.8rem; }
  .alert.error { background: rgba(239, 68, 68, 0.12); border: 1px solid rgba(239, 68, 68, 0.35); padding: 0.75rem; border-radius: 8px; margin-bottom: 1rem; }
  code { font-size: 0.85em; }
</style>