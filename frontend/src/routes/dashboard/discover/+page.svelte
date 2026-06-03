<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { Search, Globe, Zap, Clock, Shield, Check, Plus } from 'lucide-svelte/icons';

  interface FreeProvider {
    name: string;
    prefix: string;
    base_url: string;
    models: string[];
    auth_type: string;
    quota_info: string;
    enabled: boolean;
  }

  let providers = $state<FreeProvider[]>([]);
  let loading = $state(true);
  let connecting = $state<string | null>(null);
  let connected = $state<Set<string>>(new Set());

  onMount(async () => {
    try {
      const res = await api.get<any>('/api/discover/free-providers');
      providers = res.providers || [];
    } catch {}
    finally { loading = false; }
  });

  async function connectProvider(provider: FreeProvider) {
    connecting = provider.name;
    try {
      await api.post('/api/connections', {
        name: provider.name,
        provider: provider.prefix.replace('/', ''),
        base_url: provider.base_url,
        api_key: '',
        models: provider.models,
      });
      connected = new Set([...connected, provider.name]);
    } catch (e) {
      console.error('Connect failed:', e);
    } finally { connecting = null; }
  }

  function getAuthBadge(auth: string) {
    switch (auth) {
      case 'none': return { label: 'No Auth', color: '#10b981' };
      case 'apikey': return { label: 'API Key', color: '#f59e0b' };
      case 'oauth': return { label: 'OAuth', color: '#8b5cf6' };
      default: return { label: auth, color: '#6b7280' };
    }
  }
</script>

<div style="animation: fadeInUp 0.4s ease-out;">
  <!-- Header -->
  <div class="flex items-center gap-3 mb-5">
    <div style="width: 40px; height: 40px; border-radius: 10px; background: linear-gradient(135deg, #10b981 0%, #059669 100%); display: flex; align-items: center; justify-content: center;">
      <Globe size={20} color="white" />
    </div>
    <div>
      <h2 style="font-size: 18px; font-weight: 600; color: var(--color-fg-0); margin: 0;">Discover Free Providers</h2>
      <p style="font-size: 12px; color: var(--color-fg-3); margin: 2px 0 0 0;">Connect free AI providers with one click. No credit card needed.</p>
    </div>
  </div>

  {#if loading}
    <div class="flex justify-center" style="padding: 60px 0;">
      <Spinner />
    </div>
  {:else if providers.length === 0}
    <EmptyState icon={Globe} title="No Providers Found" description="Could not fetch free providers list." />
  {:else}
    <!-- Provider grid -->
    <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 16px;">
      {#each providers as provider}
        {@const badge = getAuthBadge(provider.auth_type)}
        {@const isConnected = connected.has(provider.name)}
        <div class="card" style="padding: 20px; position: relative; overflow: hidden;">
          <!-- Provider header -->
          <div class="flex items-start justify-between mb-3">
            <div class="flex items-center gap-3">
              <div style="width: 36px; height: 36px; border-radius: 8px; background: var(--color-bg-hover); display: flex; align-items: center; justify-content: center; font-size: 14px; font-weight: 700; color: var(--color-primary); font-family: var(--font-mono);">
                {provider.prefix.replace('/', '')}
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
          <div class="flex flex-wrap gap-1.5 mb-4">
            {#each provider.models.slice(0, 4) as model}
              <span style="font-size: 11px; padding: 3px 8px; border-radius: 4px; background: var(--color-bg-hover); color: var(--color-fg-2); font-family: var(--font-mono);">
                {model}
              </span>
            {/each}
            {#if provider.models.length > 4}
              <span style="font-size: 11px; padding: 3px 8px; border-radius: 4px; background: var(--color-bg-hover); color: var(--color-fg-3);">
                +{provider.models.length - 4} more
              </span>
            {/if}
          </div>

          <!-- Connect button -->
          <button
            class="btn-primary w-full flex items-center justify-center gap-2"
            onclick={() => connectProvider(provider)}
            disabled={isConnected || connecting === provider.name}
            style="opacity: {isConnected ? 0.5 : 1};"
          >
            {#if connecting === provider.name}
              <Spinner /> Connecting...
            {:else if isConnected}
              <Check size={16} /> Connected
            {:else}
              <Plus size={16} /> Connect Provider
            {/if}
          </button>
        </div>
      {/each}
    </div>
  {/if}
</div>
