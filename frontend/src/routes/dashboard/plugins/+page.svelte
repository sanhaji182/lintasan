<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import {
    Puzzle, Store, Sparkles, Settings, Trash2, Power,
    PowerOff, Search, Download, Tag, ChevronRight,
    CircleAlert, CheckCircle2, Box, Info, X, Loader2, Wand2, Eye
  } from 'lucide-svelte/icons';

  interface Plugin {
    id: string;
    name: string;
    description: string;
    version: string;
    enabled: boolean;
    author?: string;
    categories?: string[];
    installedAt?: string;
    config?: Record<string, unknown>;
  }

  interface StorePlugin {
    id: string;
    name: string;
    description: string;
    version: string;
    author: string;
    categories: string[];
    downloads: number;
    installed: boolean;
  }

  let activeTab = $state<'installed' | 'store' | 'generate'>('installed');
  let installedPlugins = $state<Plugin[]>([]);
  let storePlugins = $state<StorePlugin[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Installed tab state
  let configuringPlugin = $state<Plugin | null>(null);
  let configJson = $state('');
  let togglingPlugin = $state<string | null>(null);
  let uninstallingPlugin = $state<string | null>(null);

  // Store tab state
  let storeSearch = $state('');
  let storeCategory = $state('all');
  let installingPlugin = $state<string | null>(null);

  // AI Generate tab state
  let generatePrompt = $state('');
  let generating = $state(false);
  let generatedPlugin = $state<{ name: string; description: string; code: string } | null>(null);
  let generateError = $state<string | null>(null);

  const categoryColors: Record<string, string> = {
    'utility': 'var(--color-info-light)',
    'auth': 'var(--color-success-light)',
    'logging': 'var(--color-warning-light)',
    'caching': 'var(--color-purple-light)',
    'transform': 'var(--color-primary-light)',
    'monitoring': 'var(--color-error-light)',
    'security': 'var(--color-error-light)',
    'integration': 'var(--color-info-light)',
  };
  const categoryFg: Record<string, string> = {
    'utility': 'var(--color-info)',
    'auth': 'var(--color-success)',
    'logging': 'var(--color-warning)',
    'caching': 'var(--color-purple)',
    'transform': 'var(--color-primary)',
    'monitoring': 'var(--color-error)',
    'security': 'var(--color-error)',
    'integration': 'var(--color-info)',
  };

  async function loadInstalled() {
    try {
      const res = await api.get<any>('/api/plugins');
      installedPlugins = res?.data || res?.plugins || [];
    } catch (e: any) {
      error = e.message || 'Failed to load plugins';
    }
  }

  async function loadStore() {
    try {
      const res = await api.get<any>('/api/plugins/store');
      const raw = res?.data || res?.plugins || [];
      storePlugins = Array.isArray(raw) ? raw.map((w: any) => ({
        id: w.id || w.name || '',
        name: w.name || '',
        description: w.description || '',
        version: w.version || '1.0.0',
        author: w.author || 'Unknown',
        categories: Array.isArray(w.categories) ? w.categories : (w.category ? [w.category] : []),
        downloads: w.downloads || 0,
        installed: !!w.installed
      })) : [];
    } catch {
      // Store may not be available
    }
  }

  onMount(async () => {
    loading = true;
    await Promise.all([loadInstalled(), loadStore()]);
    loading = false;
  });

  async function togglePlugin(plugin: Plugin) {
    togglingPlugin = plugin.id;
    try {
      await api.patch(`/api/plugins/${plugin.id}`, { enabled: !plugin.enabled });
      plugin.enabled = !plugin.enabled;
      installedPlugins = [...installedPlugins];
    } catch (e: any) {
      error = e.message || 'Failed to toggle plugin';
    } finally {
      togglingPlugin = null;
    }
  }

  async function uninstallPlugin(plugin: Plugin) {
    if (!confirm(`Uninstall "${plugin.name}"? This cannot be undone.`)) return;
    uninstallingPlugin = plugin.id;
    try {
      await api.delete(`/api/plugins/${plugin.id}`);
      installedPlugins = installedPlugins.filter(p => p.id !== plugin.id);
    } catch (e: any) {
      error = e.message || 'Failed to uninstall plugin';
    } finally {
      uninstallingPlugin = null;
    }
  }

  function openConfig(plugin: Plugin) {
    configuringPlugin = plugin;
    configJson = JSON.stringify(plugin.config || {}, null, 2);
  }

  function closeConfigModal() {
    configuringPlugin = null;
  }

  function handleConfigOverlayKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' || event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      closeConfigModal();
    }
  }

  function stopConfigClose(event: MouseEvent) {
    event.stopPropagation();
  }

  function stopConfigCloseKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.stopPropagation();
      closeConfigModal();
    }
  }

  async function saveConfig() {
    if (!configuringPlugin) return;
    try {
      const parsed = JSON.parse(configJson);
      await api.patch(`/api/plugins/${configuringPlugin.id}/config`, { config: parsed });
      configuringPlugin.config = parsed;
      installedPlugins = [...installedPlugins];
      closeConfigModal();
    } catch (e: any) {
      error = e.message || 'Invalid JSON or failed to save config';
    }
  }

  async function installFromStore(plugin: StorePlugin) {
    installingPlugin = plugin.id;
    try {
      await api.post(`/api/plugins/install`, { pluginId: plugin.id });
      plugin.installed = true;
      storePlugins = [...storePlugins];
      await loadInstalled();
    } catch (e: any) {
      error = e.message || 'Failed to install plugin';
    } finally {
      installingPlugin = null;
    }
  }

  async function generatePlugin() {
    if (!generatePrompt.trim()) return;
    generating = true;
    generateError = null;
    generatedPlugin = null;
    try {
      const data = await api.post<{ name: string; description: string; code: string }>(
        '/api/plugins/generate',
        { prompt: generatePrompt }
      );
      generatedPlugin = data;
    } catch (e: any) {
      generateError = e.message || 'Failed to generate plugin';
    } finally {
      generating = false;
    }
  }

  let filteredStorePlugins = $derived(
    storePlugins.filter(p => {
      const matchesSearch = !storeSearch.trim() ||
        p.name.toLowerCase().includes(storeSearch.toLowerCase()) ||
        p.description.toLowerCase().includes(storeSearch.toLowerCase());
      const matchesCategory = storeCategory === 'all' ||
        (p.categories || []).includes(storeCategory);
      return matchesSearch && matchesCategory;
    })
  );

  let allCategories = $derived(
    [...new Set(storePlugins.flatMap(p => p.categories || []))].sort()
  );

  const tabs = [
    { id: 'installed' as const, label: 'Installed', icon: Puzzle },
    { id: 'store' as const, label: 'Store', icon: Store },
    { id: 'generate' as const, label: 'AI Generate', icon: Sparkles },
  ];
</script>

<svelte:head>
  <title>Plugins — Lintasan</title>
</svelte:head>

<div style="display: flex; flex-direction: column; gap: 24px;">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div>
      <h2 style="font-size: 20px; font-weight: 700; color: var(--color-fg-0); letter-spacing: -0.3px;">
        Plugin Management
      </h2>
      <p style="font-size: 13px; color: var(--color-fg-2); margin-top: 2px;">
        Extend your gateway with plugins, browse the store, or generate with AI
      </p>
    </div>
  </div>

  <!-- Tabs -->
  <div class="card" style="padding: 0; overflow: hidden;">
    <div
      class="flex"
      style="border-bottom: 1px solid var(--color-border);"
    >
      {#each tabs as tab}
        <button
          class="tab-btn"
          class:active={activeTab === tab.id}
          onclick={() => activeTab = tab.id}
        >
          <tab.icon size={16} />
          <span>{tab.label}</span>
        </button>
      {/each}
    </div>

    <div style="padding: 24px;">
      {#if loading}
        <Spinner />
      {:else}
        <!-- INSTALLED TAB -->
        {#if activeTab === 'installed'}
          {#if installedPlugins.length === 0}
            <EmptyState
              icon={Puzzle}
              title="No plugins installed"
              description="Browse the store or generate a plugin with AI to get started"
            />
          {:else}
            <div style="display: flex; flex-direction: column; gap: 12px;">
              {#each installedPlugins as plugin, i}
                <div
                  class="plugin-card"
                  style="animation: fadeInUp {0.3 + i * 0.05}s ease-out;"
                >
                  <div class="flex items-start justify-between">
                    <div class="flex items-start gap-3" style="flex: 1; min-width: 0;">
                      <div
                        class="flex items-center justify-center rounded-lg"
                        style="width: 40px; height: 40px; background: {plugin.enabled ? 'var(--color-primary-light)' : 'var(--color-bg-body)'}; flex-shrink: 0;"
                      >
                        <Puzzle
                          size={20}
                          style="color: {plugin.enabled ? 'var(--color-primary)' : 'var(--color-fg-3)'};"
                        />
                      </div>
                      <div style="min-width: 0;">
                        <div class="flex items-center gap-2">
                          <span style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">
                            {plugin.name}
                          </span>
                          <span
                            style="font-size: 11px; color: var(--color-fg-3); font-family: var(--font-mono);"
                          >v{plugin.version}</span>
                        </div>
                        <p style="font-size: 13px; color: var(--color-fg-2); margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                          {plugin.description}
                        </p>
                        {#if plugin.categories && plugin.categories.length > 0}
                          <div class="flex items-center gap-1.5" style="margin-top: 6px;">
                            {#each plugin.categories as cat}
                              <span
                                class="badge"
                                style="font-size: 10px; padding: 2px 8px; background: {categoryColors[cat] || 'var(--color-border-light)'}; color: {categoryFg[cat] || 'var(--color-fg-2)'};"
                              >{cat}</span>
                            {/each}
                          </div>
                        {/if}
                      </div>
                    </div>

                    <div class="flex items-center gap-2" style="flex-shrink: 0;">
                      <!-- Enable/Disable Toggle -->
                      <button
                        class="action-btn"
                        class:enabled={plugin.enabled}
                        onclick={() => togglePlugin(plugin)}
                        disabled={togglingPlugin === plugin.id}
                        title={plugin.enabled ? 'Disable plugin' : 'Enable plugin'}
                      >
                        {#if togglingPlugin === plugin.id}
                          <Loader2 size={14} class="spin-icon" />
                        {:else if plugin.enabled}
                          <Power size={14} />
                        {:else}
                          <PowerOff size={14} />
                        {/if}
                      </button>

                      <!-- Config -->
                      <button
                        class="action-btn"
                        onclick={() => openConfig(plugin)}
                        title="Configure plugin"
                      >
                        <Settings size={14} />
                      </button>

                      <!-- Uninstall -->
                      <button
                        class="action-btn danger"
                        onclick={() => uninstallPlugin(plugin)}
                        disabled={uninstallingPlugin === plugin.id}
                        title="Uninstall plugin"
                      >
                        {#if uninstallingPlugin === plugin.id}
                          <Loader2 size={14} class="spin-icon" />
                        {:else}
                          <Trash2 size={14} />
                        {/if}
                      </button>
                    </div>
                  </div>
                </div>
              {/each}
            </div>
          {/if}

        <!-- STORE TAB -->
        {:else if activeTab === 'store'}
          <!-- Search & Filter -->
          <div class="flex items-center gap-3 flex-wrap" style="margin-bottom: 20px;">
            <div class="search-wrapper">
              <Search size={14} style="color: var(--color-fg-3); position: absolute; left: 10px; top: 50%; transform: translateY(-50%); pointer-events: none;" />
              <input
                class="input-field search-input"
                placeholder="Search plugins..."
                bind:value={storeSearch}
              />
            </div>
            <select
              class="input-field"
              style="width: 160px; font-size: 12px; padding: 7px 10px;"
              bind:value={storeCategory}
            >
              <option value="all">All Categories</option>
              {#each allCategories as cat}
                <option value={cat}>{cat.charAt(0).toUpperCase() + cat.slice(1)}</option>
              {/each}
            </select>
          </div>

          {#if filteredStorePlugins.length === 0}
            <EmptyState
              icon={Store}
              title="No plugins found"
              description={storeSearch.trim() ? 'Try adjusting your search terms' : 'The plugin store is empty or unavailable'}
            />
          {:else}
            <div
              class="grid gap-4"
              style="grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));"
            >
              {#each filteredStorePlugins as plugin, i}
                <div
                  class="store-card"
                  style="animation: fadeInUp {0.3 + i * 0.05}s ease-out;"
                >
                  <div class="flex items-start justify-between" style="margin-bottom: 12px;">
                    <div>
                      <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">
                        {plugin.name}
                      </div>
                      <div style="font-size: 12px; color: var(--color-fg-3); margin-top: 2px;">
                        by {plugin.author} · v{plugin.version}
                      </div>
                    </div>
                    <div class="flex items-center gap-1" style="font-size: 11px; color: var(--color-fg-3);">
                      <Download size={12} />
                      {plugin.downloads.toLocaleString()}
                    </div>
                  </div>

                  <p style="font-size: 13px; color: var(--color-fg-2); line-height: 1.5; margin-bottom: 12px;">
                    {plugin.description}
                  </p>

                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-1.5 flex-wrap">
                      {#each (plugin.categories || []) as cat}
                        <span
                          class="badge"
                          style="font-size: 10px; padding: 2px 8px; background: {categoryColors[cat] || 'var(--color-border-light)'}; color: {categoryFg[cat] || 'var(--color-fg-2)'};"
                        >{cat}</span>
                      {/each}
                    </div>

                    {#if plugin.installed}
                      <span class="flex items-center gap-1" style="font-size: 12px; color: var(--color-success); font-weight: 500;">
                        <CheckCircle2 size={14} />
                        Installed
                      </span>
                    {:else}
                      <button
                        class="btn-primary"
                        style="padding: 6px 14px; font-size: 12px; display: flex; align-items: center; gap: 4px;"
                        onclick={() => installFromStore(plugin)}
                        disabled={installingPlugin === plugin.id}
                      >
                        {#if installingPlugin === plugin.id}
                          <Loader2 size={12} class="spin-icon" />
                          Installing...
                        {:else}
                          <Download size={12} />
                          Install
                        {/if}
                      </button>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          {/if}

        <!-- AI GENERATE TAB -->
        {:else if activeTab === 'generate'}
          <div style="max-width: 720px;">
            <div style="margin-bottom: 20px;">
              <label
                for="generate-input"
                style="display: block; font-size: 13px; font-weight: 600; color: var(--color-fg-1); margin-bottom: 8px;"
              >
                Describe your plugin
              </label>
              <textarea
                id="generate-input"
                class="input-field"
                rows="4"
                placeholder="e.g., A rate-limiter plugin that limits requests to 100 per minute per API key, with configurable window and burst size..."
                bind:value={generatePrompt}
                style="resize: vertical;"
              ></textarea>
            </div>

            <button
              class="btn-primary"
              onclick={generatePlugin}
              disabled={generating || !generatePrompt.trim()}
              style="display: flex; align-items: center; gap: 8px; margin-bottom: 24px;"
            >
              {#if generating}
                <Loader2 size={16} class="spin-icon" />
                Generating...
              {:else}
                <Wand2 size={16} />
                Generate Plugin
              {/if}
            </button>

            {#if generateError}
              <div
                class="flex items-center gap-2"
                style="
                  padding: 12px 16px; border-radius: var(--radius-sm);
                  background: var(--color-error-light); color: var(--color-error);
                  font-size: 13px; font-weight: 500; margin-bottom: 20px;
                "
              >
                <CircleAlert size={16} />
                {generateError}
              </div>
            {/if}

            {#if generatedPlugin}
              <div class="card" style="animation: fadeInUp 0.4s ease-out;">
                <div class="flex items-center gap-2" style="margin-bottom: 16px;">
                  <Sparkles size={18} style="color: var(--color-purple);" />
                  <span style="font-size: 16px; font-weight: 600; color: var(--color-fg-0);">
                    {generatedPlugin.name}
                  </span>
                </div>

                <p style="font-size: 13px; color: var(--color-fg-2); margin-bottom: 16px; line-height: 1.6;">
                  {generatedPlugin.description}
                </p>

                <div
                  style="
                    background: var(--color-bg-body);
                    border: 1px solid var(--color-border);
                    border-radius: var(--radius-sm);
                    padding: 16px;
                    overflow-x: auto;
                    margin-bottom: 16px;
                  "
                >
                  <pre style="font-family: var(--font-mono); font-size: 12px; color: var(--color-fg-1); line-height: 1.6; margin: 0; white-space: pre-wrap;">{generatedPlugin.code}</pre>
                </div>

                <div class="flex items-center gap-2">
                  <button class="btn-primary" style="display: flex; align-items: center; gap: 6px;">
                    <Download size={14} />
                    Install Plugin
                  </button>
                  <button class="btn-secondary" style="display: flex; align-items: center; gap: 6px;">
                    <Eye size={14} />
                    Preview
                  </button>
                </div>
              </div>
            {/if}
          </div>
        {/if}
      {/if}
    </div>
  </div>

  <!-- Error banner -->
  {#if error}
    <div
      class="flex items-center gap-2"
      style="
        padding: 12px 16px; border-radius: var(--radius-sm);
        background: var(--color-error-light); color: var(--color-error);
        font-size: 13px; font-weight: 500;
      "
    >
      <CircleAlert size={16} />
      {error}
      <button
        style="margin-left: auto; cursor: pointer; color: var(--color-error); background: none; border: none;"
        onclick={() => error = null}
      >&times;</button>
    </div>
  {/if}
</div>

<!-- Config Modal -->
{#if configuringPlugin}
  <div
    class="modal-backdrop"
    role="button"
    tabindex="0"
    aria-label="Close plugin config modal"
    onclick={closeConfigModal}
    onkeydown={handleConfigOverlayKeydown}
  >
    <div
      class="modal-card card"
      role="dialog"
      aria-modal="true"
      aria-label="Plugin configuration dialog"
      tabindex="-1"
      onclick={stopConfigClose}
      onkeydown={stopConfigCloseKeydown}
      style="animation: fadeInScale 0.3s ease-out;"
    >
      <div class="flex items-center justify-between" style="margin-bottom: 16px;">
        <div class="flex items-center gap-2">
          <Settings size={18} style="color: var(--color-primary);" />
          <span style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">
            Configure {configuringPlugin.name}
          </span>
        </div>
        <button
          class="action-btn"
          onclick={closeConfigModal}
        >
          <X size={16} />
        </button>
      </div>

      <textarea
        class="input-field"
        rows="10"
        bind:value={configJson}
        style="font-family: var(--font-mono); font-size: 12px; resize: vertical;"
          placeholder="&#123;&#125;"
      ></textarea>

      <div class="flex items-center justify-end gap-2" style="margin-top: 16px;">
        <button class="btn-secondary" onclick={closeConfigModal}>Cancel</button>
        <button class="btn-primary" onclick={saveConfig}>Save Configuration</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .tab-btn {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 14px 20px;
    font-size: 13px;
    font-weight: 500;
    color: var(--color-fg-2);
    background: transparent;
    border: none;
    border-bottom: 2px solid transparent;
    cursor: pointer;
    transition: var(--transition);
  }
  .tab-btn:hover {
    color: var(--color-fg-0);
    background: var(--color-bg-body);
  }
  .tab-btn.active {
    color: var(--color-primary);
    border-bottom-color: var(--color-primary);
    font-weight: 600;
  }

  .plugin-card {
    padding: 16px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    background: var(--color-bg-card);
    transition: var(--transition);
  }
  .plugin-card:hover {
    border-color: var(--color-primary);
    box-shadow: 0 0 0 3px var(--color-primary-glow);
  }

  .store-card {
    padding: 20px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-bg-card);
    transition: var(--transition);
  }
  .store-card:hover {
    border-color: var(--color-primary);
    box-shadow: var(--shadow-md);
  }

  .action-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--color-border);
    background: transparent;
    color: var(--color-fg-2);
    cursor: pointer;
    transition: var(--transition);
  }
  .action-btn:hover {
    background: var(--color-bg-body);
    color: var(--color-fg-0);
    border-color: var(--color-fg-3);
  }
  .action-btn.enabled {
    background: var(--color-success-light);
    color: var(--color-success);
    border-color: var(--color-success);
  }
  .action-btn.danger:hover {
    background: var(--color-error-light);
    color: var(--color-error);
    border-color: var(--color-error);
  }
  .action-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .search-wrapper {
    position: relative;
    flex: 1;
    min-width: 200px;
    max-width: 360px;
  }
  .search-input {
    padding-left: 32px !important;
  }

  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
    padding: 24px;
  }
  .modal-card {
    width: 100%;
    max-width: 560px;
    max-height: 80vh;
    overflow-y: auto;
  }

  :global(.spin-icon) {
    animation: spin 0.8s linear infinite;
  }
</style>
