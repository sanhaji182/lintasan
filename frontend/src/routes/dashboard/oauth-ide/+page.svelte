<script lang="ts">
  import TabNav from '$lib/components/TabNav.svelte';
  const __tabs = [
    { label: 'Accounts', path: '/dashboard/connections' },
    { label: 'Discover', path: '/dashboard/discover' },
    { label: 'OAuth IDE', path: '/dashboard/oauth-ide', lab: true },
    { label: 'Experimental', path: '/dashboard/experimental', lab: true }
  ];
  import { onDestroy, onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import { showToast } from '$lib/toast';
  import { brandForProvider, logoPaths } from '$lib/oauthIdeBrands';
  import { presetByProvider } from '$lib/oauthIdePresets';
  import {
    accountsForProvider,
    healthLabel,
    providerAccountState,
    quotaLabel,
    wireState,
    wireStateLabel,
    type OAuthAccount,
    type OAuthAccountsResponse,
    type OAuthProviderAccounts
  } from '$lib/oauthAccounts';
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

  let status = $state<OAuthStatus | null>(null);
  let accounts = $state<OAuthAccount[]>([]);
  let accountProviders = $state<OAuthProviderAccounts[]>([]);
  let loading = $state(true);
  let actionLoading = $state('');
  let selectedProvider = $state('xai');
  let deviceInfo = $state<any>(null);
  let pollSessionId = $state('');
  let lastRedirect = $state('');
  let waitingProvider = $state('');
  let flowStatus = $state('');
  let error = $state('');
  let cursorToken = $state('');
  let cursorMachineId = $state('');
  let copiedCode = $state(false);
  let xaiLoopbackWarn = $state('');
  let xaiCallbackPaste = $state('');
  let showXaiComplete = $state(false);
  let pollTimer: ReturnType<typeof setTimeout> | undefined;
  let pollDeadline = 0;
  let pollInFlight = false;
  let flowGeneration = 0;
  let providerActiveBefore = 0;

  const catalog = $derived(status?.catalog ?? []);
  const readyProviders = $derived(catalog.filter((p) => p.implementation === 'ready'));
  const importOnly = $derived(catalog.filter((p) => p.implementation === 'import_only'));
  const activeSessions = $derived(accounts.filter((account) => account.health === 'healthy' || account.health === 'expiring').length);

  const summary = $derived({
    catalog: catalog.length,
    ready: readyProviders.length,
    sessions: accounts.length,
    active: activeSessions
  });

  function providerAccounts(id: string) {
    return accountsForProvider(accounts, id);
  }

  function providerState(id: string) {
    return providerAccountState(accountProviders, id);
  }

  function sessionForProvider(id: string) {
    return accounts.find(
      (account) => account.provider === id && (account.health === 'healthy' || account.health === 'expiring')
    );
  }

  function implLabel(impl: string) {
    if (impl === 'ready') return { label: 'Ready', color: '#22c55e', bg: 'rgba(34,197,94,0.12)' };
    if (impl === 'import_only') return { label: 'Import', color: '#f59e0b', bg: 'rgba(245,158,11,0.12)' };
    return { label: impl, color: 'var(--color-fg-3)', bg: 'var(--color-bg-hover)' };
  }

  async function refreshAccounts() {
    const accountData = await api.get<OAuthAccountsResponse>('/api/oauth/accounts');
    accounts = accountData.accounts ?? [];
    accountProviders = accountData.providers ?? [];
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const st = await api.get<OAuthStatus>('/api/oauth/status');
      status = st;
      if (st.enabled) {
        await refreshAccounts();
      } else {
        accounts = [];
        accountProviders = [];
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

  function stopAutoPoll() {
    if (pollTimer) clearTimeout(pollTimer);
    pollTimer = undefined;
    flowGeneration += 1;
    pollInFlight = false;
  }

  function navigatePopup(popup: Window | null, url: string) {
    if (!popup || popup.closed || !url) return;
    try {
      popup.location.href = url;
    } catch {
      // The manual link remains visible if browser policy rejects navigation.
    }
  }

  function schedulePoll(callback: () => void, seconds: number, generation: number) {
    if (generation !== flowGeneration) return;
    if (Date.now() >= pollDeadline) {
      waitingProvider = '';
      flowStatus = '';
      error = 'Authorization expired. Start the connection again.';
      stopAutoPoll();
      return;
    }
    const boundedSeconds = Math.min(10, Math.max(1, seconds || 5));
    pollTimer = setTimeout(callback, boundedSeconds * 1000);
  }

  function activeAccountCount(provider: string) {
    const state = providerAccountState(accountProviders, provider);
    return (state?.active ?? 0) + (state?.expiring ?? 0);
  }

  async function pollBrowserAccount(provider: string, generation: number) {
    if (generation !== flowGeneration || pollInFlight) return;
    pollInFlight = true;
    try {
      await refreshAccounts();
      if (activeAccountCount(provider) > providerActiveBefore) {
        waitingProvider = '';
        flowStatus = '';
        if (pollTimer) clearTimeout(pollTimer);
        pollTimer = undefined;
        showToast('Account connected', 'success');
        return;
      }
    } catch {
      // A transient refresh failure should not cancel an in-progress OAuth callback.
    } finally {
      pollInFlight = false;
    }
    schedulePoll(() => pollBrowserAccount(provider, generation), 2, generation);
  }

  async function authorize(provider: string) {
    stopAutoPoll();
    selectedProvider = provider;
    providerActiveBefore = activeAccountCount(provider);
    const authPopup = window.open('', '_blank');
    if (authPopup) {
      try {
        authPopup.opener = null;
      } catch {
        // Best effort; navigation still happens in the reserved click-opened window.
      }
    }
    actionLoading = 'authorize-' + provider;
    waitingProvider = provider;
    flowStatus = 'Waiting for authorization…';
    error = '';
    lastRedirect = '';
    xaiLoopbackWarn = '';
    showXaiComplete = false;
    try {
      const res = await api.post<any>('/api/oauth/authorize', {
        provider,
        acknowledge_risk: true
      });
      pollSessionId = res.session_id || '';
      if (res.flow === 'device_code' && res.device) {
        deviceInfo = res.device;
        lastRedirect = res.device.verification_uri_complete || res.device.verification_uri || '';
        navigatePopup(authPopup, lastRedirect);
        pollDeadline = Date.now() + Math.max(1, Number(res.device.expires_in) || 900) * 1000;
        const generation = flowGeneration;
        schedulePoll(() => pollDevice(generation), Number(res.device.interval) || 5, generation);
      } else {
        deviceInfo = null;
        lastRedirect = res.redirect_url || '';
        xaiLoopbackWarn = res.xai_loopback_warn || '';
        if (provider === 'xai') {
          showXaiComplete = !!res.xai_manual_complete;
        }
        navigatePopup(authPopup, lastRedirect);
        pollDeadline = Date.now() + 5 * 60 * 1000;
        const generation = flowGeneration;
        schedulePoll(() => pollBrowserAccount(provider, generation), 2, generation);
      }
      if (xaiLoopbackWarn) {
        showToast('xAI login URL opened — see warning below', 'info');
      } else {
        showToast('OAuth flow started', 'info');
      }
    } catch (e: any) {
      error = e?.message || 'Authorize failed';
      waitingProvider = '';
      flowStatus = '';
      stopAutoPoll();
      if (authPopup && !authPopup.closed) authPopup.close();
    } finally {
      actionLoading = '';
    }
  }

  async function pollDevice(generation = flowGeneration) {
    if (!pollSessionId || generation !== flowGeneration || pollInFlight) return;
    if (pollTimer) clearTimeout(pollTimer);
    pollTimer = undefined;
    pollInFlight = true;
    actionLoading = 'poll';
    error = '';
    try {
      const res = await api.post<any>(`/api/oauth/device/poll?session_id=${encodeURIComponent(pollSessionId)}`, {});
      if (res.status === 'active') {
        deviceInfo = null;
        waitingProvider = '';
        flowStatus = '';
        if (pollTimer) clearTimeout(pollTimer);
        pollTimer = undefined;
        showToast('Device login complete', 'success');
        await refreshAccounts();
      } else {
        flowStatus = res.hint
          ? `Waiting for authorization… ${res.hint}`
          : 'Waiting for authorization…';
        schedulePoll(() => pollDevice(generation), Number(deviceInfo?.interval) || 5, generation);
      }
    } catch (e: any) {
      const message = e?.message || 'Poll failed';
      if (/authorization_pending|slow_down|pending/i.test(message)) {
        flowStatus = 'Waiting for authorization…';
        schedulePoll(() => pollDevice(generation), Number(deviceInfo?.interval) || 5, generation);
      } else {
        error = message;
        waitingProvider = '';
        flowStatus = '';
        stopAutoPoll();
      }
    } finally {
      pollInFlight = false;
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
        `${res.action === 'created' ? 'Created' : 'Updated'} ${res.name || preset?.name}; verifying live wire state…`,
        'info',
        4000
      );
      await load();
      const current = providerAccountState(accountProviders, provider);
      if (wireState(current) === 'healthy') {
        showToast(`${current?.connection_name || res.name || preset?.name} is wired with an active account`, 'success', 4000);
      } else {
        showToast(
          current?.wired
            ? 'Connection is wired, but no healthy OAuth account is available'
            : 'Connection was saved, but wire state could not be confirmed',
          'error',
          5000
        );
      }
    } catch (e: any) {
      error = e?.message || 'Wire proxy failed';
      showToast(error, 'error');
    } finally {
      actionLoading = '';
    }
  }

  onMount(load);
  onDestroy(stopAutoPoll);
</script>

<svelte:head><title>OAuth IDE (Experimental) — Lintasan</title></svelte:head>

<TabNav tabs={__tabs} />


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
    <span class="step"><span class="step-n">1</span> Connect provider</span>
    <ArrowRight size={14} class="step-arrow" />
    <span class="step"><span class="step-n">2</span> Finish authorization</span>
    <ArrowRight size={14} class="step-arrow" />
    <span class="step"><span class="step-n">3</span> Wire separately when ready</span>
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
          {@const providerSummary = providerState(p.id)}
          {@const state = wireState(providerSummary)}
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
                <span class="live-chip"><span class="live-dot"></span> {providerSummary?.active ?? 0} active</span>
              {/if}
              <span class="wire-chip {state}">{wireStateLabel(providerSummary)}</span>
            </div>
            {#if p.notes}
              <p class="note muted">{p.notes}</p>
            {/if}
            <div class="card-actions">
              {#if p.implementation === 'ready'}
                <button
                  type="button"
                  class="btn-primary-sm"
                  aria-label={sess ? `Add another ${p.name} account` : `Connect ${p.name}`}
                  disabled={!status?.enabled || actionLoading.startsWith('authorize-')}
                  onclick={() => authorize(p.id)}
                >
                  <KeyRound size={14} />
                  {actionLoading === 'authorize-' + p.id ? 'Starting…' : sess ? 'Add another account' : 'Connect'}
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
            {#if waitingProvider === p.id}
              <div class="waiting-status" role="status">
                <Spinner /> <span>{flowStatus || 'Waiting for authorization…'}</span>
              </div>
            {/if}
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
      <section class="card flow-card">
        <h3 class="card-h">Authorization details</h3>
        <p class="muted small">Choose a provider card above. The authorization page opens immediately; account completion and proxy wiring remain separate.</p>
        <p class="muted small">Callback base: <code>{status.public_base}</code></p>
        {#if status.xai_note}
          <p class="muted small xai-loopback-note">
            <strong>xAI:</strong> redirect <code>{status.xai_redirect_uri}</code> — {status.xai_note}
          </p>
        {/if}
        {#if status.hint}<p class="muted small">{status.hint}</p>{/if}

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
            <button type="button" class="btn-secondary-sm" disabled={actionLoading === 'poll'} onclick={() => pollDevice()}>
              {actionLoading === 'poll' ? 'Checking…' : 'Check again'}
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
              disabled={!cursorToken.trim() || !cursorMachineId.trim() || actionLoading === 'cursor-import'}
              onclick={importCursor}
            >
              {actionLoading === 'cursor-import' ? 'Importing…' : 'Import Cursor session'}
            </button>
          </div>
        {/if}
      </section>

      <section class="card">
        <div class="card-head-row">
          <div>
            <h3 class="card-h">OAuth accounts</h3>
            <p class="muted small account-intro">Health is reported per account. Restricted and expired accounts remain visible.</p>
          </div>
          <a href="/dashboard/connections" class="link-sm">Connections →</a>
        </div>
        {#if accountProviders.length === 0 && accounts.length === 0}
          <p class="muted">No OAuth accounts yet. Authorize a provider above.</p>
        {:else}
          <div class="account-provider-list">
            {#each accountProviders as provider}
              {@const brand = brandForProvider(provider.provider)}
              {@const state = wireState(provider)}
              {@const providerRows = providerAccounts(provider.provider)}
              <article class="account-provider-card">
                <div class="account-provider-head">
                  <div>
                    <div class="account-provider-title" style="color: {brand.color}">{provider.name || provider.provider}</div>
                    <div class="account-counts">
                      <span class="count active">{provider.active} active</span>
                      {#if provider.expiring > 0}<span class="count expiring">{provider.expiring} expiring</span>{/if}
                      {#if provider.restricted > 0}<span class="count restricted">{provider.restricted} restricted</span>{/if}
                      {#if provider.expired > 0}<span class="count expired">{provider.expired} expired</span>{/if}
                      {#if provider.revoked > 0}<span class="count revoked">{provider.revoked} revoked</span>{/if}
                    </div>
                  </div>
                  <div class="provider-wire-state">
                    <span class="wire-chip {state}">{wireStateLabel(provider)}</span>
                    <span class="quota-label">Quota <strong>{quotaLabel()}</strong></span>
                  </div>
                </div>
                {#if provider.wired && provider.connection_name}
                  <p class="connection-note">Connection: <strong>{provider.connection_name}</strong></p>
                {/if}
                <ul class="session-list account-list">
                  {#each providerRows as account}
                    <li class="session-row account-row">
                      <div class="session-main">
                        <span class="health-dot {account.health}" aria-hidden="true"></span>
                        <span class="session-provider">{healthLabel(account.health)}</span>
                        {#if account.masked_token}<code class="masked-token">{account.masked_token}</code>{/if}
                        {#if account.expires_at}<span class="muted small">exp {account.expires_at}</span>{/if}
                      </div>
                      <code class="session-id">{account.id.slice(0, 8)}…</code>
                      <div class="session-actions">
                        <button
                          type="button"
                          class="btn-ghost-danger"
                          disabled={actionLoading === account.id || account.status === 'revoked'}
                          onclick={() => revoke(account.id)}
                        >
                          <Trash2 size={14} /> {account.status === 'revoked' ? 'Revoked' : 'Revoke'}
                        </button>
                      </div>
                    </li>
                  {/each}
                </ul>
                <div class="account-actions">
                  <button
                    type="button"
                    class="btn-secondary-sm"
                    disabled={provider.active + provider.expiring === 0 || actionLoading === 'wire-' + provider.provider}
                    onclick={() => wireProxy(provider.provider)}
                  >
                    <Link2 size={14} />
                    {actionLoading === 'wire-' + provider.provider ? 'Verifying…' : 'Wire / verify'}
                  </button>
                </div>
              </article>
            {/each}
          </div>
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
    color: var(--color-fg-3);
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
    background: var(--color-bg-hover);
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
  .waiting-status {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 12px;
    padding: 8px 10px;
    border-radius: 8px;
    background: rgba(59, 130, 246, 0.08);
    color: var(--color-fg-2);
    font-size: 12px;
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
    background: var(--color-bg-hover);
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
    background: var(--color-bg-hover);
    color: var(--color-fg-3);
  }
  .session-status.active {
    background: rgba(34, 197, 94, 0.12);
    color: #22c55e;
  }
  .session-id {
    font-size: 10px;
    color: var(--color-fg-3);
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
  .btn-ghost-danger:disabled {
    cursor: not-allowed;
    opacity: 0.5;
  }
  .wire-chip {
    display: inline-flex;
    align-items: center;
    padding: 3px 8px;
    border-radius: 999px;
    border: 1px solid var(--color-border);
    background: var(--color-bg-hover);
    color: var(--color-fg-3);
    font-size: 10px;
    font-weight: 700;
  }
  .wire-chip.healthy {
    border-color: rgba(34, 197, 94, 0.3);
    background: rgba(34, 197, 94, 0.1);
    color: #15803d;
  }
  .wire-chip.error {
    border-color: rgba(245, 158, 11, 0.35);
    background: rgba(245, 158, 11, 0.1);
    color: #b45309;
  }
  .account-intro {
    margin: 0;
  }
  .account-provider-list {
    display: grid;
    gap: 12px;
  }
  .account-provider-card {
    border: 1px solid var(--color-border);
    background: var(--color-bg-card);
    border-radius: 12px;
    padding: 14px;
    box-shadow: 0 1px 2px rgba(15, 23, 42, 0.03);
  }
  .account-provider-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
  }
  .account-provider-title {
    font-size: 14px;
    font-weight: 750;
  }
  .account-counts {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 6px;
  }
  .count {
    padding: 2px 7px;
    border-radius: 999px;
    background: var(--color-bg-hover);
    color: var(--color-fg-3);
    font-size: 10px;
    font-weight: 650;
  }
  .count.active {
    color: #15803d;
    background: rgba(34, 197, 94, 0.1);
  }
  .count.expiring,
  .count.restricted {
    color: #b45309;
    background: rgba(245, 158, 11, 0.1);
  }
  .count.expired {
    color: #b91c1c;
    background: rgba(239, 68, 68, 0.08);
  }
  .provider-wire-state {
    display: flex;
    align-items: flex-end;
    flex-direction: column;
    gap: 6px;
  }
  .quota-label,
  .connection-note {
    color: var(--color-fg-3);
    font-size: 11px;
  }
  .connection-note {
    margin: 8px 0 0;
  }
  .account-list {
    margin-top: 8px;
    border-top: 1px solid var(--color-border);
  }
  .account-row:last-child {
    border-bottom: 0;
  }
  .health-dot {
    width: 8px;
    height: 8px;
    flex: 0 0 auto;
    border-radius: 50%;
    background: #94a3b8;
  }
  .health-dot.healthy {
    background: #22c55e;
    box-shadow: 0 0 0 3px rgba(34, 197, 94, 0.1);
  }
  .health-dot.expiring,
  .health-dot.restricted {
    background: #f59e0b;
  }
  .health-dot.expired,
  .health-dot.revoked {
    background: #ef4444;
  }
  .masked-token {
    font-size: 10px;
    color: var(--color-fg-2);
  }
  .account-actions {
    display: flex;
    justify-content: flex-end;
    padding-top: 10px;
  }
  @media (max-width: 640px) {
    .account-provider-head {
      flex-direction: column;
    }
    .provider-wire-state {
      align-items: flex-start;
    }
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
    background: var(--color-bg-hover);
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