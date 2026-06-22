<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import { showToast } from '$lib/toast';
  import { brandForProvider, logoPaths } from '$lib/oauthIdeBrands';
  import { presetByProvider } from '$lib/oauthIdePresets';
  import {
    FlaskConical,
    ShieldAlert,
    ExternalLink,
    Trash2,
    Link2,
    Copy,
    Check,
    CircleAlert,
    X,
    KeyRound,
    ArrowRight,
    RefreshCw
  } from 'lucide-svelte';

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
    xai_redirect_uri?: string;
    xai_note?: string;
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
  let copiedCode = $state(false);
  let xaiLoopbackWarn = $state('');
  let xaiCallbackPaste = $state('');
  let showXaiComplete = $state(false);

  const catalog = $derived(status?.catalog ?? []);
  const readyProviders = $derived(catalog.filter((p) => p.implementation === 'ready'));
  const importOnly = $derived(catalog.filter((p) => p.implementation === 'import_only'));
  const activeSessions = $derived(sessions.filter((s) => s.status === 'active').length);

  const summary = $derived({
    catalog: catalog.length,
    ready: readyProviders.length,
    sessions: sessions.length,
    active: activeSessions
  });

  function sessionForProvider(id: string) {
    return sessions.find((s) => s.provider === id && s.status === 'active');
  }

  function implLabel(impl: string) {
    if (impl === 'ready') return { label: 'Ready', color: '#22c55e', bg: 'rgba(34,197,94,0.12)' };
    if (impl === 'import_only') return { label: 'Import', color: '#f59e0b', bg: 'rgba(245,158,11,0.12)' };
    return { label: impl, color: 'var(--color-fg-3)', bg: 'var(--color-bg-3)' };
  }

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
      const cat = st.catalog ?? [];
      if (cat.length && !cat.some((c) => c.id === selectedProvider)) {
        const firstReady = cat.find((c) => c.implementation === 'ready');
        selectedProvider = firstReady?.id ?? cat[0]?.id ?? 'xai';
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
    xaiLoopbackWarn = '';
    showXaiComplete = false;
    try {
      const res = await api.post<any>('/api/oauth/authorize', {
        provider: selectedProvider,
        acknowledge_risk: true
      });
      pollSessionId = res.session_id || '';
      if (res.flow === 'device_code' && res.device) {
        deviceInfo = res.device;
        lastRedirect = '';
      } else {
        deviceInfo = null;
        lastRedirect = res.redirect_url || '';
        xaiLoopbackWarn = res.xai_loopback_warn || '';
        if (selectedProvider === 'xai') {
          showXaiComplete = !!res.xai_manual_complete;
        }
        if (lastRedirect) window.open(lastRedirect, '_blank', 'noopener,noreferrer');
      }
      if (xaiLoopbackWarn) {
        showToast('xAI login URL opened — see warning below', 'info');
      } else {
        showToast('OAuth flow started', 'info');
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
        showToast('Device login complete', 'success');
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

  async function copyUserCode() {
    const code = deviceInfo?.user_code;
    if (!code) return;
    try {
      await navigator.clipboard.writeText(code);
      copiedCode = true;
      showToast('User code copied', 'success', 2000);
      setTimeout(() => (copiedCode = false), 2000);
    } catch {
      showToast('Copy failed', 'error');
    }
  }

  async function completeXaiLogin() {
    actionLoading = 'xai-complete';
    error = '';
    try {
      await api.post('/api/oauth/xai/complete', { callback_url: xaiCallbackPaste });
      showToast('xAI session active', 'success');
      xaiCallbackPaste = '';
      showXaiComplete = false;
      await load();
    } catch (e: any) {
      error = e?.message || 'Complete failed';
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
        acknowledge_risk: true
      });
      cursorToken = '';
      cursorMachineId = '';
      showToast('Cursor session imported', 'success');
      await load();
    } catch (e: any) {
      error = e?.message || 'Cursor import failed';
    } finally {
      actionLoading = '';
    }
  }

  async function revoke(id: string) {
    if (!confirm('Revoke this OAuth session?')) return;
    actionLoading = id;
    try {
      await api.delete(`/api/oauth/sessions/${id}`);
      showToast('Session revoked', 'info');
      await load();
    } catch (e: any) {
      error = e?.message || 'Revoke failed';
    } finally {
      actionLoading = '';
    }
  }

  async function wireProxy(provider: string) {
    actionLoading = 'wire-' + provider;
    error = '';
    try {
      const res = await api.post<any>('/api/oauth/provision-connection', { provider });
      const preset = presetByProvider(provider);
      showToast(
        `${res.action === 'created' ? 'Created' : 'Updated'} ${res.name || preset?.name}`,
        'success',
        4000
      );
    } catch (e: any) {
      error = e?.message || 'Wire proxy failed';
      showToast(error, 'error');
    } finally {
      actionLoading = '';
    }
  }

  onMount(load);
</script>

<svelte:head><title>OAuth IDE (Experimental) — Lintasan</title></svelte:head>

<div class="oauth-page">
  <div class="section-header">
    <div class="header-icon">
      <FlaskConical size={22} />
    </div>
    <div class="header-text">
      <h1 class="header-title">OAuth IDE <span class="exp-pill">Experimental</span></h1>
      <p class="header-desc">9router-parity BYO subscription routing — lab only, admin-gated</p>
    </div>
    <button class="btn-icon" onclick={load} disabled={loading} title="Refresh">
      <RefreshCw size={18} class={loading ? 'spin' : ''} />
    </button>
  </div>

  <div class="stats-strip">
    {#each [
      { label: 'CATALOG', value: summary.catalog, color: 'var(--color-fg-0)' },
      { label: 'READY', value: summary.ready, color: '#22c55e' },
      { label: 'SESSIONS', value: summary.sessions, color: '#a78bfa' },
      { label: 'ACTIVE', value: summary.active, color: '#3b82f6' }
    ] as stat}
      <div class="stat-cell">
        <div class="stat-value" style="color: {stat.color}">{stat.value}</div>
        <div class="stat-label">{stat.label}</div>
      </div>
    {/each}
  </div>

  <div class="steps-strip">
    <span class="step"><span class="step-n">1</span> Acknowledge risk</span>
    <ArrowRight size={14} class="step-arrow" />
    <span class="step"><span class="step-n">2</span> Authorize provider</span>
    <ArrowRight size={14} class="step-arrow" />
    <span class="step"><span class="step-n">3</span> Wire proxy → Accounts</span>
    <ArrowRight size={14} class="step-arrow" />
    <span class="step"><span class="step-n">4</span> Test &amp; chat</span>
  </div>

  {#if error}
    <div class="error-banner">
      <CircleAlert size={16} />
      <span>{error}</span>
      <button type="button" class="error-close" onclick={() => (error = '')}><X size={14} /></button>
    </div>
  {/if}

  {#if loading}
    <div class="loading-state"><Spinner /><p>Loading OAuth IDE…</p></div>
  {:else}
    <section class="card warn-card">
      <ShieldAlert size={20} />
      <div>
        <strong>ToS &amp; risk</strong>
        <pre class="disclaimer">{status?.disclaimer ?? 'Upstream providers may prohibit third-party OAuth routing.'}</pre>
      </div>
    </section>

    {#if catalog.length > 0}
      <h2 class="section-title">Provider catalog</h2>
      <p class="section-sub muted">{status?.source}</p>
      <div class="provider-grid">
        {#each catalog as p}
          {@const brand = brandForProvider(p.id)}
          {@const impl = implLabel(p.implementation)}
          {@const sess = sessionForProvider(p.id)}
          <article
            class="provider-card"
            style="--brand-color: {brand.color}; --brand-bg: {brand.bg}; --brand-border: {brand.border}"
          >
            <div class="card-top">
              <div class="brand-logo" style="background: {brand.bg};">
                <svg viewBox="0 0 24 24" width="20" height="20">
                  {#each logoPaths(brand) as path}
                    <path d={path.d} fill={path.fill} />
                  {/each}
                </svg>
              </div>
              <div class="card-titles">
                <div class="provider-name">{p.name}</div>
                <div class="company-tag">{brand.company}</div>
              </div>
              <span class="state-pill" style="background: {impl.bg}; color: {impl.color}">{impl.label}</span>
            </div>
            <p class="tagline">{brand.tagline}</p>
            <div class="meta-row">
              <code class="id-chip">{p.id}</code>
              <span class="flow-chip">{p.flow}</span>
              {#if sess}
                <span class="live-chip"><span class="live-dot"></span> session</span>
              {/if}
            </div>
            {#if p.notes}
              <p class="note muted">{p.notes}</p>
            {/if}
            <div class="card-actions">
              {#if p.implementation === 'ready'}
                <button
                  type="button"
                  class="btn-primary-sm"
                  disabled={!status?.enabled || actionLoading === 'authorize'}
                  onclick={() => {
                    selectedProvider = p.id;
                    authorize();
                  }}
                >
                  <KeyRound size={14} /> Authorize
                </button>
              {:else if p.implementation === 'import_only'}
                <span class="hint-import muted">Use import panel below</span>
              {/if}
              {#if sess}
                <button
                  type="button"
                  class="btn-secondary-sm"
                  disabled={actionLoading === 'wire-' + p.id}
                  onclick={() => wireProxy(p.id)}
                >
                  <Link2 size={14} />
                  {actionLoading === 'wire-' + p.id ? '…' : 'Wire'}
                </button>
              {/if}
            </div>
          </article>
        {/each}
      </div>
    {/if}

    {#if !status?.enabled}
      <section class="card disabled-card">
        <h3>OAuth IDE lab is off</h3>
        <p class="muted">Turn it on in <a href="/dashboard/settings" class="link">Dashboard → Settings</a> → <strong>Experimental</strong> → OAuth IDE (lab).</p>
        <p class="muted small">Set <code>LINTASAN_OAUTH_PUBLIC_BASE_URL</code> on the server for redirect URLs. Per provider: <code>LINTASAN_OAUTH_IDE_*_CLIENT_ID</code> / secrets.</p>
      </section>
    {:else}
      <section class="card">
        <h3 class="card-h">Connect a provider</h3>
        <p class="muted small">Public base: <code>{status.public_base}</code></p>
        {#if status.xai_note}
          <p class="muted small xai-loopback-note">
            <strong>xAI:</strong> redirect <code>{status.xai_redirect_uri}</code> — {status.xai_note}
          </p>
        {/if}
        {#if status.hint}<p class="muted small">{status.hint}</p>{/if}

        <label class="ack">
          <input type="checkbox" bind:checked={acknowledge} />
          I understand this is experimental, for my own account, and may violate upstream terms.
        </label>

        <div class="row">
          <select class="input-select" bind:value={selectedProvider}>
            {#each catalog as p}
              <option value={p.id} disabled={p.implementation !== 'ready'}>
                {p.name} ({p.implementation})
              </option>
            {/each}
          </select>
          <button
            type="button"
            class="btn-primary"
            disabled={!acknowledge || actionLoading === 'authorize' || readyProviders.length === 0}
            onclick={authorize}
          >
            {actionLoading === 'authorize' ? 'Starting…' : 'Authorize (admin)'}
          </button>
        </div>

        {#if deviceInfo}
          <div class="device-box">
            <p><strong>Device login</strong> — {selectedProvider}</p>
            <div class="code-row">
              <code class="user-code">{deviceInfo.user_code}</code>
              <button type="button" class="btn-secondary-sm" onclick={copyUserCode}>
                {#if copiedCode}<Check size={14} />{:else}<Copy size={14} />{/if}
                Copy
              </button>
            </div>
            <p>
              <a
                href={deviceInfo.verification_uri_complete || deviceInfo.verification_uri}
                target="_blank"
                rel="noopener noreferrer"
                class="link"
              >
                Open verification page <ExternalLink size={14} />
              </a>
            </p>
            <button type="button" class="btn-primary" disabled={actionLoading === 'poll'} onclick={pollDevice}>
              {actionLoading === 'poll' ? 'Polling…' : 'Poll for completion'}
            </button>
          </div>
        {/if}
        {#if lastRedirect}
          <p class="muted small">
            Opened: <a href={lastRedirect} target="_blank" rel="noopener noreferrer">provider login</a>
          </p>
        {/if}
        {#if xaiLoopbackWarn}
          <p class="muted small" style="color: var(--color-warning);">{xaiLoopbackWarn}</p>
        {/if}
        {#if showXaiComplete}
          <div class="device-box">
            <p><strong>xAI — complete login</strong></p>
            <p class="muted small">
              After xAI redirects, paste the authorization code OR the full address bar URL.
              Paste just the code value (e.g. <code>peILQs52...</code>) if you got it, or the
              full <code>http://127.0.0.1:56121/callback?code=...&state=...</code> URL.
            </p>
            <textarea
              class="import-ta"
              placeholder="http://127.0.0.1:56121/callback?code=...&state=..."
              bind:value={xaiCallbackPaste}
              rows="2"
            ></textarea>
            <button
              type="button"
              class="btn-primary"
              disabled={!xaiCallbackPaste.trim() || actionLoading === 'xai-complete'}
              onclick={completeXaiLogin}
            >
              {actionLoading === 'xai-complete' ? 'Completing…' : 'Complete xAI login'}
            </button>
          </div>
        {/if}

        {#if importOnly.length > 0}
          <div class="device-box import-box">
            <p><strong>Cursor — import token</strong></p>
            <p class="muted small">
              From <code>state.vscdb</code>: <code>cursorAuth/accessToken</code> + <code>storage.serviceMachineId</code>
            </p>
            <textarea class="import-ta" placeholder="accessToken" bind:value={cursorToken} rows="2"></textarea>
            <input class="import-in" placeholder="machineId" bind:value={cursorMachineId} />
            <button
              type="button"
              class="btn-primary"
              disabled={!acknowledge || actionLoading === 'cursor-import'}
              onclick={importCursor}
            >
              {actionLoading === 'cursor-import' ? 'Importing…' : 'Import Cursor session'}
            </button>
          </div>
        {/if}
      </section>

      <section class="card">
        <div class="card-head-row">
          <h3 class="card-h">Active sessions</h3>
          <a href="/dashboard/connections" class="link-sm">Accounts →</a>
        </div>
        {#if sessions.length === 0}
          <p class="muted">No OAuth sessions yet. Authorize a provider above.</p>
        {:else}
          <ul class="session-list">
            {#each sessions as s}
              {@const brand = brandForProvider(s.provider)}
              <li class="session-row">
                <div class="session-main">
                  <span class="session-provider" style="color: {brand.color}">{s.provider}</span>
                  <span class="session-status" class:active={s.status === 'active'}>{s.status}</span>
                  {#if s.expires_at}
                    <span class="muted small">exp {s.expires_at}</span>
                  {/if}
                </div>
                <code class="session-id">{s.id.slice(0, 8)}…</code>
                <div class="session-actions">
                  <button
                    type="button"
                    class="btn-secondary-sm"
                    disabled={s.status !== 'active' || actionLoading === 'wire-' + s.provider}
                    onclick={() => wireProxy(s.provider)}
                  >
                    <Link2 size={14} /> Wire proxy
                  </button>
                  <button type="button" class="btn-ghost-danger" disabled={actionLoading === s.id} onclick={() => revoke(s.id)}>
                    <Trash2 size={14} /> Revoke
                  </button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/if}
  {/if}
</div>

<style>
  .oauth-page {
    animation: fadeInUp 0.35s ease-out;
    max-width: 960px;
    margin: 0 auto;
  }
  .section-header {
    display: flex;
    align-items: flex-start;
    gap: 14px;
    margin-bottom: 20px;
  }
  .header-icon {
    width: 44px;
    height: 44px;
    border-radius: 12px;
    background: rgba(139, 92, 246, 0.15);
    color: #a78bfa;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .header-title {
    font-size: 20px;
    font-weight: 700;
    margin: 0;
    color: var(--color-fg-0);
    letter-spacing: -0.3px;
  }
  .exp-pill {
    font-size: 10px;
    font-weight: 600;
    vertical-align: middle;
    margin-left: 8px;
    padding: 3px 8px;
    border-radius: 6px;
    background: rgba(139, 92, 246, 0.15);
    color: #a78bfa;
    border: 1px solid rgba(139, 92, 246, 0.35);
  }
  .header-desc {
    margin: 4px 0 0;
    font-size: 13px;
    color: var(--color-fg-3);
  }
  .btn-icon {
    margin-left: auto;
    border: 1px solid var(--color-border);
    background: var(--color-bg-card);
    border-radius: 10px;
    padding: 8px;
    cursor: pointer;
    color: var(--color-fg-2);
  }
  .btn-icon:disabled {
    opacity: 0.5;
  }
  :global(.spin) {
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .stats-strip {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 1px;
    background: var(--color-border);
    border-radius: 12px;
    overflow: hidden;
    margin-bottom: 16px;
  }
  .stat-cell {
    background: var(--color-bg-card);
    padding: 14px 16px;
    text-align: center;
  }
  .stat-value {
    font-size: 22px;
    font-weight: 700;
    font-family: var(--font-mono);
  }
  .stat-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--color-fg-3);
    letter-spacing: 0.5px;
    margin-top: 2px;
  }
  .steps-strip {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    padding: 12px 14px;
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: 10px;
    margin-bottom: 20px;
    font-size: 12px;
    color: var(--color-fg-2);
  }
  .step-n {
    display: inline-flex;
    width: 18px;
    height: 18px;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    background: var(--color-primary-light);
    color: var(--color-primary);
    font-size: 10px;
    font-weight: 700;
    margin-right: 6px;
  }
  .step-arrow {
    color: var(--color-fg-4);
    flex-shrink: 0;
  }
  .error-banner {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 14px;
    background: rgba(239, 68, 68, 0.08);
    border: 1px solid rgba(239, 68, 68, 0.25);
    border-radius: 10px;
    margin-bottom: 16px;
    font-size: 13px;
    color: var(--color-error);
  }
  .error-close {
    margin-left: auto;
    border: none;
    background: transparent;
    cursor: pointer;
    color: inherit;
    padding: 4px;
  }
  .loading-state {
    text-align: center;
    padding: 48px;
    color: var(--color-fg-3);
  }
  .card {
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: 12px;
    padding: 1.25rem;
    margin-bottom: 1rem;
  }
  .warn-card {
    display: flex;
    gap: 12px;
    border-color: rgba(234, 179, 8, 0.35);
    background: rgba(234, 179, 8, 0.06);
  }
  .disclaimer {
    white-space: pre-wrap;
    font-size: 0.8rem;
    margin: 0.5rem 0 0;
    font-family: inherit;
    color: var(--color-fg-3);
  }
  .section-title {
    font-size: 15px;
    font-weight: 600;
    margin: 0 0 4px;
    color: var(--color-fg-0);
  }
  .section-sub {
    font-size: 12px;
    margin: 0 0 12px;
  }
  .provider-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 14px;
    margin-bottom: 20px;
  }
  .provider-card {
    border: 1px solid var(--brand-border);
    border-radius: 14px;
    padding: 16px;
    background: linear-gradient(160deg, var(--brand-bg) 0%, var(--color-bg-card) 55%);
  }
  .card-top {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    margin-bottom: 8px;
  }
  .brand-logo {
    width: 40px;
    height: 40px;
    border-radius: 10px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .brand-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    margin-top: 6px;
    flex-shrink: 0;
    box-shadow: 0 0 10px var(--brand-color);
  }
  .card-titles {
    flex: 1;
    min-width: 0;
  }
  .provider-name {
    font-size: 14px;
    font-weight: 650;
    color: var(--color-fg-0);
  }
  .company-tag {
    font-size: 10px;
    font-weight: 600;
    color: var(--brand-color);
    margin-top: 2px;
  }
  .state-pill {
    font-size: 10px;
    font-weight: 600;
    padding: 3px 8px;
    border-radius: 6px;
  }
  .tagline {
    font-size: 12px;
    color: var(--color-fg-2);
    margin: 0 0 10px;
    line-height: 1.4;
  }
  .meta-row {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
    margin-bottom: 10px;
  }
  .id-chip {
    font-size: 10px;
    padding: 2px 6px;
    border-radius: 4px;
    background: var(--color-bg-3);
  }
  .flow-chip {
    font-size: 10px;
    color: var(--color-fg-3);
  }
  .live-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 10px;
    font-weight: 600;
    color: #22c55e;
  }
  .live-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #22c55e;
    animation: pulse-live 2s ease-in-out infinite;
  }
  @keyframes pulse-live {
    0%,
    100% {
      opacity: 1;
    }
    50% {
      opacity: 0.4;
    }
  }
  .note {
    font-size: 11px;
    margin: 0 0 8px;
  }
  .card-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .btn-primary-sm,
  .btn-secondary-sm {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 12px;
    border-radius: 9px;
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    border: none;
  }
  .btn-primary-sm {
    background: var(--color-primary);
    color: white;
  }
  .btn-primary-sm:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
  .btn-secondary-sm {
    background: var(--color-bg-3);
    color: var(--color-fg-1);
    border: 1px solid var(--color-border);
  }
  .btn-secondary-sm:disabled {
    opacity: 0.5;
  }
  .ack {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    margin: 1rem 0;
    font-size: 0.9rem;
  }
  .row {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    align-items: center;
  }
  .input-select {
    flex: 1;
    min-width: 180px;
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border);
    background: var(--color-bg-body);
    color: var(--color-fg-0);
    font-size: 13px;
  }
  .btn-primary {
    padding: 10px 16px;
    border-radius: 10px;
    border: none;
    background: var(--color-primary);
    color: white;
    font-weight: 600;
    font-size: 13px;
    cursor: pointer;
  }
  .btn-primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .device-box {
    margin-top: 1rem;
    padding: 1rem;
    border: 1px dashed var(--color-border);
    border-radius: 10px;
    background: var(--color-bg-body);
  }
  .code-row {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    margin: 8px 0;
  }
  .user-code {
    font-size: 1.35rem;
    letter-spacing: 0.12em;
    font-weight: 700;
    color: var(--color-fg-0);
  }
  .import-ta,
  .import-in {
    width: 100%;
    margin: 0.35rem 0;
    padding: 0.5rem;
    border-radius: 8px;
    border: 1px solid var(--color-border);
    background: var(--color-bg-card);
    font-family: var(--font-mono);
    font-size: 0.8rem;
  }
  .card-h {
    margin: 0 0 8px;
    font-size: 15px;
    font-weight: 600;
  }
  .card-head-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
  }
  .link-sm {
    font-size: 12px;
    color: var(--color-primary);
  }
  .session-list {
    list-style: none;
    padding: 0;
    margin: 0;
  }
  .session-row {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    align-items: center;
    padding: 12px 0;
    border-bottom: 1px solid var(--color-border);
  }
  .session-main {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
    flex: 1;
    min-width: 140px;
  }
  .session-provider {
    font-weight: 700;
    font-size: 14px;
    text-transform: lowercase;
  }
  .session-status {
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 6px;
    background: var(--color-bg-3);
    color: var(--color-fg-3);
  }
  .session-status.active {
    background: rgba(34, 197, 94, 0.12);
    color: #22c55e;
  }
  .session-id {
    font-size: 10px;
    color: var(--color-fg-4);
  }
  .session-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
  .btn-ghost-danger {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 8px 10px;
    border: 1px solid rgba(239, 68, 68, 0.25);
    background: transparent;
    color: var(--color-error);
    border-radius: 9px;
    font-size: 12px;
    cursor: pointer;
  }
  .muted {
    color: var(--color-fg-3);
  }
  .small {
    font-size: 0.85rem;
  }
  .block {
    display: block;
    margin: 0.35rem 0;
    padding: 0.35rem 0.5rem;
    background: var(--color-bg-3);
    border-radius: 6px;
    font-size: 0.8rem;
    font-family: var(--font-mono);
  }
  .link {
    color: var(--color-primary);
  }
  code {
    font-size: 0.85em;
  }
  .disabled-card h3 {
    margin: 0 0 8px;
  }
  .hint-import {
    font-size: 11px;
  }
</style>