<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { Server, ChevronDown, ChevronRight, Search, Brain, Zap, Eye, Wrench, FileJson, Activity, ExternalLink, Cpu } from 'lucide-svelte/icons';

  interface Model {
    id: string; name: string; provider: string;
    context_window: number; max_tokens: number;
    input_price_per_1m: number; output_price_per_1m: number;
    capabilities: string[];
  }

  interface Provider {
    id: string; name: string; base_url: string; format: string;
    model_count: number; models: Model[];
  }

  let providers = $state<Provider[]>([]);
  let loading = $state(true);
  let error = $state('');
  let search = $state('');
  let expanded = $state<Set<string>>(new Set());

  const filtered = $derived(
    search
      ? providers.map(p => ({
          ...p,
          models: p.models.filter(m =>
            m.name.toLowerCase().includes(search.toLowerCase()) ||
            m.id.toLowerCase().includes(search.toLowerCase()) ||
            p.name.toLowerCase().includes(search.toLowerCase())
          )
        })).filter(p => p.models.length > 0)
      : providers
  );

  const meta = $derived({
    totalProviders: providers.length,
    totalModels: providers.reduce((s, p) => s + p.models.length, 0),
    visibleModels: filtered.reduce((s, p) => s + p.models.length, 0),
  });

  onMount(async () => {
    try {
      const res = await api.get<any>('/api/models/catalog');
      const raw = res.data || [];
      // Map snake_case to camelCase for the API response fields
      providers = raw.map((p: any) => ({
        id: p.id,
        name: p.name,
        base_url: p.base_url,
        format: p.format,
        model_count: p.model_count || p.models?.length || 0,
        models: (p.models || []).map((m: any) => ({
          id: m.id,
          name: m.name,
          provider: m.provider || p.name,
          context_window: m.context_window,
          max_tokens: m.max_tokens,
          input_price_per_1m: m.input_price_per_1m || m.input_price,
          output_price_per_1m: m.output_price_per_1m || m.output_price,
          capabilities: m.capabilities || [],
        }))
      }));
    } catch (e: any) { error = e.message; }
    finally { loading = false; }
  });

  function toggle(id: string) {
    const next = new Set(expanded);
    next.has(id) ? next.delete(id) : next.add(id);
    expanded = next;
  }

  function fmtPrice(price: number) {
    if (price === 0) return 'Free';
    if (price < 0.1) return '$' + price.toFixed(3);
    if (price < 1) return '$' + price.toFixed(2);
    return '$' + price.toFixed(2);
  }

  function fmtTokens(n: number) {
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
    if (n >= 1_000) return (n / 1_000).toFixed(0) + 'K';
    return String(n);
  }

  function capColor(cap: string) {
    switch (cap) {
      case 'chat': return '#8b5cf6';
      case 'vision': return '#06b6d4';
      case 'tools': return '#f59e0b';
      case 'streaming': return '#10b981';
      case 'json_mode': return '#ec4899';
      default: return '#6b7280';
    }
  }

  // Provider brand colors
  const providerColors: Record<string, { bg: string; fg: string; border: string }> = {
    openai: { bg: '#10a37f15', fg: '#10a37f', border: '#10a37f30' },
    anthropic: { bg: '#d9775715', fg: '#d97757', border: '#d9775730' },
    google: { bg: '#4285f415', fg: '#4285f4', border: '#4285f430' },
    deepseek: { bg: '#4d6bfe15', fg: '#4d6bfe', border: '#4d6bfe30' },
    meta: { bg: '#0668e115', fg: '#0668e1', border: '#0668e130' },
    mistral: { bg: '#f59e0b15', fg: '#f59e0b', border: '#f59e0b30' },
    qwen: { bg: '#6366f115', fg: '#6366f1', border: '#6366f130' },
    xai: { bg: '#ffffff15', fg: '#ffffff', border: '#ffffff30' },
    cohere: { bg: '#39594d15', fg: '#39594d', border: '#39594d30' },
    ai21: { bg: '#f43f5e15', fg: '#f43f5e', border: '#f43f5e30' },
    reka: { bg: '#a855f715', fg: '#a855f7', border: '#a855f730' },
    perplexity: { bg: '#22c55e15', fg: '#22c55e', border: '#22c55e30' },
    commandcode: { bg: '#ef444415', fg: '#ef4444', border: '#ef444430' },
  };

  function providerStyle(id: string) {
    const c = providerColors[id] || { bg: `var(--color-primary-light)`, fg: `var(--color-primary)`, border: `var(--color-border)` };
    return `background: ${c.bg}; color: ${c.fg}; border: 1px solid ${c.border};`;
  }
</script>

<div style="animation: fadeInUp 0.4s ease-out;">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
    <div>
      <h1 style="font-size: 24px; font-weight: 700; color: var(--color-fg-0); letter-spacing: -0.5px; margin-bottom: 2px;">Providers</h1>
      <p style="font-size: 13px; color: var(--color-fg-3);">Model catalog with pricing &amp; capabilities</p>
    </div>
  </div>

  <!-- Stats strip -->
  <div class="card mb-5" style="padding: 0; overflow: hidden;">
    <div class="grid grid-cols-3" style="gap: 1px; background: var(--color-border);">
      {#each [
        { label: 'PROVIDERS', value: meta.totalProviders },
        { label: 'MODELS', value: meta.totalModels },
        { label: 'MATCHING', value: meta.visibleModels },
      ] as stat, i}
        <div class="text-center flex flex-col items-center justify-center" style="padding: 16px 20px; background: var(--color-bg-card); gap: 6px;">
          {#if i === 0}<Server size={16} style="color: var(--color-primary); opacity: 0.7;" />{:else if i === 1}<Cpu size={16} style="color: var(--color-primary); opacity: 0.7;" />{:else}<Search size={16} style="color: var(--color-primary); opacity: 0.7;" />{/if}
          <div>
            <div class="text-xs font-medium uppercase tracking-wider mb-1" style="color: var(--color-fg-3); font-size: 10px; letter-spacing: 0.8px;">{stat.label}</div>
            <div style="font-size: 22px; font-weight: 700; color: var(--color-fg-0); font-family: var(--font-mono);">{stat.value}</div>
          </div>
        </div>
      {/each}
    </div>
  </div>

  <!-- Search -->
  <div class="relative mb-5" style="max-width: 360px;">
    <Search size={16} class="absolute" style="left: 12px; top: 50%; transform: translateY(-50%); color: var(--color-fg-3); pointer-events: none;" />
    <input
      class="input-field"
      placeholder="Search models or providers..."
      bind:value={search}
      style="padding-left: 38px; font-size: 13px;"
    />
    {#if search}
      <button
        class="absolute"
        style="right: 10px; top: 50%; transform: translateY(-50%); color: var(--color-fg-3); font-size: 16px; line-height: 1; padding: 2px 6px; border-radius: 4px; opacity: 0.6;"
        onclick={() => search = ''}
      >×</button>
    {/if}
  </div>

  {#if loading}
    <Spinner />
  {:else if error}
    <div class="card text-center" style="color: var(--color-error); padding: 40px;">
      <Server size={32} style="margin: 0 auto 12px; opacity: 0.5;" />
      Failed to load: {error}
    </div>
  {:else if filtered.length === 0}
    <EmptyState icon={Server} title="No providers found" description={search ? `No models matching "${search}"` : 'No provider data available'} />
  {:else}
    <!-- Provider Grid -->
    <div class="grid grid-cols-1 lg:grid-cols-2" style="gap: 16px;">
      {#each filtered as provider (provider.id)}
        {@const isOpen = expanded.has(provider.id)}
        <div
          class="card provider-card"
          style="padding: 0; overflow: hidden;"
        >
          <!-- Provider Header -->
          <button
            class="w-full text-left flex items-center justify-between"
            style="padding: 16px 20px; cursor: pointer;"
            onclick={() => toggle(provider.id)}
          >
            <div class="flex items-center gap-3">
              <div
                class="flex items-center justify-center rounded-xl text-sm font-bold"
                style="{providerStyle(provider.id)} width: 40px; height: 40px; border-radius: 12px; font-size: 13px;"
              >
                {provider.name.charAt(0)}
              </div>
              <div>
                <div class="flex items-center gap-2 mb-0.5">
                  <span style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">{provider.name}</span>
                  <span class="badge" style="font-size: 10px; padding: 2px 6px; background: var(--color-border-light); color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">
                    {provider.format}
                  </span>
                </div>
                <div class="flex items-center gap-3" style="font-size: 11px; color: var(--color-fg-3); font-family: var(--font-mono);">
                  <span>{provider.base_url.replace('https://', '')}</span>
                </div>
              </div>
            </div>
            <div class="flex items-center gap-3">
              <span class="badge" style="background: var(--color-primary-light); color: var(--color-primary); font-weight: 600; padding: 3px 8px; font-size: 11px;">
                {provider.models.length} models
              </span>
              {#if isOpen}
                <ChevronDown size={18} style="color: var(--color-fg-2);" />
              {:else}
                <ChevronRight size={18} style="color: var(--color-fg-2);" />
              {/if}
            </div>
          </button>

          <!-- Models List -->
          {#if isOpen}
            <div style="border-top: 1px solid var(--color-border); animation: fadeInScale 0.25s ease-out;">
              <div class="overflow-x-auto">
                <table style="width: 100%; border-collapse: collapse; font-size: 12px;">
                  <thead>
                    <tr style="border-bottom: 1px solid var(--color-border);">
                      <th style="text-align: left; padding: 10px 20px; font-weight: 500; color: var(--color-fg-3); font-size: 10px; text-transform: uppercase; letter-spacing: 0.8px;">Model</th>
                      <th style="text-align: center; padding: 10px 12px; font-weight: 500; color: var(--color-fg-3); font-size: 10px; text-transform: uppercase; letter-spacing: 0.8px;">Context</th>
                      <th style="text-align: right; padding: 10px 12px; font-weight: 500; color: var(--color-fg-3); font-size: 10px; text-transform: uppercase; letter-spacing: 0.8px;">Input /1M</th>
                      <th style="text-align: right; padding: 10px 12px; font-weight: 500; color: var(--color-fg-3); font-size: 10px; text-transform: uppercase; letter-spacing: 0.8px;">Output /1M</th>
                      <th style="text-align: center; padding: 10px 20px; font-weight: 500; color: var(--color-fg-3); font-size: 10px; text-transform: uppercase; letter-spacing: 0.8px;">Features</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each provider.models as model, i}
                      <tr
                        style="border-bottom: {i < provider.models.length - 1 ? '1px solid var(--color-border)' : 'none'}; transition: background 0.15s;"
                        onmouseenter={e => (e.currentTarget as HTMLElement).style.background = 'var(--color-bg-sidebar-hover)'}
                        onmouseleave={e => (e.currentTarget as HTMLElement).style.background = 'transparent'}
                      >
                        <td style="padding: 10px 20px;">
                          <div style="font-weight: 600; color: var(--color-fg-0); font-size: 12px; font-family: var(--font-mono); margin-bottom: 1px;">{model.name}</div>
                          <div style="color: var(--color-fg-3); font-size: 10px; font-family: var(--font-mono);">{model.id}</div>
                        </td>
                        <td style="text-align: center; padding: 10px 12px;">
                          <span style="color: var(--color-fg-1); font-family: var(--font-mono); font-size: 11px;">{fmtTokens(model.context_window)}</span>
                        </td>
                        <td style="text-align: right; padding: 10px 12px;">
                          <span style="color: var(--color-fg-1); font-family: var(--font-mono); font-size: 11px;">{fmtPrice(model.input_price_per_1m)}</span>
                        </td>
                        <td style="text-align: right; padding: 10px 12px;">
                          <span style="color: var(--color-fg-1); font-family: var(--font-mono); font-size: 11px;">{fmtPrice(model.output_price_per_1m)}</span>
                        </td>
                        <td style="padding: 10px 20px;">
                          <div class="flex justify-center gap-1.5">
                            {#each model.capabilities as cap}
                              <span
                                class="flex items-center gap-1"
                                style="padding: 2px 6px; border-radius: 4px; font-size: 10px; font-weight: 500;
                                  background: {capColor(cap)}15; color: {capColor(cap)};"
                                title={cap}
                              >
                                <span style="display: flex; align-items: center;">
                                  {#if cap === 'chat'}<Brain size={10} />{:else if cap === 'vision'}<Eye size={10} />{:else if cap === 'tools'}<Wrench size={10} />{:else if cap === 'streaming'}<Activity size={10} />{:else if cap === 'json_mode'}<FileJson size={10} />{:else}<Zap size={10} />{/if}
                                </span>
                                {cap.replace('_', ' ')}
                              </span>
                            {/each}
                          </div>
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .provider-card {
    transition: box-shadow 0.2s;
  }

  .provider-card:hover {
    box-shadow: var(--shadow-md);
  }

  :global(.input-field) {
    width: 100%;
    padding: 10px 14px;
    border-radius: 8px;
    border: 1px solid var(--color-border);
    background: var(--color-bg-card);
    color: var(--color-fg-0);
    font-size: 13px;
    transition: border-color 0.2s, box-shadow 0.2s;
  }
  :global(.input-field:focus) {
    outline: none;
    border-color: var(--color-primary);
    box-shadow: 0 0 0 3px var(--color-primary-glow);
  }
</style>
