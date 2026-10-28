<script lang="ts">
  import TabNav from '$lib/components/TabNav.svelte';
  const __tabs = [
    { label: 'Accounts', path: '/dashboard/connections' },
    { label: 'Discover Free', path: '/dashboard/discover' },
    { label: 'OAuth Providers', path: '/dashboard/oauth-ide' },
    { label: 'Muse', path: '/dashboard/muse' }
  ];
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { showToast } from '$lib/toast';
  import { Search, Globe, Zap, Clock, Shield, Check, Plus, AlertCircle, X, RefreshCw } from 'lucide-svelte/icons';

  interface FreeProvider {
    name: string;
    prefix: string;
    base_url: string;
    models: string[];
    auth_type: string;
    quota_info: string;
    enabled: boolean;
    chat_path?: string;
    models_path?: string;
    free_only?: boolean;
    seed_only?: boolean;
    no_model_fetch?: boolean;
    logo?: string;
    sync_source?: string;
    sync_updated_at?: string;
  }

  interface Connection {
    id: string;
    name: string;
    base_url: string;
    is_active: number;
  }

  let providers = $state<FreeProvider[]>([]);
  let loading = $state(true);
  let error = $state('');
  let connecting = $state<string | null>(null);
  let connected = $state<Set<string>>(new Set());
  let apiKeyModal = $state<FreeProvider | null>(null);
  let apiKeyInput = $state('');
  let keyInputEl = $state<HTMLInputElement | null>(null);
  let connectionIds = $state<Record<string, string>>({});
  let fetchedModels = $state<Record<string, string[]>>({});
  let modelSources = $state<Record<string, string>>({});
  let fetchingModels = $state<Record<string, boolean>>({});

  async function loadProviders() {
    loading = true;
    error = '';
    try {
      const [res, conns] = await Promise.all([
        api.get<any>('/api/discover/free-providers'),
        api.get<any>('/api/connections').catch(() => ({ data: [] }))
      ]);
      providers = (res.providers || [])
        .filter((p: FreeProvider) => p.enabled !== false)
        .map((p: FreeProvider) => ({ ...p, models: Array.isArray(p.models) ? p.models : [] }));
      const existing = new Set<string>();
      const ids: Record<string, string> = {};
      for (const conn of conns?.data ?? []) {
        if (conn.is_active === 0) continue;
        for (const p of providers) {
          if (conn.name === p.name && trimURL(conn.base_url) === trimURL(p.base_url)) {
            existing.add(p.name);
            ids[p.name] = conn.id;
          }
        }
      }
      connected = existing;
      connectionIds = ids;
    } catch (e: any) {
      error = e.message || 'Failed to load providers';
    }
    loading = false;
  }

  function trimURL(url: string | undefined): string {
    return (url || '').trim().replace(/\/$/, '').toLowerCase();
  }

  onMount(loadProviders);

  function handleConnectClick(provider: FreeProvider) {
    if (provider.auth_type === 'none') {
      connectProvider(provider, '');
    } else {
      apiKeyModal = provider;
      apiKeyInput = '';
      setTimeout(() => keyInputEl?.focus(), 30);
    }
  }

  function modelsFor(provider: FreeProvider): string[] {
    return fetchedModels[provider.name] ?? provider.models;
  }

  async function loadDiscoveredModels(id: string): Promise<string[]> {
    const res = await api.get<any>(`/api/models/discovered?connection_id=${encodeURIComponent(id)}`);
    return (res?.data ?? [])
      .filter((model: any) => model.is_active !== 0)
      .map((model: any) => model.model_id)
      .filter(Boolean);
  }

  async function fetchModels(provider: FreeProvider) {
    if (provider.no_model_fetch || fetchingModels[provider.name]) return;
    fetchingModels[provider.name] = true;
    try {
      const connId = connectionIds[provider.name];
      if (connId && !provider.free_only) {
        await api.post(`/api/models/sync/${encodeURIComponent(connId)}`, {});
        const ids = await loadDiscoveredModels(connId);
        if (!ids.length) throw new Error('provider returned no models');
        fetchedModels[provider.name] = ids;
        modelSources[provider.name] = 'live';
        showToast(`${provider.name}: ${ids.length} live models`, 'success');
      } else {
        const prefix = provider.prefix.replace(/\/$/, '');
        const res = await api.get<any>(`/api/discover/free-providers/${encodeURIComponent(prefix)}/models`);
        const ids = res?.models ?? [];
        if (!ids.length) throw new Error('no models returned');
        fetchedModels[provider.name] = ids;
        modelSources[provider.name] = res?.source || 'live';
        showToast(`${provider.name}: ${ids.length}${provider.free_only ? ' free' : ''} models`, 'success');
      }
    } catch (e: any) {
      if (e?.status === 409 || e?.detail?.needs_key) {
        showToast(`Connect ${provider.name} with an API key first`, 'info', 4000);
      } else {
        showToast(e?.message || 'Failed to fetch models', 'error');
      }
    } finally {
      fetchingModels[provider.name] = false;
    }
  }

  async function connectProvider(provider: FreeProvider, key: string) {
    connecting = provider.name;
    try {
      const res = await api.post<any>('/api/connections', {
        name: provider.name,
        base_url: provider.base_url,
        api_key: key,
        models: modelsFor(provider),
        chat_path: provider.chat_path,
        models_path: provider.models_path,
      });
      connected = new Set([...connected, provider.name]);
      if (res?.id) connectionIds[provider.name] = res.id;
      apiKeyModal = null;
      const count = Number(res?.models_count ?? 0);
      const verb = res?.status === 'updated' ? 'Updated' : 'Connected';
      showToast(`${verb} ${provider.name}${count ? ` · ${count} model${count === 1 ? '' : 's'}` : ''}`, 'success');
    } catch (e: any) {
      showToast(e.message || 'Failed to connect provider', 'error');
    } finally { connecting = null; }
  }

  function getAuthBadge(auth: string) {
    switch (auth) {
      case 'none': return { label: 'No Auth', color: 'var(--color-success)' };
      case 'apikey': return { label: 'API Key', color: 'var(--color-warning)' };
      case 'oauth': return { label: 'OAuth', color: 'var(--color-purple)' };
      default: return { label: auth, color: 'var(--color-fg-3)' };
    }
  }
</script>

<TabNav tabs={__tabs} />


<div style="animation: fadeInUp 0.4s ease-out;">
  <!-- Header -->
  <div class="flex items-center gap-3 mb-5">
    <div style="width: 40px; height: 40px; border-radius: 10px; background: linear-gradient(135deg, var(--color-success) 0%, color-mix(in srgb, var(--color-success) 80%, black) 100%); display: flex; align-items: center; justify-content: center;">
      <Globe size={20} color="white" />
    </div>
    <div>
      <h2 style="font-size: 18px; font-weight: 600; color: var(--color-fg-0); margin: 0;">Discover Free Providers</h2>
      <p style="font-size: 12px; color: var(--color-fg-3); margin: 2px 0 0 0;">Connect free-tier AI providers. Some need a free API key; limits and data policies vary.</p>
    </div>
  </div>

  {#if loading}
    <div class="flex justify-center" style="padding: 60px 0;">
      <Spinner />
    </div>
  {:else if error}
    <EmptyState icon={AlertCircle} title="Failed to load providers" description={error} action={loadProviders} actionLabel="Retry" />
  {:else if providers.length === 0}
    <EmptyState icon={Globe} title="No Providers Found" description="Could not fetch free providers list." />
  {:else}
    <!-- Provider grid -->
    <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 16px;">
      {#each providers as provider}
        {@const badge = getAuthBadge(provider.auth_type)}
        {@const isConnected = connected.has(provider.name)}
        {@const models = fetchedModels[provider.name] ?? provider.models}
        <div class="card" style="padding: 20px; position: relative; overflow: hidden;">
          <!-- Provider header -->
          <div class="flex items-start justify-between mb-3">
            <div class="flex items-center gap-3">
              <div style="position: relative; width: 36px; height: 36px; border-radius: 8px; background: var(--color-bg-sidebar-hover); display: flex; align-items: center; justify-content: center; font-size: 10px; font-weight: 700; color: var(--color-fg-3); font-family: var(--font-mono); overflow: hidden;">
                <span>{provider.prefix.replace('/', '')}</span>
                {#if provider.logo}
                  <img
                    src={provider.logo}
                    alt={`${provider.name} logo`}
                    loading="lazy"
                    style="position: absolute; width: 24px; height: 24px; object-fit: contain;"
                    onerror={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none'; }}
                  />
                {/if}
              </div>
              <div>
                <h3 style="font-size: 15px; font-weight: 600; color: var(--color-fg-0); margin: 0;">{provider.name}</h3>
                <span style="font-size: 11px; padding: 2px 6px; border-radius: 4px; background: {badge.color}20; color: {badge.color}; font-weight: 500;">{badge.label}</span>
              </div>
            </div>
            {#if isConnected}
              <span class="flex items-center gap-1" style="font-size: 12px; color: #10b981; font-weight: 500;">
                <Check size={14} /> Connected
              </span>
            {/if}
          </div>

          <!-- Quota info -->
          <div class="flex items-center gap-2 mb-3" style="font-size: 12px; color: var(--color-fg-2);">
            <Zap size={12} style="color: var(--color-primary);" />
            <span>{provider.quota_info}</span>
          </div>

          <!-- Models -->
          <div class="flex items-center justify-between mb-2" style="gap: 8px;">
            <span
              style="font-size: 11px; color: var(--color-fg-3);"
              title={provider.sync_source ? `${provider.sync_source}${provider.sync_updated_at ? ` · ${provider.sync_updated_at}` : ''}` : undefined}
            >
              {models.length}{provider.free_only ? ' free' : ''} model{models.length === 1 ? '' : 's'}{#if modelSources[provider.name] === 'live'}{' · live'}{:else if provider.sync_source}{' · synced'}{/if}
            </span>
            {#if !provider.no_model_fetch}
              <button
                type="button"
                class="btn-secondary"
                style="font-size: 11px; padding: 4px 8px; display: inline-flex; align-items: center; gap: 4px;"
                disabled={fetchingModels[provider.name] === true}
                title="Fetch this provider's current model list"
                onclick={() => fetchModels(provider)}
              >
                <RefreshCw size={12} />
                {fetchingModels[provider.name] ? 'Fetching…' : 'Fetch models'}
              </button>
            {:else}
              <span style="font-size: 10px; color: var(--color-fg-3);">curated</span>
            {/if}
          </div>
          <div class="flex flex-wrap gap-1.5 mb-4">
            {#each models.slice(0, 4) as model}
              <span style="font-size: 11px; padding: 3px 8px; border-radius: 4px; background: var(--color-bg-sidebar-hover); color: var(--color-fg-2); font-family: var(--font-mono);">
                {model}
              </span>
            {/each}
            {#if models.length > 4}
              <span style="font-size: 11px; padding: 3px 8px; border-radius: 4px; background: var(--color-bg-sidebar-hover); color: var(--color-fg-3);">
                +{models.length - 4} more
              </span>
            {/if}
          </div>

          <!-- Connect button -->
          <button
            class="btn-primary w-full flex items-center justify-center gap-2"
            onclick={() => handleConnectClick(provider)}
            disabled={isConnected || connecting === provider.name}
            style="opacity: {isConnected ? 0.5 : 1};"
          >
            {#if connecting === provider.name}
              <Spinner /> Connecting...
            {:else if isConnected}
              <Check size={16} /> Connected
            {:else if provider.auth_type === 'apikey'}
              <Plus size={16} /> Connect with API Key
            {:else}
              <Plus size={16} /> Connect Provider
            {/if}
          </button>
        </div>
      {/each}
    </div>
  {/if}

  {#if apiKeyModal}
    <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
      <button
        type="button"
        class="absolute inset-0 border-0 bg-black/50 p-0"
        data-testid="api-key-backdrop"
        aria-label="Close API key dialog"
        onclick={() => apiKeyModal = null}
      ></button>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="API key required"
        tabindex="-1"
        class="card relative z-10 w-full max-w-md p-6 bg-[var(--color-bg-card)] border border-[var(--color-border)] shadow-xl rounded-xl"
        onkeydown={(e) => { if (e.key === 'Escape') { e.stopPropagation(); apiKeyModal = null; } }}
      >
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-base font-semibold text-[var(--color-fg-0)] m-0">Connect {apiKeyModal.name}</h3>
          <button class="btn-ghost p-1" aria-label="Close dialog" onclick={() => apiKeyModal = null}><X size={16} /></button>
        </div>
        <p class="text-xs text-[var(--color-fg-2)] mb-3">
          This provider requires an API key. Enter your key below:
        </p>
        <form onsubmit={(e) => { e.preventDefault(); if (apiKeyModal) connectProvider(apiKeyModal, apiKeyInput); }}>
          <input
            bind:this={keyInputEl}
            type="password"
            class="input-field w-full mb-4"
            placeholder="sk-…"
            aria-label="Provider API key"
            bind:value={apiKeyInput}
            required
          />
          <div class="flex justify-end gap-2">
            <button type="button" class="btn-secondary" onclick={() => apiKeyModal = null}>Cancel</button>
            <button type="submit" class="btn-primary" disabled={!apiKeyInput.trim()}>
              Save &amp; Connect
            </button>
          </div>
        </form>
      </div>
    </div>
  {/if}
</div>
