<script lang="ts">
  import TabNav from '$lib/components/TabNav.svelte';
  const __tabs = [
    { label: 'Accounts', path: '/dashboard/connections' },
    { label: 'Discover Free', path: '/dashboard/discover' },
    { label: 'OAuth Providers', path: '/dashboard/oauth-ide' }
  ];
  import { onDestroy, onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import HelpTooltip from '$lib/components/HelpTooltip.svelte';
  import { showToast } from '$lib/toast';
  import { brandForProvider, logoPaths } from '$lib/oauthIdeBrands';
  import { presetByProvider } from '$lib/oauthIdePresets';
  import {
    accountsForProvider,
    flowDescription,
    flowLabel,
    formatAge,
    expiryLabel,
    healthDescription,
    healthLabel,
    providerAccountState,
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
    RefreshCw,
    Search
  } from 'lucide-svelte';

  interface CatalogEntry {
    id: string;
    name: string;
    flow: string;
    implementation: string;
    deprecated?: boolean;
    deprecation_note?: string;
    risk_notice?: string;
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
  let antigravityCallbackPaste = $state('');
  let showAntigravityComplete = $state(false);
  let pollTimer: ReturnType<typeof setTimeout> | undefined;
  let pollDeadline = 0;
  let pollInFlight = false;
  let flowGeneration = 0;
  let providerActiveBefore = 0;
  let providerSearch = $state('');
  let providerFilter = $state<'connectable' | 'ready' | 'import' | 'all' | 'planned'>('connectable');
  let pendingRevoke = $state('');
  let activePopup: Window | null = null;
  let lastLoadedAt = $state<number | null>(null);
  let copiedSetup = $state('');
  let showCursorImport = $state(false);

  const catalog = $derived(status?.catalog ?? []);
  const readyProviders = $derived(catalog.filter((p) => p.implementation === 'ready'));
  const importOnly = $derived(catalog.filter((p) => p.implementation === 'import_only'));
  const connectableProviders = $derived([...readyProviders, ...importOnly]);
  const plannedProviders = $derived(
    catalog.filter((p) => p.implementation !== 'ready' && p.implementation !== 'import_only')
  );
  const activeSessions = $derived(accounts.filter((account) => account.health === 'healthy' || account.health === 'expiring').length);
  const attentionCount = $derived(
    accountProviders.reduce((sum, p) => sum + p.restricted + p.expired + p.revoked, 0)
  );

  const summary = $derived({
    available: connectableProviders.length,
    connected: accountProviders.length,
    sessions: accounts.length,
    active: activeSessions,
    attention: attentionCount
  });

  const filterCounts = $derived({
    connectable: connectableProviders.length,
    ready: readyProviders.length,
    import: importOnly.length,
    planned: plannedProviders.length,
    all: catalog.length
  });

  const filteredCatalog = $derived.by(() => {
    let list: CatalogEntry[];
    switch (providerFilter) {
      case 'ready':
        list = readyProviders;
        break;
      case 'import':
        list = importOnly;
        break;
      case 'planned':
        list = plannedProviders;
        break;
      case 'all':
        list = catalog;
        break;
      default:
        list = connectableProviders;
    }
    const term = providerSearch.trim().toLowerCase();
    if (term) {
      list = list.filter((p) => {
        const brand = brandForProvider(p.id);
        return [p.id, p.name, p.flow, brand.company, brand.tagline].join(' ').toLowerCase().includes(term);
      });
    }
    const rank = (p: CatalogEntry) =>
      p.implementation === 'ready' ? 0 : p.implementation === 'import_only' ? 1 : 2;
    return [...list].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name));
  });

  function scrollToFlow() {
    setTimeout(() => {
      document.getElementById('oauth-flow-panel')?.scrollIntoView?.({ behavior: 'smooth', block: 'start' });
    }, 50);
  }

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
    if (impl === 'import_only') return { label: 'Import', color: '#3b82f6', bg: 'rgba(59,130,246,0.12)' };
    return { label: 'Coming Soon', color: 'var(--color-fg-3)', bg: 'var(--color-bg-hover)' };
  }

  async function copyValue(text: string, key: string) {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      copiedSetup = key;
      showToast('Copied to clipboard', 'success', 1800);
      setTimeout(() => {
        if (copiedSetup === key) copiedSetup = '';
      }, 1800);
    } catch {
      showToast('Copy failed', 'error');
    }
  }

  function cancelFlow() {
    stopAutoPoll();
    waitingProvider = '';
    flowStatus = '';
    deviceInfo = null;
    lastRedirect = '';
    xaiLoopbackWarn = '';
    showXaiComplete = false;
    showAntigravityComplete = false;
    pollSessionId = '';
    if (activePopup && !activePopup.closed) activePopup.close();
    activePopup = null;
    showToast('Authorization cancelled', 'info');
  }

  const healthRank = (health: string) =>
    (({ restricted: 0, expired: 1, expiring: 2, healthy: 3, revoked: 4 }) as Record<string, number>)[health] ?? 5;

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
      lastLoadedAt = Date.now();
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
        activePopup = null;
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
    activePopup = authPopup;
    actionLoading = 'authorize-' + provider;
    waitingProvider = provider;
    flowStatus = 'Waiting for authorization…';
    error = '';
    scrollToFlow();
    lastRedirect = '';
    xaiLoopbackWarn = '';
    showXaiComplete = false;
    showAntigravityComplete = false;
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
        if (provider === 'antigravity') {
          showAntigravityComplete = !!res.antigravity_manual_complete;
        }
        navigatePopup(authPopup, lastRedirect);
        pollDeadline = Date.now() + 5 * 60 * 1000;
        const generation = flowGeneration;
        schedulePoll(() => pollBrowserAccount(provider, generation), 2, generation);
      }
      if (xaiLoopbackWarn) {
        showToast(provider === 'antigravity' ? 'Antigravity login URL opened — see note below' : 'xAI login URL opened — see warning below', 'info');
      } else {
        showToast('OAuth flow started', 'info');
      }
    } catch (e: any) {
      error = e?.message || 'Authorize failed';
      waitingProvider = '';
      flowStatus = '';
      stopAutoPoll();
      if (authPopup && !authPopup.closed) authPopup.close();
      activePopup = null;
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
        activePopup = null;
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

  async function completeAntigravityLogin() {
    actionLoading = 'antigravity-complete';
    error = '';
    try {
      await api.post('/api/oauth/antigravity/complete', { callback_url: antigravityCallbackPaste });
      showToast('Antigravity session active', 'success');
      antigravityCallbackPaste = '';
      showAntigravityComplete = false;
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
    pendingRevoke = '';
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

<svelte:head><title>OAuth Providers — Lintasan</title></svelte:head>

<TabNav tabs={__tabs} />


<div class="oauth-page">
  <div class="section-header">
    <div class="header-icon">
      <KeyRound size={22} />
    </div>
    <div class="header-text">
      <h1 class="header-title">
        OAuth Providers
        {#if status?.enabled}
          <span class="lab-pill on"><FlaskConical size={11} /> Lab active</span>
        {:else}
          <span class="lab-pill off">Lab off</span>
        {/if}
      </h1>
      <p class="header-desc">
        Sign in to provider subscriptions and turn them into gateway connections. Experimental — use only accounts
        you are authorized to connect.
      </p>
    </div>
    <div class="header-actions">
      {#if lastLoadedAt}
        <span class="updated-at">Updated {new Date(lastLoadedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
      {/if}
      <button class="btn-icon" onclick={load} disabled={loading} title="Refresh" aria-label="Refresh providers">
        <RefreshCw size={18} class={loading ? 'spin' : ''} />
      </button>
    </div>
  </div>

  <div class="stats-strip">
    {#each [
      { label: 'Available now', value: summary.available, tone: 'ready', hint: 'Providers you can connect today (Ready + Import).' },
      { label: 'Connected providers', value: summary.connected, tone: 'neutral', hint: 'Providers with at least one OAuth session or wired connection.' },
      { label: 'Active accounts', value: summary.active, tone: 'ok', hint: 'Accounts that are healthy or still automatically refreshable.' },
      { label: 'Needs attention', value: summary.attention, tone: summary.attention > 0 ? 'warn' : 'neutral', hint: 'Restricted, expired, or revoked accounts.' }
    ] as stat}
      <div class="stat-cell" title={stat.hint}>
        <div class="stat-value {stat.tone}">{stat.value}</div>
        <div class="stat-label">{stat.label}</div>
      </div>
    {/each}
  </div>

  <div class="steps-strip">
    <span class="step"><span class="step-n">1</span> Add an account — sign in or paste a token</span>
    <ArrowRight size={14} class="step-arrow" />
    <span class="step"><span class="step-n">2</span> Verify it shows <strong>healthy</strong></span>
    <ArrowRight size={14} class="step-arrow" />
    <span class="step">
      <span class="step-n">3</span> Wire a connection
      <HelpTooltip
        content="Wire creates or updates a gateway connection that uses this account. Until you wire it, the account exists but no model routes through it."
      />
    </span>
  </div>

  {#if status?.disclaimer}
    <div class="risk-banner" role="note">
      <ShieldAlert size={18} />
      <div>
        <strong>Experimental — bring your own subscription.</strong>
        <p>{status.disclaimer}</p>
      </div>
    </div>
  {/if}

  {#if error}
    <div class="error-banner" role="alert" aria-live="assertive">
      <CircleAlert size={16} />
      <span>{error}</span>
      <button type="button" class="error-close" aria-label="Dismiss error" onclick={() => (error = '')}><X size={14} /></button>
    </div>
  {/if}

  {#if loading}
    <div class="loading-state"><Spinner /><p>Loading OAuth Providers…</p></div>
  {:else}
    {#if catalog.length > 0}
      <h2 class="section-title">Provider catalog</h2>
      <p class="section-sub muted">
        {summary.available} can be connected now · {plannedProviders.length} catalogued but not implemented yet
      </p>

      <div class="catalog-toolbar">
        <div class="search-box">
          <Search size={15} />
          <input
            id="provider-search"
            type="search"
            placeholder="Search provider, company, or flow…"
            aria-label="Search providers"
            bind:value={providerSearch}
          />
        </div>
        <div class="filter-tabs" role="group" aria-label="Filter providers">
          {#each [
            { id: 'connectable', label: 'Connectable' },
            { id: 'ready', label: 'Ready' },
            { id: 'import', label: 'Import' },
            { id: 'planned', label: 'Coming soon' },
            { id: 'all', label: 'All' }
          ] as f}
            <button
              type="button"
              class="filter-tab"
              aria-pressed={providerFilter === f.id}
              onclick={() => (providerFilter = f.id as typeof providerFilter)}
            >
              {f.label}
              <span class="filter-count">{filterCounts[f.id as keyof typeof filterCounts]}</span>
            </button>
          {/each}
        </div>
      </div>

      <p class="catalog-help muted">
        <strong>Connect</strong> adds a login. <strong>Wire</strong> creates or updates the gateway connection so models can
        use it.
      </p>

      <div class="provider-grid">
        {#each filteredCatalog as p}
          {@const brand = brandForProvider(p.id)}
          {@const impl = implLabel(p.implementation)}
          {@const sess = sessionForProvider(p.id)}
          {@const providerSummary = providerState(p.id)}
          {@const hasAccounts = (providerSummary?.total ?? 0) > 0}
          {@const state = wireState(providerSummary)}
          <article
            class="provider-card"
            class:planned-card={p.implementation !== 'ready' && p.implementation !== 'import_only'}
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
            <p class="flow-help"><strong>{flowLabel(p.flow)}</strong> — {flowDescription(p.flow)}</p>
            <div class="meta-row">
              <code class="id-chip" title="Provider id used by the API">{p.id}</code>
              {#if hasAccounts}
                <span class="live-chip"><span class="live-dot"></span>{providerSummary?.active ?? 0} active</span>
              {/if}
              {#if p.implementation === 'ready' || p.implementation === 'import_only'}
                <span class="wire-chip {state}">{wireStateLabel(providerSummary)}</span>
              {/if}
              {#if p.deprecated || p.risk_notice}
                <span class="risk-chip" title={p.deprecation_note || p.risk_notice || 'Experimental provider'}>
                  Risk notice
                </span>
              {/if}
            </div>
            {#if p.notes}
              <p class="note muted">{p.notes}</p>
            {/if}
            {#if hasAccounts}
              <p class="account-summary">
                {providerSummary?.total ?? 0} account{(providerSummary?.total ?? 0) === 1 ? '' : 's'} ·
                {providerSummary?.active ?? 0} active
                {#if (providerSummary?.restricted ?? 0) > 0} · {providerSummary?.restricted} restricted{/if}
                {#if (providerSummary?.expired ?? 0) > 0} · {providerSummary?.expired} expired{/if}
              </p>
            {/if}
            <div class="card-actions">
              {#if p.implementation === 'ready'}
                <button
                  type="button"
                  class="btn-primary-sm"
                  aria-label={hasAccounts ? `Add another ${p.name} account` : `Connect ${p.name}`}
                  disabled={!status?.enabled || actionLoading.startsWith('authorize-')}
                  onclick={() => authorize(p.id)}
                >
                  <KeyRound size={14} />
                  {actionLoading === 'authorize-' + p.id ? 'Starting…' : hasAccounts ? 'Add another account' : 'Connect'}
                </button>
              {:else if p.implementation === 'import_only'}
                <button
                  type="button"
                  class="btn-primary-sm"
                  onclick={() => {
                    showCursorImport = true;
                    setTimeout(() => {
                      document.getElementById('cursor-import')?.scrollIntoView?.({ behavior: 'smooth', block: 'center' });
                    }, 50);
                  }}
                >
                  <KeyRound size={14} /> Import token
                </button>
              {:else}
                <span class="hint-import muted">Not implemented yet</span>
              {/if}
              {#if hasAccounts}
                <button
                  type="button"
                  class="btn-secondary-sm"
                  disabled={actionLoading === 'wire-' + p.id || (providerSummary?.active ?? 0) + (providerSummary?.expiring ?? 0) === 0}
                  title={
                    (providerSummary?.active ?? 0) + (providerSummary?.expiring ?? 0) === 0
                      ? 'Needs a healthy account before it can be wired'
                      : 'Create or update the gateway connection for this provider'
                  }
                  onclick={() => wireProxy(p.id)}
                >
                  <Link2 size={14} />
                  {actionLoading === 'wire-' + p.id ? 'Verifying…' : sess ? 'Wire / verify' : 'Wire'}
                </button>
              {/if}
              {#if providerSummary?.wired}
                <a class="btn-secondary-sm" href="/dashboard/connections">Open connection</a>
              {/if}
            </div>
          </article>
        {/each}
      </div>

      {#if filteredCatalog.length === 0}
        <div class="empty-catalog">
          <Search size={22} />
          <p>No providers match “{providerSearch}”.</p>
          <button
            type="button"
            class="btn-secondary-sm"
            onclick={() => {
              providerSearch = '';
              providerFilter = 'all';
            }}
          >
            Show all providers
          </button>
        </div>
      {/if}

      {#if providerFilter !== 'planned' && plannedProviders.length > 0 && !providerSearch.trim()}
        <p class="planned-hint muted">
          {plannedProviders.length} providers are catalogued but not implemented yet.
          <button type="button" class="link-btn" onclick={() => (providerFilter = 'planned')}>See what's coming</button>
        </p>
      {/if}
    {/if}

    {#if !status?.enabled}
      <section class="card disabled-card">
        <div class="card-head-row">
          <div>
            <h3 class="card-h">OAuth Providers is turned off</h3>
            <p class="muted small">The catalog is visible, but Connect stays disabled until the lab is enabled.</p>
          </div>
          <span class="state-pill" style="background: var(--color-bg-hover); color: var(--color-fg-2)">Lab off</span>
        </div>
        <ol class="setup-steps">
          <li>
            <strong>Enable the lab.</strong> Open
            <a class="link" href="/dashboard/settings">Settings → Experimental</a> and turn on
            <code>OAuth IDE (lab)</code>.
          </li>
          <li>
            <strong>Set the public callback base.</strong> Add <code>LINTASAN_OAUTH_PUBLIC_BASE_URL</code> on the
            server, for example <code>https://lintasan.example.com</code>.
            <button
              type="button"
              class="copy-chip"
              onclick={() => copyValue('LINTASAN_OAUTH_PUBLIC_BASE_URL', 'public-base')}
            >
              {#if copiedSetup === 'public-base'}<Check size={12} />{:else}<Copy size={12} />{/if}
              Copy variable
            </button>
          </li>
          <li>
            <strong>Optional per-provider clients.</strong> Providers that need their own OAuth client use
            <code>LINTASAN_OAUTH_IDE_*_CLIENT_ID</code> and the matching secret. Built-in public clients need no setup.
          </li>
        </ol>
      </section>
    {:else}
      <section class="card flow-card" id="oauth-flow-panel" aria-live="polite">
        <div class="flow-head">
          {#if waitingProvider}
            {@const activeBrand = brandForProvider(waitingProvider)}
            {@const activeEntry = catalog.find((c) => c.id === waitingProvider)}
            <div class="brand-logo" style="background: {activeBrand.bg};">
              <svg viewBox="0 0 24 24" width="20" height="20">
                {#each logoPaths(activeBrand) as path}
                  <path d={path.d} fill={path.fill} />
                {/each}
              </svg>
            </div>
            <div>
              <h3 class="card-h">Authorizing {activeEntry?.name ?? waitingProvider}</h3>
              <p class="muted small">{flowLabel(activeEntry?.flow ?? '')}</p>
            </div>
          {:else}
            <div>
              <h3 class="card-h">Authorization details</h3>
              <p class="muted small">
                Pick a provider card above to start. The login window opens immediately; wiring the connection is a
                separate step.
              </p>
            </div>
          {/if}
        </div>

        {#if waitingProvider || deviceInfo}
          <div class="flow-progress" role="status">
            <Spinner />
            <span>{flowStatus || 'Waiting for authorization…'}</span>
            <button type="button" class="btn-ghost-sm" onclick={cancelFlow}>Cancel</button>
          </div>
        {/if}

        <details class="flow-meta">
          <summary>Connection settings &amp; callback</summary>
          <p class="muted small">Callback base: <code>{status.public_base || 'not set'}</code></p>
          {#if status.xai_note}
            <p class="muted small">
              <strong>xAI:</strong> redirect <code>{status.xai_redirect_uri}</code> — {status.xai_note}
            </p>
          {/if}
          {#if status.hint}<p class="muted small">{status.hint}</p>{/if}
        </details>

        {#if deviceInfo}
          <div class="device-box">
            <p class="device-title">Enter this code on the provider page</p>
            <div class="code-row">
              <code class="user-code">{deviceInfo.user_code}</code>
              <button type="button" class="btn-secondary-sm" onclick={copyUserCode}>
                {#if copiedCode}<Check size={14} />{:else}<Copy size={14} />{/if}
                Copy code
              </button>
            </div>
            <ol class="device-steps">
              <li>Open the verification page below.</li>
              <li>Paste the code and approve the login.</li>
              <li>This page checks automatically — you do not need to click anything.</li>
            </ol>
            <div class="device-actions">
              <a
                class="btn-primary-sm"
                href={deviceInfo.verification_uri_complete || deviceInfo.verification_uri}
                target="_blank"
                rel="noopener noreferrer"
              >
                Open verification page <ExternalLink size={14} />
              </a>
              <button type="button" class="btn-secondary-sm" disabled={actionLoading === 'poll'} onclick={() => pollDevice()}>
                {actionLoading === 'poll' ? 'Checking…' : 'Check again'}
              </button>
              <button type="button" class="btn-ghost-sm" onclick={cancelFlow}>Cancel</button>
            </div>
            {#if deviceInfo.expires_in}
              <p class="muted small">
                Code is valid for about {Math.max(1, Math.round(Number(deviceInfo.expires_in) / 60))} minutes.
              </p>
            {/if}
          </div>
        {/if}
        {#if lastRedirect}
          <p class="muted small">
            Login window opened. If you do not see it,
            <a href={lastRedirect} target="_blank" rel="noopener noreferrer">open the provider login page</a>.
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
              aria-label="xAI callback URL or authorization code"
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

        {#if showAntigravityComplete}
          <div class="device-box">
            <p><strong>Google Antigravity — complete login</strong></p>
            <p class="muted small">
              After signing in with Google, paste the authorization code (starting with <code>4/0A...</code>) OR the full address bar URL (<code>http://127.0.0.1:51121/callback?code=...&state=...</code>).
            </p>
            <textarea
              class="import-ta"
              aria-label="Google Antigravity callback URL or authorization code"
              placeholder="http://127.0.0.1:51121/callback?code=...&state=... or code (4/0A...)"
              bind:value={antigravityCallbackPaste}
              rows="2"
            ></textarea>
            <button
              type="button"
              class="btn-primary"
              disabled={!antigravityCallbackPaste.trim() || actionLoading === 'antigravity-complete'}
              onclick={completeAntigravityLogin}
            >
              {actionLoading === 'antigravity-complete' ? 'Completing…' : 'Complete Antigravity login'}
            </button>
          </div>
        {/if}

        {#if importOnly.length > 0 && showCursorImport}
          <div class="device-box import-box" id="cursor-import">
            <div class="import-head">
              <p class="device-title">Import a Cursor token</p>
              <button type="button" class="btn-ghost-sm" onclick={() => (showCursorImport = false)}>Hide</button>
            </div>
            <p class="muted small">
              Cursor does not use a browser login here. From Cursor's <code>state.vscdb</code>, copy
              <code>cursorAuth/accessToken</code> and <code>storage.serviceMachineId</code>.
            </p>
            <label class="field-label" for="cursor-token">Access token</label>
            <textarea
              id="cursor-token"
              class="import-ta"
              placeholder="cursorAuth/accessToken value"
              bind:value={cursorToken}
              rows="2"
            ></textarea>
            <label class="field-label" for="cursor-machine">Machine ID</label>
            <input
              id="cursor-machine"
              class="import-in"
              placeholder="storage.serviceMachineId value"
              bind:value={cursorMachineId}
            />
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
            <h3 class="card-h">Connected accounts</h3>
            <p class="muted small account-intro">
              {summary.sessions}
              {summary.sessions === 1 ? 'account' : 'accounts'} across {summary.connected}
              {summary.connected === 1 ? 'provider' : 'providers'}. Restricted and expired accounts stay visible so you
              can fix them.
            </p>
          </div>
          <a href="/dashboard/connections" class="link-sm">Connections →</a>
        </div>
        {#if accountProviders.length === 0 && accounts.length === 0}
          <div class="accounts-empty">
            <KeyRound size={22} />
            <p><strong>No accounts connected yet.</strong></p>
            <p class="muted small">
              Start with a Connectable provider above. Its account appears here as soon as the login finishes.
            </p>
            <button
              type="button"
              class="btn-secondary-sm"
              onclick={() => {
                providerFilter = 'connectable';
                document.getElementById('provider-search')?.focus();
              }}
            >
              Choose a provider
            </button>
          </div>
        {:else}
          <div class="account-provider-list">
            {#each accountProviders as provider}
              {@const brand = brandForProvider(provider.provider)}
              {@const state = wireState(provider)}
              {@const providerRows = [...providerAccounts(provider.provider)].sort((a, b) => healthRank(a.health) - healthRank(b.health))}
              <article class="account-provider-card">
                <div class="account-provider-head">
                  <div>
                    <div class="account-title-row">
                      <span class="brand-dot" style="background: {brand.color}; box-shadow: 0 0 6px {brand.color};"></span>
                      <div class="account-provider-title">{provider.name || provider.provider}</div>
                    </div>
                    <div class="account-counts">
                      {#if provider.active > 0}<span class="count active" title={healthDescription('healthy')}>{provider.active} active</span>{/if}
                      {#if provider.expiring > 0}<span class="count expiring" title={healthDescription('expiring')}>{provider.expiring} expiring</span>{/if}
                      {#if provider.restricted > 0}<span class="count restricted" title={healthDescription('restricted')}>{provider.restricted} restricted</span>{/if}
                      {#if provider.expired > 0}<span class="count expired" title={healthDescription('expired')}>{provider.expired} expired</span>{/if}
                      {#if provider.revoked > 0}<span class="count revoked" title={healthDescription('revoked')}>{provider.revoked} revoked</span>{/if}
                      {#if provider.total === 0}<span class="count">No accounts</span>{/if}
                    </div>
                  </div>
                  <div class="provider-wire-state">
                    <span class="wire-chip {state}">{wireStateLabel(provider)}</span>
                  </div>
                </div>
                {#if provider.wired && provider.connection_name}
                  <p class="connection-note">
                    Connection: <strong>{provider.connection_name}</strong> ·
                    <a href="/dashboard/connections" class="link-sm">Manage</a>
                  </p>
                {/if}
                {#if provider.wired && provider.active + provider.expiring === 0}
                  <p class="warn-note">
                    This provider is wired but has no usable account. Reconnect a session, then verify again.
                  </p>
                {/if}
                <ul class="session-list account-list">
                  {#each providerRows as account}
                    <li class="session-row account-row">
                      <div class="session-main">
                        <span class="health-dot {account.health}" aria-hidden="true"></span>
                        <div class="account-primary">
                          <div class="account-health-row">
                            <span class="session-provider">{healthLabel(account.health)}</span>
                            {#if account.masked_token}
                              <code class="masked-token" title="Masked token — secrets are never shown">{account.masked_token}</code>
                            {/if}
                          </div>
                          <p class="account-hint muted">{healthDescription(account.health)}</p>
                          <div class="account-meta">
                            {#if account.expires_at}
                              <span title={account.expires_at}>{expiryLabel(account.expires_at)}</span>
                            {:else}
                              <span>No expiry reported</span>
                            {/if}
                            {#if account.created_at}<span>· Added {formatAge(account.created_at)}</span>{/if}
                            <span>· ID {account.id.slice(0, 8)}…</span>
                          </div>
                        </div>
                      </div>
                      <div class="session-actions">
                        {#if pendingRevoke === account.id}
                          <button
                            type="button"
                            class="btn-ghost-danger"
                            disabled={actionLoading === account.id}
                            onclick={() => revoke(account.id)}
                          >
                            <Trash2 size={14} /> {actionLoading === account.id ? 'Revoking…' : 'Confirm revoke'}
                          </button>
                          <button type="button" class="btn-secondary-sm" onclick={() => (pendingRevoke = '')}>Keep</button>
                        {:else}
                          <button
                            type="button"
                            class="btn-ghost-danger"
                            disabled={actionLoading === account.id || account.status === 'revoked'}
                            onclick={() => (pendingRevoke = account.id)}
                          >
                            <Trash2 size={14} /> {account.status === 'revoked' ? 'Revoked' : 'Revoke'}
                          </button>
                        {/if}
                      </div>
                    </li>
                  {/each}
                </ul>
                <div class="account-actions">
                  <span class="account-action-hint muted">
                    {provider.active + provider.expiring === 0
                      ? 'Wire needs at least one healthy account.'
                      : 'Wire creates or updates the gateway connection for these accounts.'}
                  </span>
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
    max-width: 1080px;
    margin: 0 auto;
  }
  .section-header {
    display: flex;
    align-items: flex-start;
    gap: 14px;
    margin-bottom: 20px;
    flex-wrap: wrap;
  }
  .header-text {
    flex: 1;
    min-width: 0;
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
    margin-left: 0;
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
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
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
    justify-content: space-between;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
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
  .header-actions {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 10px;
    flex-shrink: 0;
  }
  .updated-at {
    font-size: 11px;
    color: var(--color-fg-3);
    white-space: nowrap;
  }
  .header-title {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }
  .lab-pill {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 10px;
    font-weight: 650;
    padding: 3px 8px;
    border-radius: 999px;
    border: 1px solid var(--color-border);
    color: var(--color-fg-2);
    background: var(--color-bg-hover);
  }
  .lab-pill.on {
    color: var(--color-success);
    border-color: rgba(5, 150, 105, 0.25);
    background: var(--color-success-light);
  }
  .stat-value.ready {
    color: var(--color-primary);
  }
  .stat-value.ok {
    color: var(--color-success);
  }
  .stat-value.warn {
    color: var(--color-warning);
  }
  .stat-value.neutral {
    color: var(--color-fg-0);
  }
  .risk-banner {
    display: flex;
    gap: 10px;
    align-items: flex-start;
    padding: 12px 14px;
    border: 1px solid rgba(217, 119, 6, 0.25);
    background: var(--color-warning-light);
    color: var(--color-fg-1);
    border-radius: 10px;
    margin-bottom: 16px;
    font-size: 12px;
    line-height: 1.5;
  }
  .risk-banner :global(svg) {
    color: var(--color-warning);
    flex-shrink: 0;
    margin-top: 1px;
  }
  .risk-banner p {
    margin: 4px 0 0;
    color: var(--color-fg-2);
  }
  .catalog-toolbar {
    display: flex;
    gap: 10px;
    align-items: center;
    flex-wrap: wrap;
    margin-bottom: 8px;
  }
  .search-box {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1 1 240px;
    min-width: 200px;
    padding: 8px 10px;
    border: 1px solid var(--color-border);
    border-radius: 9px;
    background: var(--color-bg-card);
    color: var(--color-fg-3);
  }
  .search-box:focus-within {
    border-color: var(--color-primary);
    box-shadow: 0 0 0 3px var(--color-primary-glow);
  }
  .search-box input {
    border: none;
    outline: none;
    background: transparent;
    color: var(--color-fg-0);
    font-size: 12px;
    width: 100%;
  }
  .filter-tabs {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
  }
  .filter-tab {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    border: 1px solid var(--color-border);
    background: var(--color-bg-card);
    color: var(--color-fg-2);
    border-radius: 999px;
    padding: 6px 10px;
    font-size: 11px;
    font-weight: 600;
    cursor: pointer;
  }
  .filter-tab[aria-pressed='true'] {
    background: var(--color-primary-light);
    border-color: rgba(59, 91, 219, 0.3);
    color: var(--color-primary);
  }
  .filter-count {
    font-size: 10px;
    color: var(--color-fg-3);
    background: var(--color-bg-hover);
    border-radius: 999px;
    padding: 0 5px;
  }
  .filter-tab[aria-pressed='true'] .filter-count {
    background: var(--color-bg-card);
    color: var(--color-primary);
  }
  .catalog-help {
    font-size: 11px;
    margin: 0 0 12px;
  }
  .flow-help {
    font-size: 11.5px;
    line-height: 1.45;
    color: var(--color-fg-2);
    margin: 0 0 10px;
  }
  .risk-chip {
    font-size: 10px;
    font-weight: 600;
    padding: 2px 6px;
    border-radius: 999px;
    color: var(--color-warning);
    background: var(--color-warning-light);
    border: 1px solid rgba(217, 119, 6, 0.2);
    cursor: help;
  }
  .account-summary {
    font-size: 11px;
    color: var(--color-fg-3);
    margin: -2px 0 10px;
  }
  .planned-card {
    opacity: 0.78;
    background: var(--color-bg-card);
  }
  .empty-catalog {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    padding: 28px 16px;
    border: 1px dashed var(--color-border-strong);
    border-radius: 12px;
    color: var(--color-fg-3);
    text-align: center;
    margin-bottom: 16px;
    font-size: 13px;
  }
  .empty-catalog p {
    margin: 0;
  }
  .planned-hint {
    font-size: 12px;
    margin: -4px 0 16px;
  }
  .link-btn {
    border: none;
    background: transparent;
    padding: 0;
    color: var(--color-primary);
    font: inherit;
    cursor: pointer;
    text-decoration: underline;
  }
  .flow-head {
    display: flex;
    gap: 12px;
    align-items: center;
    margin-bottom: 10px;
  }
  .flow-head .card-h {
    margin: 0;
  }
  .flow-progress {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    border-radius: 9px;
    background: rgba(59, 130, 246, 0.08);
    color: var(--color-fg-1);
    font-size: 12px;
    margin: 10px 0;
  }
  .flow-progress span {
    flex: 1;
  }
  .btn-ghost-sm {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    border: 1px solid var(--color-border);
    background: transparent;
    color: var(--color-fg-2);
    border-radius: 8px;
    padding: 6px 9px;
    font-size: 11px;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-ghost-sm:hover {
    color: var(--color-error);
    border-color: rgba(220, 38, 38, 0.3);
  }
  .flow-meta {
    margin-top: 10px;
    font-size: 11px;
  }
  .flow-meta summary {
    cursor: pointer;
    color: var(--color-fg-2);
    font-weight: 600;
  }
  .flow-meta p {
    margin: 6px 0 0;
  }
  .device-title {
    font-size: 13px;
    font-weight: 650;
    margin: 0 0 4px;
    color: var(--color-fg-0);
  }
  .device-steps {
    margin: 10px 0;
    padding-left: 18px;
    font-size: 12px;
    color: var(--color-fg-2);
    line-height: 1.6;
  }
  .device-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
  }
  .field-label {
    display: block;
    font-size: 11px;
    font-weight: 600;
    color: var(--color-fg-2);
    margin-top: 6px;
  }
  .import-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 10px;
  }
  .setup-steps {
    margin: 12px 0 0;
    padding-left: 18px;
    font-size: 13px;
    line-height: 1.7;
    color: var(--color-fg-2);
  }
  .setup-steps li {
    margin-bottom: 8px;
  }
  .setup-steps code {
    font-size: 11px;
  }
  .copy-chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-left: 6px;
    border: 1px solid var(--color-border);
    background: var(--color-bg-card);
    color: var(--color-fg-2);
    border-radius: 6px;
    padding: 2px 7px;
    font-size: 10px;
    font-weight: 600;
    cursor: pointer;
    vertical-align: middle;
  }
  .accounts-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    padding: 28px 12px;
    text-align: center;
    color: var(--color-fg-3);
    font-size: 13px;
  }
  .accounts-empty p {
    margin: 0;
  }
  .account-title-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .account-title-row .brand-dot {
    margin-top: 0;
    width: 8px;
    height: 8px;
  }
  .account-primary {
    flex: 1;
    min-width: 200px;
  }
  .account-health-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }
  .account-row .session-provider {
    text-transform: none;
    font-size: 13px;
  }
  .account-hint {
    font-size: 11px;
    margin: 2px 0 0;
  }
  .account-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    font-size: 11px;
    color: var(--color-fg-3);
    margin-top: 3px;
  }
  .account-action-hint {
    font-size: 11px;
  }
  .warn-note {
    margin: 8px 0 0;
    padding: 8px 10px;
    border-radius: 8px;
    background: var(--color-warning-light);
    color: var(--color-warning);
    font-size: 11.5px;
    line-height: 1.5;
  }
  @media (max-width: 720px) {
    .header-actions {
      flex-basis: 100%;
      justify-content: space-between;
    }
    .stats-strip {
      grid-template-columns: repeat(2, 1fr);
    }
    .catalog-toolbar {
      flex-direction: column;
      align-items: stretch;
    }
    .filter-tabs {
      flex-wrap: nowrap;
      overflow-x: auto;
      padding-bottom: 2px;
    }
    .filter-tab {
      flex-shrink: 0;
    }
    .account-actions {
      flex-direction: column;
      align-items: flex-start;
    }
    .session-row {
      align-items: flex-start;
    }
  }
</style>