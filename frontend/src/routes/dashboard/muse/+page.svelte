<script lang="ts">
  import TabNav from '$lib/components/TabNav.svelte';
  const __tabs = [
    { label: 'Accounts', path: '/dashboard/connections' },
    { label: 'Discover Free', path: '/dashboard/discover' },
    { label: 'OAuth Providers', path: '/dashboard/oauth-ide' },
    { label: 'Muse', path: '/dashboard/muse' }
  ];
  import { onMount, tick } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import { showToast } from '$lib/toast';
  import {
    Sparkles,
    RefreshCw,
    Server,
    KeyRound,
    CircleAlert,
    X,
    Check,
    Copy,
    Trash2,
    RotateCw,
    RotateCcw,
    Plus,
    ShieldAlert,
    Monitor
  } from 'lucide-svelte';

  interface MusePool {
    total?: number;
    available?: number;
    invalid?: number;
    inflight?: number;
  }

  interface MuseStatusPayload {
    version?: string;
    driver?: { driver?: string; ok?: boolean };
    capabilities?: Record<string, boolean>;
    accounts?: MusePool;
  }

  interface MuseStatus {
    configured: boolean;
    reachable?: boolean;
    ok?: boolean;
    base_url?: string;
    hint?: string;
    error?: string;
    status?: MuseStatusPayload;
  }

  interface MuseAccount {
    id: string;
    label?: string;
    enabled: boolean;
    status: string;
    cookies?: Record<string, string>;
    cookie_expires?: Record<string, number>;
    created_at?: number;
    last_used_at?: number;
    cooldown_until?: number;
    last_error?: string | null;
    success_count?: number;
    fail_count?: number;
    meta?: Record<string, unknown>;
  }

  interface MuseImportToken {
    token: string;
    expires_at: number;
    expires_in: number;
  }

  let status = $state<MuseStatus | null>(null);
  let accounts = $state<MuseAccount[]>([]);
  let loading = $state(true);
  let error = $state('');
  let lastLoadedAt = $state<number | null>(null);
  let action = $state('');
  let pendingDelete = $state('');
  let importLabel = $state('');
  let cookieInput = $state('');
  let importing = $state(false);
  let copiedSetup = $state('');
  let quotaResult = $state<Record<string, string>>({});
  let importToken = $state('');
  let generatingToken = $state(false);

  type BrowserLoginState = {
    open: boolean;
    id: string;
    status: string;
    error: string;
    width: number;
    height: number;
    frame: boolean;
    starting: boolean;
  };
  let browserLogin = $state<BrowserLoginState>({
    open: false,
    id: '',
    status: '',
    error: '',
    width: 1280,
    height: 900,
    frame: false,
    starting: false
  });
  let browserCanvas: HTMLCanvasElement | undefined = $state();
  let browserAbort: AbortController | null = null;

  const configured = $derived(status?.configured === true);
  const reachable = $derived(status?.configured === true && status?.reachable === true && status?.ok === true);
  const pool = $derived(status?.status?.accounts ?? {});
  const capabilities = $derived(status?.status?.capabilities ?? {});
  const driver = $derived(status?.status?.driver ?? {});
  const sortedAccounts = $derived(
    [...accounts].sort((a, b) => accountRank(a) - accountRank(b) || (a.label || a.id).localeCompare(b.label || b.id))
  );

  type Tone = 'ok' | 'warn' | 'danger' | 'muted';

  function accountRank(a: MuseAccount): number {
    const s = accountState(a).tone;
    if (a.status === 'invalid') return 0;
    if (s === 'warn') return 1;
    if (!a.enabled) return 3;
    return 2;
  }

  function accountState(a: MuseAccount): { label: string; tone: Tone; hint: string } {
    if (!a.enabled) {
      return { label: 'Disabled', tone: 'muted', hint: 'This account is excluded from scheduling.' };
    }
    if (a.status === 'invalid') {
      return { label: 'Invalid session', tone: 'danger', hint: 'muse.ai rejected the cookies. Import fresh ones.' };
    }
    if (a.status === 'cooling') {
      if (/quota|credit|limit|exhaust/i.test(a.last_error || '')) {
        return { label: 'Quota exhausted', tone: 'warn', hint: 'Cooling down until the upstream quota resets.' };
      }
      return { label: 'Cooling down', tone: 'warn', hint: 'Temporarily paused after an upstream error.' };
    }
    return { label: 'Active', tone: 'ok', hint: 'Available for new requests.' };
  }

  function unixNow(): number {
    return Date.now() / 1000;
  }

  function ago(value?: number): string {
    if (!value || value <= 0) return 'never';
    const diff = unixNow() - value;
    if (diff < 60) return 'just now';
    if (diff < 3600) return `${Math.round(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.round(diff / 3600)}h ago`;
    return `${Math.round(diff / 86400)}d ago`;
  }

  function until(value?: number): string {
    if (!value || value <= 0) return '';
    const diff = value - unixNow();
    if (diff <= 0) return 'now';
    if (diff < 3600) return `in ${Math.round(diff / 60)}m`;
    if (diff < 86400) return `in ${Math.round(diff / 3600)}h`;
    return `in ${Math.round(diff / 86400)}d`;
  }

  function cookieCount(a: MuseAccount): number {
    return Object.keys(a.cookies ?? {}).length;
  }

  function soonestCookieExpiry(a: MuseAccount): string {
    const values = Object.values(a.cookie_expires ?? {}).filter((v) => v > 0);
    if (values.length === 0) return 'no expiry reported';
    const soonest = Math.min(...values);
    return until(soonest) ? `cookie expiry ${until(soonest)}` : 'cookie expired';
  }

  function parseCookieInput(raw: string): unknown {
    const value = raw.trim();
    if (value.startsWith('{') || value.startsWith('[')) {
      try {
        return JSON.parse(value);
      } catch {
        return value;
      }
    }
    return value;
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const st = await api.get<MuseStatus>('/api/muse/status');
      status = st;
      if (st.configured && st.reachable && st.ok) {
        await refreshAccounts();
      } else {
        accounts = [];
      }
    } catch (e: any) {
      error = e?.message || 'Failed to load Muse sidecar status';
    } finally {
      loading = false;
      lastLoadedAt = Date.now();
    }
  }

  async function refreshAccounts() {
    const res = await api.get<{ data: MuseAccount[] }>('/api/muse/accounts');
    accounts = res.data ?? [];
  }

  async function importAccount() {
    if (!cookieInput.trim()) return;
    importing = true;
    error = '';
    try {
      const res = await api.post<any>('/api/muse/accounts', {
        label: importLabel.trim(),
        cookies: parseCookieInput(cookieInput)
      });
      const missing = Array.isArray(res?.missing_cookies) ? res.missing_cookies : [];
      showToast(
        missing.length
          ? `Account saved, but missing cookies: ${missing.join(', ')}`
          : 'Muse account imported',
        missing.length ? 'info' : 'success',
        5000
      );
      importLabel = '';
      cookieInput = '';
      await load();
    } catch (e: any) {
      error = e?.message || 'Import failed';
    } finally {
      importing = false;
    }
  }

  async function toggleAccount(a: MuseAccount) {
    action = 'toggle-' + a.id;
    error = '';
    try {
      await api.patch(`/api/muse/accounts/${encodeURIComponent(a.id)}`, { enabled: !a.enabled });
      showToast(a.enabled ? 'Account disabled' : 'Account enabled', 'success');
      await load();
    } catch (e: any) {
      error = e?.message || 'Update failed';
    } finally {
      action = '';
    }
  }

  async function renewAccount(a: MuseAccount) {
    action = 'renew-' + a.id;
    error = '';
    try {
      const res = await api.post<any>(`/api/muse/accounts/${encodeURIComponent(a.id)}/renew`, {});
      if (res?.ok === false) {
        showToast(`Renew failed: ${res.error || 'unknown error'}`, 'error', 5000);
      } else {
        showToast('Session renewed', 'success');
      }
      await load();
    } catch (e: any) {
      error = e?.message || 'Renew failed';
    } finally {
      action = '';
    }
  }

  async function resetAccount(a: MuseAccount) {
    action = 'reset-' + a.id;
    error = '';
    try {
      await api.post(`/api/muse/accounts/${encodeURIComponent(a.id)}/reset`, {});
      showToast('Account reset', 'success');
      await load();
    } catch (e: any) {
      error = e?.message || 'Reset failed';
    } finally {
      action = '';
    }
  }

  async function deleteAccount(a: MuseAccount) {
    pendingDelete = '';
    action = 'delete-' + a.id;
    error = '';
    try {
      await api.delete(`/api/muse/accounts/${encodeURIComponent(a.id)}`);
      showToast('Account deleted', 'info');
      await load();
    } catch (e: any) {
      error = e?.message || 'Delete failed';
    } finally {
      action = '';
    }
  }

  function formatQuota(q: any): string {
    if (!q || typeof q !== 'object') return String(q ?? 'unknown');
    const parts: string[] = [];
    if (q.plan) parts.push(q.plan);
    if (typeof q.weekly_used_percent === 'number') {
      const left = Math.max(0, 100 - q.weekly_used_percent);
      parts.push(`weekly ${left}% left${q.weekly_limit_reset ? `, resets ${q.weekly_limit_reset}` : ''}`);
    }
    if (q.additional_tokens_left) {
      parts.push(`${q.additional_tokens_left} extra tokens left${q.additional_tokens_expiry ? ` (${q.additional_tokens_expiry})` : ''}`);
    }
    return parts.length ? parts.join(' · ') : JSON.stringify(q);
  }

  async function checkQuota(a: MuseAccount) {
    action = 'quota-' + a.id;
    error = '';
    try {
      const res = await api.get<any>(`/api/muse/accounts/${encodeURIComponent(a.id)}/quota`);
      quotaResult[a.id] = formatQuota(res?.quota ?? res);
      showToast('Quota snapshot updated', 'success');
    } catch (e: any) {
      error = e?.message || 'Quota lookup failed';
    } finally {
      action = '';
    }
  }

  async function copySetup(value: string, key: string) {
    try {
      await navigator.clipboard.writeText(value);
      copiedSetup = key;
      showToast('Copied', 'success', 1500);
      setTimeout(() => {
        if (copiedSetup === key) copiedSetup = '';
      }, 1500);
    } catch {
      showToast('Copy failed', 'error');
    }
  }

  async function generateImportToken() {
    generatingToken = true;
    error = '';
    try {
      const res = await api.post<MuseImportToken>('/api/muse/import-token', {});
      importToken = res.token;
      showToast('Import token generated — valid for 10 minutes, single use', 'success', 5000);
    } catch (e: any) {
      error = e?.message || 'Failed to generate import token';
    } finally {
      generatingToken = false;
    }
  }

  function resetBrowserLogin() {
    browserLogin = {
      open: false,
      id: '',
      status: '',
      error: '',
      width: 1280,
      height: 900,
      frame: false,
      starting: false
    };
  }

  function closeBrowserLogin() {
    const id = browserLogin.id;
    browserAbort?.abort();
    browserAbort = null;
    resetBrowserLogin();
    if (id) {
      api.delete(`/api/muse/browser/sessions/${encodeURIComponent(id)}`).catch(() => {});
    }
  }

  function finishBrowserLogin() {
    closeBrowserLogin();
    void load();
  }

  async function startBrowserLogin() {
    resetBrowserLogin();
    browserLogin = {
      open: true,
      id: '',
      status: 'Starting an isolated browser…',
      error: '',
      width: 1280,
      height: 900,
      frame: false,
      starting: true
    };
    try {
      const res = await api.post<any>('/api/muse/browser/sessions', {
        label: importLabel.trim() || 'muse-browser'
      });
      browserLogin.id = res.session_id;
      browserLogin.width = Number(res.width) || 1280;
      browserLogin.height = Number(res.height) || 900;
      browserLogin.starting = false;
      browserLogin.status = 'Loading muse.ai…';
      await tick();
      browserCanvas?.focus();
      await openBrowserStream(res.stream_url);
    } catch (e: any) {
      browserLogin.starting = false;
      browserLogin.status = '';
      browserLogin.error = e?.message || 'Failed to start the login browser';
    }
  }

  async function openBrowserStream(url: string) {
    const token = localStorage.getItem('lintasan_token') || '';
    browserAbort = new AbortController();
    try {
      const res = await fetch(url, {
        headers: { Authorization: 'Bearer ' + token, Accept: 'text/event-stream' },
        signal: browserAbort.signal
      });
      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error || `stream failed (HTTP ${res.status})`);
      }
      if (!res.body) throw new Error('stream body is unavailable');
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let split = buffer.indexOf('\n\n');
        while (split >= 0) {
          const chunk = buffer.slice(0, split);
          buffer = buffer.slice(split + 2);
          for (const line of chunk.split('\n')) {
            if (!line.startsWith('data: ')) continue;
            try {
              handleBrowserEvent(JSON.parse(line.slice(6)));
            } catch {
              // Ignore malformed keep-alive/metadata lines.
            }
          }
          split = buffer.indexOf('\n\n');
        }
      }
      if (browserLogin.open && !browserLogin.error) {
        browserLogin.status = 'Browser stream ended.';
      }
    } catch (e: any) {
      if (e?.name === 'AbortError') return;
      browserLogin.error = e?.message || 'Browser stream failed';
    }
  }

  function handleBrowserEvent(ev: any) {
    if (!browserLogin.open) return;
    switch (ev?.type) {
      case 'connected':
        browserLogin.status = 'Connected. Waiting for muse.ai…';
        break;
      case 'frame':
        drawBrowserFrame(ev);
        break;
      case 'status':
        browserLogin.status = ev.message || browserLogin.status;
        break;
      case 'imported': {
        const id = ev?.account?.id ? `: ${ev.account.id}` : '';
        showToast(`Muse account imported${id}`, 'success', 5000);
        finishBrowserLogin();
        break;
      }
      case 'error':
        browserLogin.error = ev.error || 'Remote login failed';
        browserLogin.status = '';
        break;
      case 'closed':
        browserLogin.status = ev.message || 'Session closed.';
        break;
    }
  }

  function drawBrowserFrame(ev: any) {
    const canvas = browserCanvas;
    if (!canvas || !ev?.data) return;
    const width = Number(ev.w) || browserLogin.width;
    const height = Number(ev.h) || browserLogin.height;
    if (canvas.width !== width) canvas.width = width;
    if (canvas.height !== height) canvas.height = height;
    const image = new Image();
    image.onload = () => {
      const ctx = canvas.getContext('2d');
      if (ctx) ctx.drawImage(image, 0, 0);
      if (!browserLogin.frame) browserLogin.frame = true;
    };
    image.src = 'data:image/jpeg;base64,' + ev.data;
  }

  function browserPoint(event: MouseEvent): { x: number; y: number } {
    const canvas = browserCanvas;
    if (!canvas) return { x: 0, y: 0 };
    const rect = canvas.getBoundingClientRect();
    const x = ((event.clientX - rect.left) / rect.width) * canvas.width;
    const y = ((event.clientY - rect.top) / rect.height) * canvas.height;
    return {
      x: Math.max(0, Math.min(canvas.width - 1, x)),
      y: Math.max(0, Math.min(canvas.height - 1, y))
    };
  }

  async function sendBrowserInput(payload: Record<string, unknown>) {
    if (!browserLogin.id) return;
    try {
      await api.post(`/api/muse/browser/sessions/${encodeURIComponent(browserLogin.id)}/input`, payload);
    } catch (e: any) {
      if (!browserLogin.error) {
        browserLogin.error = e?.message || 'Browser input failed';
      }
    }
  }

  function onBrowserMouseDown(event: MouseEvent) {
    if (event.button !== 0) return;
    event.preventDefault();
    browserCanvas?.focus();
    const point = browserPoint(event);
    void sendBrowserInput({ type: 'mouse', event: 'pressed', ...point, button: 'left', clickCount: 1 });
  }

  function onBrowserMouseUp(event: MouseEvent) {
    if (event.button !== 0) return;
    event.preventDefault();
    const point = browserPoint(event);
    void sendBrowserInput({ type: 'mouse', event: 'released', ...point, button: 'left', clickCount: 1 });
  }

  const browserSpecialKeys = new Set([
    'Enter',
    'Tab',
    'Backspace',
    'Escape',
    'Delete',
    'ArrowLeft',
    'ArrowUp',
    'ArrowRight',
    'ArrowDown',
    'Home',
    'End'
  ]);

  function onBrowserKeyDown(event: KeyboardEvent) {
    if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault();
      void sendBrowserInput({ type: 'text', text: event.key });
      return;
    }
    if (browserSpecialKeys.has(event.key)) {
      event.preventDefault();
      void sendBrowserInput({ type: 'key', key: event.key });
    }
  }

  function onBrowserPaste(event: ClipboardEvent) {
    const text = event.clipboardData?.getData('text') || '';
    if (!text) return;
    event.preventDefault();
    void sendBrowserInput({ type: 'text', text });
  }

  onMount(load);
</script>

<svelte:head><title>Muse Accounts — Lintasan</title></svelte:head>

<TabNav tabs={__tabs} />

<div class="muse-page">
  <section class="section-header" aria-labelledby="muse-title">
    <div class="header-icon"><Sparkles size={22} /></div>
    <div class="header-text">
      <h1 id="muse-title">
        Muse Accounts
        <span class="side-pill"><Server size={11} /> Sidecar</span>
      </h1>
      <p class="header-desc">
        muse.ai accounts used by the muse2api sidecar. Lintasan relays admin calls; cookies and the admin key stay in
        muse2api and never reach the browser.
      </p>
    </div>
    <div class="header-actions">
      {#if lastLoadedAt}
        <span class="updated-at">Updated {new Date(lastLoadedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
      {/if}
      <button class="btn-icon" onclick={load} disabled={loading} title="Refresh" aria-label="Refresh Muse accounts">
        <RefreshCw size={18} class={loading ? 'spin' : ''} />
      </button>
    </div>
  </section>

  {#if error}
    <div class="error-banner" role="alert" aria-live="assertive">
      <CircleAlert size={16} />
      <span>{error}</span>
      <button type="button" class="error-close" aria-label="Dismiss error" onclick={() => (error = '')}><X size={14} /></button>
    </div>
  {/if}

  {#if loading}
    <div class="loading-state"><Spinner /><p>Loading Muse sidecar…</p></div>
  {:else if !configured}
    <section class="card setup-card">
      <div class="setup-head">
        <ShieldAlert size={18} />
        <h3>Connect the muse2api sidecar</h3>
      </div>
      <p class="muted small">
        Lintasan does not embed a browser. Muse sessions are driven by a separate
        <a href="https://github.com/www222fff/muse2api" target="_blank" rel="noopener noreferrer">muse2api</a>
        process. Set the admin key on the Lintasan server (the URL is optional), then restart it:
      </p>
      <div class="setup-code">
        <code>LINTASAN_MUSE2API_ADMIN_KEY=&lt;muse2api admin key&gt;</code>
        <button type="button" class="copy-chip" onclick={() => copySetup('LINTASAN_MUSE2API_ADMIN_KEY=', 'key')}>
          {#if copiedSetup === 'key'}<Check size={12} />{:else}<Copy size={12} />{/if} Copy
        </button>
      </div>
      <div class="setup-code">
        <code>LINTASAN_MUSE2API_URL={status?.base_url || 'http://127.0.0.1:18610'}</code>
        <button
          type="button"
          class="copy-chip"
          onclick={() => copySetup(`LINTASAN_MUSE2API_URL=${status?.base_url || 'http://127.0.0.1:18610'}`, 'url')}
        >
          {#if copiedSetup === 'url'}<Check size={12} />{:else}<Copy size={12} />{/if} Copy
        </button>
      </div>
      <p class="muted small">
        The URL is optional and defaults to <code>http://127.0.0.1:18610</code>. The admin key is read only on the
        server side and is never sent to the dashboard.
      </p>
    </section>
  {:else if !reachable}
    <section class="card unreachable-card">
      <div class="setup-head"><CircleAlert size={18} /><h3>muse2api is configured but not reachable</h3></div>
      <p class="muted small">{status?.error || 'The sidecar did not answer the status check.'}</p>
      <p class="muted small">Base URL: <code>{status?.base_url}</code>. Start muse2api, then retry.</p>
      <button type="button" class="btn-secondary-sm" onclick={load}><RefreshCw size={14} /> Retry</button>
    </section>
  {:else}
    <div class="stats-strip">
      {#each [
        { label: 'Accounts', value: pool.total ?? 0, tone: 'neutral', hint: 'Accounts stored in the muse2api pool.' },
        { label: 'Available', value: pool.available ?? 0, tone: 'ok', hint: 'Accounts currently schedulable.' },
        { label: 'Invalid', value: pool.invalid ?? 0, tone: (pool.invalid ?? 0) > 0 ? 'warn' : 'neutral', hint: 'Accounts whose cookies muse.ai rejected.' },
        { label: 'In flight', value: pool.inflight ?? 0, tone: 'ready', hint: 'Requests currently using an account.' }
      ] as stat}
        <div class="stat-cell" title={stat.hint}>
          <div class="stat-value {stat.tone}">{stat.value}</div>
          <div class="stat-label">{stat.label}</div>
        </div>
      {/each}
    </div>

    <div class="sidecar-line">
      <span class="sidecar-dot ok"></span>
      <span>muse2api {status?.status?.version ? 'v' + status.status.version : ''} · driver {driver.driver || 'unknown'}</span>
      <code class="sidecar-url">{status?.base_url}</code>
    </div>

    <section class="card import-card">
      <div class="card-head-row">
        <div>
          <h3 class="card-h">Import an account</h3>
          <p class="muted small">Paste the cookies from a browser logged in to muse.ai.</p>
        </div>
        <KeyRound size={18} class="muted" />
      </div>

      <div class="browser-login-row">
        <div>
          <div class="browser-login-title"><Monitor size={15} /> Login with browser (automatic)</div>
          <p class="muted small">
            A temporary browser opens here. Log in to muse.ai; the four cookies are captured and imported
            automatically — no extension, no token, no copy/paste. Uses the label below if set.
          </p>
        </div>
        <button type="button" class="btn-primary-sm" onclick={startBrowserLogin}>
          <Monitor size={14} /> Open remote login
        </button>
      </div>

      <p class="divider-label muted small">or paste cookies manually</p>

      <form onsubmit={(e) => { e.preventDefault(); importAccount(); }}>
        <label class="field-label" for="muse-label">Label (optional)</label>
        <input id="muse-label" class="input-field" bind:value={importLabel} placeholder="e.g. muse-main" autocomplete="off" />
        <label class="field-label" for="muse-cookies">Cookies</label>
        <textarea
          id="muse-cookies"
          class="input-field cookie-input"
          bind:value={cookieInput}
          rows="4"
          spellcheck="false"
          autocomplete="off"
          placeholder="hatch_sess=...; hatch_gw=...; hatch_vml=...; hatch_native_auth_device=..."
          aria-describedby="muse-cookie-help"
        ></textarea>
        <p class="form-help muted" id="muse-cookie-help">
          Accepts a raw <code>Cookie</code> header, a JSON object, or an EditThisCookie JSON list. Lintasan forwards
          them to muse2api and does not store them.
        </p>
        <div class="form-actions">
          <button type="submit" class="btn-primary-sm" disabled={!cookieInput.trim() || importing}>
            <Plus size={14} /> {importing ? 'Importing…' : 'Import account'}
          </button>
        </div>
      </form>

      <div class="extension-box">
        <div class="extension-head">
          <Sparkles size={16} />
          <strong>Automatic import with the browser extension</strong>
        </div>
        <p class="muted small">
          Load <code>tools/muse-cookie-extension</code> in Chrome (Developer mode → Load unpacked), log in to
          muse.ai, then paste a one-time token there. The extension reads the four <code>hatch_*</code> cookies and
          posts them here; it never sees the muse2api admin key.
        </p>
        {#if importToken}
          <div class="token-row">
            <code class="token-value">{importToken}</code>
            <button type="button" class="copy-chip" onclick={() => copySetup(importToken, 'import-token')}>
              {#if copiedSetup === 'import-token'}<Check size={12} />{:else}<Copy size={12} />{/if} Copy
            </button>
          </div>
          <div class="token-actions">
            <span class="muted small">Valid ~10 minutes · single use · cannot access other endpoints</span>
            <button type="button" class="btn-secondary-sm" onclick={generateImportToken} disabled={generatingToken}>
              {generatingToken ? 'Generating…' : 'New token'}
            </button>
          </div>
        {:else}
          <div class="form-actions">
            <button type="button" class="btn-secondary-sm" onclick={generateImportToken} disabled={generatingToken}>
              {generatingToken ? 'Generating…' : 'Generate import token'}
            </button>
          </div>
        {/if}
      </div>
    </section>

    <section class="card accounts-card">
      <div class="card-head-row">
        <div>
          <h3 class="card-h">Accounts</h3>
          <p class="muted small">Problems are listed first. Cookies are masked by muse2api.</p>
        </div>
        <span class="account-total muted small">{accounts.length} total</span>
      </div>

      {#if accounts.length === 0}
        <div class="accounts-empty">
          <Sparkles size={22} />
          <p><strong>No Muse accounts yet.</strong></p>
          <p class="muted small">Import cookies above to add one.</p>
        </div>
      {:else}
        <div class="account-list">
          {#each sortedAccounts as a (a.id)}
            {@const state = accountState(a)}
            {@const cooling = (a.cooldown_until ?? 0) > unixNow()}
            <article class="muse-account">
              <div class="account-head">
                <div class="account-identity">
                  <div class="account-name">{a.label || a.id}</div>
                  <code class="account-id">{a.id}</code>
                </div>
                <span class="state-pill {state.tone}">{state.label}</span>
                <label class="switch" title={a.enabled ? 'Click to disable' : 'Click to enable'}>
                  <input
                    type="checkbox"
                    checked={a.enabled}
                    disabled={action === 'toggle-' + a.id}
                    onchange={() => toggleAccount(a)}
                    aria-label={`${a.enabled ? 'Disable' : 'Enable'} ${a.label || a.id}`}
                  />
                  <span>{a.enabled ? 'Enabled' : 'Disabled'}</span>
                </label>
              </div>
              <p class="account-hint muted">{state.hint}</p>
              {#if a.last_error}<p class="account-error">{a.last_error}</p>{/if}
              {#if cooling}<p class="account-cooling">Cooling until {until(a.cooldown_until)}.</p>{/if}

              <div class="account-metrics">
                <div><span>Success</span><strong>{a.success_count ?? 0}</strong></div>
                <div><span>Failures</span><strong>{a.fail_count ?? 0}</strong></div>
                <div><span>Last used</span><strong>{ago(a.last_used_at)}</strong></div>
                <div><span>Cookies</span><strong>{cookieCount(a)} · {soonestCookieExpiry(a)}</strong></div>
              </div>

              {#if quotaResult[a.id]}
                <p class="quota-result"><strong>Quota:</strong> <code>{quotaResult[a.id]}</code></p>
              {/if}
              {#if !capabilities.quota}
                <p class="quota-note muted small">
                  muse2api does not report quota or credit limits yet. The success, failure, and last-used counters above
                  come from the sidecar; see <a href="/dashboard/usage">gateway usage</a> for request and token totals.
                </p>
              {/if}

              <div class="account-actions">
                {#if capabilities.renew_session}
                  <button
                    type="button"
                    class="btn-secondary-sm"
                    disabled={action === 'renew-' + a.id}
                    onclick={() => renewAccount(a)}
                  >
                    <RotateCw size={13} /> {action === 'renew-' + a.id ? 'Renewing…' : 'Renew session'}
                  </button>
                {/if}
                {#if capabilities.quota}
                  <button
                    type="button"
                    class="btn-secondary-sm"
                    disabled={action === 'quota-' + a.id}
                    onclick={() => checkQuota(a)}
                  >
                    {action === 'quota-' + a.id ? 'Checking…' : 'Quota'}
                  </button>
                {/if}
                <button
                  type="button"
                  class="btn-secondary-sm"
                  disabled={action === 'reset-' + a.id}
                  onclick={() => resetAccount(a)}
                >
                  <RotateCcw size={13} /> {action === 'reset-' + a.id ? 'Resetting…' : 'Reset'}
                </button>
                {#if pendingDelete === a.id}
                  <button type="button" class="btn-ghost-danger" disabled={action === 'delete-' + a.id} onclick={() => deleteAccount(a)}>
                    <Trash2 size={13} /> {action === 'delete-' + a.id ? 'Deleting…' : 'Confirm delete'}
                  </button>
                  <button type="button" class="btn-secondary-sm" onclick={() => (pendingDelete = '')}>Keep</button>
                {:else}
                  <button type="button" class="btn-ghost-danger" onclick={() => (pendingDelete = a.id)}>
                    <Trash2 size={13} /> Delete
                  </button>
                {/if}
              </div>

              {#if cookieCount(a) > 0}
                <details class="cookie-details">
                  <summary>Session cookies ({cookieCount(a)})</summary>
                  <ul>
                    {#each Object.entries(a.cookies ?? {}) as [name, masked]}
                      <li><code>{name}</code><span>{masked}</span></li>
                    {/each}
                  </ul>
                </details>
              {/if}
            </article>
          {/each}
        </div>
      {/if}
    </section>
  {/if}

  {#if browserLogin.open}
    <div class="browser-modal" role="dialog" aria-modal="true" aria-labelledby="muse-browser-title">
      <div class="browser-shell">
        <div class="browser-head">
          <div>
            <div id="muse-browser-title" class="browser-title">Login muse.ai</div>
            <div class="muted small">Temporary browser on the server · cookies imported automatically</div>
          </div>
          <button type="button" class="btn-ghost-sm" onclick={closeBrowserLogin}>Close</button>
        </div>
        {#if browserLogin.error}
          <div class="error-banner" role="alert">
            <CircleAlert size={16} /><span>{browserLogin.error}</span>
          </div>
        {/if}
        <div class="browser-stage">
          <canvas
            bind:this={browserCanvas}
            width={browserLogin.width}
            height={browserLogin.height}
            tabindex="0"
            aria-label="Remote muse.ai login browser"
            onmousedown={onBrowserMouseDown}
            onmouseup={onBrowserMouseUp}
            onkeydown={onBrowserKeyDown}
            onpaste={onBrowserPaste}
          ></canvas>
          {#if !browserLogin.frame}
            <div class="browser-wait">
              <Spinner />
              <span>{browserLogin.starting ? 'Starting browser…' : 'Loading muse.ai…'}</span>
            </div>
          {/if}
        </div>
        <div class="browser-status" role="status" aria-live="polite">{browserLogin.status}</div>
        <p class="muted small">
          Click inside the browser area, then type. Login (email/phone + code) goes directly to muse.ai; Lintasan never
          receives your password or OTP.
        </p>
      </div>
    </div>
  {/if}
</div>

<style>
  .muse-page {
    animation: fadeInUp 0.35s ease-out;
    max-width: 1080px;
    margin: 0 auto;
  }
  .section-header {
    display: flex;
    align-items: flex-start;
    gap: 14px;
    flex-wrap: wrap;
    margin-bottom: 20px;
  }
  .header-icon {
    width: 44px;
    height: 44px;
    border-radius: 12px;
    background: rgba(139, 92, 246, 0.12);
    color: #7c3aed;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .header-text {
    flex: 1;
    min-width: 0;
  }
  .header-text h1 {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    margin: 0;
    color: var(--color-fg-0);
    font-size: 20px;
    font-weight: 700;
    letter-spacing: -0.3px;
  }
  .header-desc {
    margin: 4px 0 0;
    color: var(--color-fg-3);
    font-size: 13px;
  }
  .side-pill {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 10px;
    font-weight: 650;
    padding: 3px 8px;
    border-radius: 999px;
    border: 1px solid var(--color-border);
    background: var(--color-bg-hover);
    color: var(--color-fg-2);
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
  .btn-icon {
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
  .card-h {
    margin: 0 0 6px;
    font-size: 15px;
    font-weight: 600;
  }
  .card-head-row {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
    margin-bottom: 10px;
  }
  .account-total {
    white-space: nowrap;
    flex-shrink: 0;
  }
  .setup-card {
    border-color: rgba(217, 119, 6, 0.3);
    background: linear-gradient(120deg, var(--color-warning-light), var(--color-bg-card) 65%);
  }
  .unreachable-card {
    border-color: rgba(239, 68, 68, 0.25);
  }
  .setup-head {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--color-fg-0);
    margin-bottom: 8px;
  }
  .setup-head :global(svg) {
    color: var(--color-warning);
    flex-shrink: 0;
  }
  .setup-head h3 {
    margin: 0;
    font-size: 15px;
    font-weight: 650;
  }
  .setup-code {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin: 8px 0;
    padding: 8px 10px;
    border: 1px solid var(--color-border);
    border-radius: 8px;
    background: var(--color-bg-card);
    font-size: 11.5px;
    overflow-wrap: anywhere;
  }
  .copy-chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    flex-shrink: 0;
    border: 1px solid var(--color-border);
    background: var(--color-bg-card);
    color: var(--color-fg-2);
    border-radius: 6px;
    padding: 3px 7px;
    font-size: 10px;
    font-weight: 600;
    cursor: pointer;
  }
  .stats-strip {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 1px;
    background: var(--color-border);
    border-radius: 12px;
    overflow: hidden;
    margin-bottom: 12px;
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
  .stat-value.neutral {
    color: var(--color-fg-0);
  }
  .stat-value.ok {
    color: var(--color-success);
  }
  .stat-value.warn {
    color: var(--color-warning);
  }
  .stat-value.ready {
    color: var(--color-primary);
  }
  .stat-label {
    font-size: 10px;
    font-weight: 600;
    color: var(--color-fg-3);
    letter-spacing: 0.5px;
    margin-top: 2px;
    text-transform: uppercase;
  }
  .sidecar-line {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    margin: 0 0 16px;
    font-size: 11.5px;
    color: var(--color-fg-3);
  }
  .sidecar-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--color-fg-3);
  }
  .sidecar-dot.ok {
    background: var(--color-success);
    box-shadow: 0 0 0 3px rgba(5, 150, 105, 0.12);
  }
  .sidecar-url {
    margin-left: auto;
    font-size: 10.5px;
  }
  .field-label {
    display: block;
    font-size: 11px;
    font-weight: 600;
    color: var(--color-fg-2);
    margin: 8px 0 4px;
  }
  .cookie-input {
    width: 100%;
    font-family: var(--font-mono);
    font-size: 11.5px;
    resize: vertical;
    line-height: 1.5;
  }
  .form-help {
    margin: 6px 0 0;
    font-size: 11px;
  }
  .form-actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 10px;
  }
  .extension-box {
    margin-top: 14px;
    padding-top: 14px;
    border-top: 1px dashed var(--color-border);
  }
  .extension-head {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: var(--color-fg-0);
    margin-bottom: 6px;
  }
  .extension-head :global(svg) {
    color: var(--color-primary);
  }
  .token-row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 10px 0 8px;
    padding: 8px 10px;
    border: 1px solid var(--color-border);
    border-radius: 8px;
    background: var(--color-bg-body);
  }
  .token-value {
    flex: 1;
    min-width: 0;
    font-size: 11px;
    overflow-wrap: anywhere;
  }
  .token-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    flex-wrap: wrap;
  }
  .browser-login-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
    padding: 12px;
    margin-bottom: 8px;
    border: 1px solid rgba(59, 91, 219, 0.22);
    background: var(--color-primary-light);
    border-radius: 10px;
  }
  .browser-login-title {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12.5px;
    font-weight: 650;
    color: var(--color-fg-0);
  }
  .browser-login-title :global(svg) {
    color: var(--color-primary);
  }
  .divider-label {
    text-align: center;
    margin: 8px 0 2px;
  }
  .browser-modal {
    position: fixed;
    inset: 0;
    z-index: 2000;
    background: rgba(15, 23, 42, 0.68);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 18px;
  }
  .browser-shell {
    width: min(1100px, 100%);
    max-height: 94vh;
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 14px;
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: 14px;
    box-shadow: 0 20px 50px rgba(15, 23, 42, 0.28);
  }
  .browser-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 10px;
  }
  .browser-title {
    font-size: 15px;
    font-weight: 650;
    color: var(--color-fg-0);
  }
  .browser-stage {
    position: relative;
    flex: 1;
    min-height: 280px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: #0b1220;
    border-radius: 10px;
    overflow: auto;
  }
  .browser-stage canvas {
    display: block;
    max-width: 100%;
    max-height: 72vh;
    outline: none;
    cursor: default;
  }
  .browser-wait {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    background: rgba(11, 18, 32, 0.72);
    color: #cbd5e1;
    font-size: 12px;
  }
  .browser-status {
    font-size: 11.5px;
    color: var(--color-fg-2);
    min-height: 16px;
  }
  .btn-primary-sm,
  .btn-secondary-sm,
  .btn-ghost-danger {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 7px 11px;
    border-radius: 9px;
    font-size: 11.5px;
    font-weight: 600;
    cursor: pointer;
  }
  .btn-primary-sm {
    border: none;
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
    cursor: not-allowed;
  }
  .btn-ghost-danger {
    background: transparent;
    color: var(--color-error);
    border: 1px solid rgba(239, 68, 68, 0.25);
  }
  .btn-ghost-danger:disabled {
    opacity: 0.5;
    cursor: not-allowed;
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
  .account-list {
    display: grid;
    gap: 12px;
  }
  .muse-account {
    border: 1px solid var(--color-border);
    border-radius: 12px;
    padding: 14px;
    background: var(--color-bg-card);
  }
  .account-head {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }
  .account-identity {
    flex: 1;
    min-width: 160px;
  }
  .account-name {
    font-size: 14px;
    font-weight: 650;
    color: var(--color-fg-0);
  }
  .account-id {
    font-size: 10px;
    color: var(--color-fg-3);
  }
  .state-pill {
    font-size: 10px;
    font-weight: 700;
    padding: 3px 8px;
    border-radius: 999px;
  }
  .state-pill.ok {
    background: var(--color-success-light);
    color: var(--color-success);
  }
  .state-pill.warn {
    background: var(--color-warning-light);
    color: var(--color-warning);
  }
  .state-pill.danger {
    background: var(--color-error-light);
    color: var(--color-error);
  }
  .state-pill.muted {
    background: var(--color-bg-hover);
    color: var(--color-fg-3);
  }
  .switch {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
    color: var(--color-fg-2);
    cursor: pointer;
  }
  .switch input {
    accent-color: var(--color-primary);
  }
  .account-hint {
    margin: 6px 0 0;
    font-size: 11.5px;
  }
  .account-error {
    margin: 8px 0 0;
    padding: 6px 8px;
    border-radius: 6px;
    background: var(--color-error-light);
    color: var(--color-error);
    font-size: 11px;
    overflow-wrap: anywhere;
  }
  .account-cooling {
    margin: 6px 0 0;
    font-size: 11px;
    color: var(--color-warning);
  }
  .account-metrics {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 8px;
    margin: 10px 0;
  }
  .account-metrics div {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }
  .account-metrics span {
    font-size: 10px;
    color: var(--color-fg-3);
    text-transform: uppercase;
    letter-spacing: 0.4px;
  }
  .account-metrics strong {
    font-size: 12px;
    color: var(--color-fg-1);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .quota-note {
    margin: 8px 0 0;
    font-size: 11px;
    line-height: 1.45;
  }
  .quota-result {
    margin: 0 0 8px;
    font-size: 11px;
    overflow-wrap: anywhere;
  }
  .account-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 8px;
  }
  .cookie-details {
    margin-top: 10px;
    font-size: 11px;
    color: var(--color-fg-2);
  }
  .cookie-details summary {
    cursor: pointer;
    font-weight: 600;
  }
  .cookie-details ul {
    list-style: none;
    margin: 8px 0 0;
    padding: 0;
    display: grid;
    gap: 4px;
  }
  .cookie-details li {
    display: flex;
    gap: 8px;
    justify-content: space-between;
    border-bottom: 1px dashed var(--color-border);
    padding-bottom: 3px;
  }
  .muted {
    color: var(--color-fg-3);
  }
  .small {
    font-size: 11.5px;
  }
  @media (max-width: 720px) {
    .header-actions {
      flex-basis: 100%;
      justify-content: space-between;
    }
    .stats-strip {
      grid-template-columns: repeat(2, 1fr);
    }
    .account-metrics {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .sidecar-url {
      margin-left: 0;
      flex-basis: 100%;
    }
    .browser-modal {
      padding: 0;
    }
    .browser-shell {
      height: 100vh;
      max-height: 100vh;
      border-radius: 0;
    }
    .browser-stage canvas {
      max-height: 64vh;
    }
  }
</style>
