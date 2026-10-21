<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { showToast } from '$lib/toast';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import {
    Server, ChevronDown, ChevronRight, Search, Brain, Zap, Eye, Wrench,
    FileJson, Activity, ExternalLink, Cpu, Plus, CheckCircle2,
    AlertCircle, Link2, Key, Sliders, X, ShieldCheck
  } from 'lucide-svelte';

  interface Model {
    id: string;
    name: string;
    provider: string;
    context_window: number;
    max_tokens: number;
    input_price_per_1m: number;
    output_price_per_1m: number;
    capabilities: string[];
  }

  interface ConnectedAccount {
    id: string;
    name: string;
    is_active: number;
    priority: number;
    format: string;
    base_url: string;
    created_at?: string;
  }

  interface Provider {
    id: string;
    name: string;
    base_url: string;
    format: string;
    isOAuth?: boolean;
    oauthProvider?: string;
    description?: string;
    model_count: number;
    models: Model[];
    accounts: ConnectedAccount[];
    activeCount: number;
  }

  let providers = $state<Provider[]>([]);
  let allConnections = $state<any[]>([]);
  let loading = $state(true);
  let error = $state('');
  let search = $state('');
  let statusFilter = $state<'all' | 'connected' | 'not_configured' | 'oauth'>('all');
  let expanded = $state<Set<string>>(new Set());
  let expandedAccounts = $state<Set<string>>(new Set());

  // Modal State for Quick Add
  let showAddModal = $state(false);
  let modalProvider = $state<Provider | null>(null);
  let addFormName = $state('');
  let addFormApiKey = $state('');
  let addFormBaseUrl = $state('');
  let addFormFormat = $state('openai');
  let addFormPriority = $state(10);
  let isSavingConnection = $state(false);

  const EXTRA_KNOWN_PROVIDERS = [
    {
      id: 'antigravity',
      name: 'Google Antigravity',
      base_url: 'https://daily-cloudcode-pa.googleapis.com',
      format: 'openai',
      isOAuth: true,
      oauthProvider: 'antigravity',
      description: 'Google Cloud Code Assist & Gemini models via OAuth IDE',
      models: [
        { id: 'gemini-2.5-pro', name: 'Gemini 2.5 Pro', provider: 'Google Antigravity', context_window: 1048576, max_tokens: 8192, input_price_per_1m: 0, output_price_per_1m: 0, capabilities: ['chat', 'vision', 'tools', 'streaming'] },
        { id: 'gemini-2.5-flash', name: 'Gemini 2.5 Flash', provider: 'Google Antigravity', context_window: 1048576, max_tokens: 8192, input_price_per_1m: 0, output_price_per_1m: 0, capabilities: ['chat', 'vision', 'tools', 'streaming'] },
        { id: 'gemini-2.0-flash-exp', name: 'Gemini 2.0 Flash Exp', provider: 'Google Antigravity', context_window: 1048576, max_tokens: 8192, input_price_per_1m: 0, output_price_per_1m: 0, capabilities: ['chat', 'vision', 'tools', 'streaming'] }
      ]
    },
    {
      id: 'qoder',
      name: 'Qoder AI',
      base_url: 'https://openapi.qoder.sh',
      format: 'openai',
      isOAuth: false,
      description: 'High-concurrency multi-account proxy pool for coding agents',
      models: [
        { id: 'claude-3-7-sonnet', name: 'Claude 3.7 Sonnet', provider: 'Qoder AI', context_window: 200000, max_tokens: 8192, input_price_per_1m: 0, output_price_per_1m: 0, capabilities: ['chat', 'vision', 'tools', 'streaming'] },
        { id: 'claude-3-5-sonnet', name: 'Claude 3.5 Sonnet', provider: 'Qoder AI', context_window: 200000, max_tokens: 8192, input_price_per_1m: 0, output_price_per_1m: 0, capabilities: ['chat', 'vision', 'tools', 'streaming'] },
        { id: 'gpt-4o', name: 'GPT-4o', provider: 'Qoder AI', context_window: 128000, max_tokens: 4096, input_price_per_1m: 0, output_price_per_1m: 0, capabilities: ['chat', 'tools', 'streaming'] }
      ]
    },
    {
      id: 'copilot',
      name: 'GitHub Copilot',
      base_url: 'https://api.githubcopilot.com',
      format: 'openai',
      isOAuth: true,
      oauthProvider: 'github',
      description: 'GitHub Copilot Chat & Completion via OAuth Device Flow',
      models: [
        { id: 'gpt-4o', name: 'GPT-4o (Copilot)', provider: 'GitHub Copilot', context_window: 128000, max_tokens: 4096, input_price_per_1m: 0, output_price_per_1m: 0, capabilities: ['chat', 'tools', 'streaming'] },
        { id: 'claude-3.5-sonnet', name: 'Claude 3.5 Sonnet (Copilot)', provider: 'GitHub Copilot', context_window: 200000, max_tokens: 8192, input_price_per_1m: 0, output_price_per_1m: 0, capabilities: ['chat', 'tools', 'streaming'] }
      ]
    },
    {
      id: 'opencode',
      name: 'OpenCode Free',
      base_url: 'https://opencode.ai/zen/go/v1',
      format: 'openai',
      isOAuth: false,
      description: 'OpenCode Zen community gateway endpoint',
      models: [
        { id: 'opencode-zen-chat', name: 'OpenCode Zen Chat', provider: 'OpenCode Free', context_window: 64000, max_tokens: 4096, input_price_per_1m: 0, output_price_per_1m: 0, capabilities: ['chat', 'streaming'] }
      ]
    }
  ];

  function extractProviderKey(conn: any): string {
    if (conn.oauth_provider) {
      const p = conn.oauth_provider.toLowerCase();
      if (p === 'antigravity') return 'antigravity';
      if (p === 'github' || p === 'copilot') return 'copilot';
      if (p === 'xai') return 'xai';
      if (p === 'claude') return 'anthropic';
      if (p === 'codex') return 'openai';
      return p;
    }
    const name = (conn.name || '').toLowerCase();
    const url = (conn.base_url || '').toLowerCase();
    if (name.includes('qoder') || url.includes('qoder')) return 'qoder';
    if (name.includes('antigravity') || url.includes('cloudcode-pa') || url.includes('googleapis')) return 'antigravity';
    if (name.includes('copilot') || url.includes('githubcopilot')) return 'copilot';
    if (name.includes('opencode') || url.includes('opencode.ai')) return 'opencode';
    if (name.includes('openai') || url.includes('api.openai.com')) return 'openai';
    if (name.includes('anthropic') || name.includes('claude') || url.includes('anthropic.com')) return 'anthropic';
    if (name.includes('deepseek') || url.includes('deepseek.com')) return 'deepseek';
    if (name.includes('xai') || name.includes('grok') || url.includes('x.ai')) return 'xai';
    if (name.includes('gemini') || url.includes('generativelanguage.googleapis')) return 'google';
    if (name.includes('commandcode') || url.includes('commandcode')) return 'commandcode';
    if (name.includes('groq') || url.includes('groq.com')) return 'groq';
    if (name.includes('openrouter') || url.includes('openrouter.ai')) return 'openrouter';
    if (name.includes('mistral') || url.includes('mistral.ai')) return 'mistral';
    if (name.includes('meta') || name.includes('llama')) return 'meta';
    if (name.includes('qwen') || name.includes('alibaba')) return 'qwen';
    if (name.includes('cohere') || url.includes('cohere.com')) return 'cohere';
    if (name.includes('perplexity') || url.includes('perplexity.ai')) return 'perplexity';
    if (name.includes('mimo') || url.includes('mimo')) return 'xiaomi';
    return conn.pool_id ? conn.pool_id.toLowerCase() : 'custom';
  }

  async function loadData() {
    loading = true;
    error = '';
    try {
      const [catRes, connsRes, oauthRes] = await Promise.all([
        api.get<any>('/api/models/catalog'),
        api.get<any>('/api/connections'),
        api.get<any>('/api/oauth/sessions').catch(() => ({ data: [] }))
      ]);

      const rawCatalog = catRes.data || [];
      const conns = connsRes.data || [];
      allConnections = conns;
      const oauthSessions = oauthRes.data || [];

      // Build initial list from catalog
      const list: Provider[] = rawCatalog.map((p: any) => ({
        id: p.id,
        name: p.name,
        base_url: p.base_url,
        format: p.format,
        isOAuth: p.id === 'xai' || p.id === 'anthropic' || p.id === 'openai',
        oauthProvider: p.id === 'xai' ? 'xai' : p.id === 'anthropic' ? 'claude' : p.id === 'openai' ? 'codex' : undefined,
        description: undefined,
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
        })),
        accounts: [],
        activeCount: 0,
      }));

      // Add extra known providers if not already in catalog
      for (const extra of EXTRA_KNOWN_PROVIDERS) {
        if (!list.some(p => p.id === extra.id)) {
          list.push({
            id: extra.id,
            name: extra.name,
            base_url: extra.base_url,
            format: extra.format,
            isOAuth: extra.isOAuth,
            oauthProvider: extra.oauthProvider,
            description: extra.description,
            model_count: extra.models.length,
            models: extra.models,
            accounts: [],
            activeCount: 0,
          });
        }
      }

      // Map connections to providers
      for (const conn of conns) {
        const key = extractProviderKey(conn);
        let target = list.find(p => p.id === key);
        if (!target) {
          // Try matching by name
          target = list.find(p => p.name.toLowerCase().includes(key));
        }

        const accountItem: ConnectedAccount = {
          id: conn.id,
          name: conn.name || 'Unnamed Account',
          is_active: conn.is_active ?? 1,
          priority: conn.priority ?? 10,
          format: conn.format || 'openai',
          base_url: conn.base_url || '',
          created_at: conn.created_at
        };

        if (target) {
          target.accounts.push(accountItem);
          if (accountItem.is_active === 1) target.activeCount++;
        }
      }

      // Check if Antigravity has active OAuth session even if not yet in conns
      const antigravitySession = oauthSessions.find((s: any) => s.provider === 'antigravity' && s.status === 'active');
      const antigravityProvider = list.find(p => p.id === 'antigravity');
      if (antigravitySession && antigravityProvider && antigravityProvider.accounts.length === 0) {
        antigravityProvider.accounts.push({
          id: antigravitySession.id,
          name: 'Google Antigravity (Active Session)',
          is_active: 1,
          priority: 10,
          format: 'openai',
          base_url: 'https://daily-cloudcode-pa.googleapis.com',
          created_at: antigravitySession.created_at
        });
        antigravityProvider.activeCount = 1;
      }

      // Sort: connected providers first (by activeCount desc), then alphabetically
      list.sort((a, b) => {
        if (a.activeCount !== b.activeCount) return b.activeCount - a.activeCount;
        if (a.accounts.length !== b.accounts.length) return b.accounts.length - a.accounts.length;
        return a.name.localeCompare(b.name);
      });

      providers = list;
    } catch (e: any) {
      error = e.message || 'Failed to load providers';
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    loadData();
  });

  const filtered = $derived.by(() => {
    let list = providers;

    // Filter by status tab
    if (statusFilter === 'connected') {
      list = list.filter(p => p.activeCount > 0);
    } else if (statusFilter === 'not_configured') {
      list = list.filter(p => p.accounts.length === 0);
    } else if (statusFilter === 'oauth') {
      list = list.filter(p => p.isOAuth);
    }

    // Filter by search
    if (search.trim()) {
      const q = search.trim().toLowerCase();
      list = list.filter(p =>
        p.name.toLowerCase().includes(q) ||
        p.id.toLowerCase().includes(q) ||
        p.format.toLowerCase().includes(q) ||
        p.models.some(m => m.name.toLowerCase().includes(q) || m.id.toLowerCase().includes(q)) ||
        p.accounts.some(a => a.name.toLowerCase().includes(q))
      );
    }

    return list;
  });

  const statsMeta = $derived.by(() => {
    const totalProviders = providers.length;
    const connectedProviders = providers.filter(p => p.activeCount > 0).length;
    const totalActiveAccounts = allConnections.filter(c => c.is_active === 1).length;
    const totalModels = providers.reduce((s, p) => s + p.models.length, 0);

    return {
      totalProviders,
      connectedProviders,
      totalActiveAccounts,
      totalModels,
      filteredCount: filtered.length
    };
  });

  function toggleExpandModels(id: string) {
    const next = new Set(expanded);
    next.has(id) ? next.delete(id) : next.add(id);
    expanded = next;
  }

  function toggleExpandAccounts(id: string) {
    const next = new Set(expandedAccounts);
    next.has(id) ? next.delete(id) : next.add(id);
    expandedAccounts = next;
  }

  function openAddModal(p: Provider) {
    modalProvider = p;
    addFormName = `${p.name} Account`;
    addFormApiKey = '';
    addFormBaseUrl = p.base_url;
    addFormFormat = p.format || 'openai';
    addFormPriority = 10;
    showAddModal = true;
  }

  function closeAddModal() {
    showAddModal = false;
    modalProvider = null;
    addFormApiKey = '';
  }

  async function handleSaveConnection() {
    if (!modalProvider) return;
    if (!addFormApiKey.trim() && !modalProvider.isOAuth) {
      showToast('API Key is required', 'warning');
      return;
    }

    isSavingConnection = true;
    try {
      await api.post('/api/connections', {
        name: addFormName.trim() || `${modalProvider.name} Account`,
        base_url: addFormBaseUrl.trim() || modalProvider.base_url,
        api_key: addFormApiKey.trim(),
        format: addFormFormat || 'openai',
        priority: Number(addFormPriority) || 10,
        is_active: 1
      });

      showToast(`Connected ${modalProvider.name} successfully!`, 'success');
      closeAddModal();
      await loadData();
    } catch (e: any) {
      showToast(e.message || 'Failed to save connection', 'error');
    } finally {
      isSavingConnection = false;
    }
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

  const providerColors: Record<string, { bg: string; fg: string; border: string }> = {
    antigravity: { bg: '#4285f418', fg: '#4285f4', border: '#4285f435' },
    qoder: { bg: '#8b5cf618', fg: '#8b5cf6', border: '#8b5cf635' },
    copilot: { bg: '#10b98118', fg: '#10b981', border: '#10b98135' },
    opencode: { bg: '#06b6d418', fg: '#06b6d4', border: '#06b6d435' },
    openai: { bg: '#10a37f18', fg: '#10a37f', border: '#10a37f35' },
    anthropic: { bg: '#d9775718', fg: '#d97757', border: '#d9775735' },
    google: { bg: '#ea433518', fg: '#ea4335', border: '#ea433535' },
    deepseek: { bg: '#4d6bfe18', fg: '#4d6bfe', border: '#4d6bfe35' },
    meta: { bg: '#0668e118', fg: '#0668e1', border: '#0668e135' },
    mistral: { bg: '#f59e0b18', fg: '#f59e0b', border: '#f59e0b35' },
    qwen: { bg: '#6366f118', fg: '#6366f1', border: '#6366f135' },
    xai: { bg: '#ffffff18', fg: '#ffffff', border: '#ffffff35' },
    cohere: { bg: '#39594d18', fg: '#39594d', border: '#39594d35' },
    ai21: { bg: '#f43f5e18', fg: '#f43f5e', border: '#f43f5e35' },
    reka: { bg: '#a855f718', fg: '#a855f7', border: '#a855f735' },
    perplexity: { bg: '#22c55e18', fg: '#22c55e', border: '#22c55e35' },
    commandcode: { bg: '#ef444418', fg: '#ef4444', border: '#ef444435' },
  };

  function providerStyle(id: string) {
    const c = providerColors[id] || { bg: `var(--color-primary-light)`, fg: `var(--color-primary)`, border: `var(--color-border)` };
    return `background: ${c.bg}; color: ${c.fg}; border: 1px solid ${c.border};`;
  }
</script>

<div style="animation: fadeInUp 0.4s ease-out;">
  <!-- Header Title -->
  <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
    <div>
      <h1 style="font-size: 20px; font-weight: 700; color: var(--color-fg-0); margin: 0 0 4px 0;">Providers Hub</h1>
      <p style="font-size: 13px; color: var(--color-fg-3); margin: 0;">
        Unified AI provider directory. View connected accounts, check live health, and connect new credentials.
      </p>
    </div>
    <div class="flex items-center gap-2">
      <a href="/dashboard/connections" class="btn-secondary flex items-center gap-1.5" style="font-size: 12px; padding: 7px 12px; text-decoration: none;">
        <Link2 size={14} />
        <span>Manage Connections</span>
      </a>
      <a href="/dashboard/oauth-ide" class="btn-secondary flex items-center gap-1.5" style="font-size: 12px; padding: 7px 12px; text-decoration: none;">
        <ShieldCheck size={14} />
        <span>OAuth Providers</span>
      </a>
    </div>
  </div>

  <!-- Summary Stats Bar ala 9router -->
  <div class="card mb-6" style="padding: 0; overflow: hidden;">
    <div class="grid grid-cols-2 md:grid-cols-4" style="gap: 1px; background: var(--color-border);">
      <div class="stat-box">
        <Server size={18} style="color: var(--color-primary);" />
        <div>
          <div class="stat-label">TOTAL PROVIDERS</div>
          <div class="stat-val">{statsMeta.totalProviders}</div>
        </div>
      </div>
      <div class="stat-box">
        <CheckCircle2 size={18} style="color: var(--color-success);" />
        <div>
          <div class="stat-label">CONNECTED PROVIDERS</div>
          <div class="stat-val" style="color: var(--color-success);">{statsMeta.connectedProviders}</div>
        </div>
      </div>
      <div class="stat-box">
        <Sliders size={18} style="color: var(--color-primary);" />
        <div>
          <div class="stat-label">ACTIVE ACCOUNTS</div>
          <div class="stat-val">{statsMeta.totalActiveAccounts}</div>
        </div>
      </div>
      <div class="stat-box">
        <Cpu size={18} style="color: var(--color-primary);" />
        <div>
          <div class="stat-label">CATALOG MODELS</div>
          <div class="stat-val">{statsMeta.totalModels}</div>
        </div>
      </div>
    </div>
  </div>

  <!-- Search & Filter Controls -->
  <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4 mb-6">
    <!-- Filter Tabs -->
    <div class="flex items-center gap-1 p-1" style="background: var(--color-bg-card); border: 1px solid var(--color-border); border-radius: 8px; width: fit-content;">
      <button
        type="button"
        class="filter-pill {statusFilter === 'all' ? 'active' : ''}"
        onclick={() => statusFilter = 'all'}
      >
        All ({providers.length})
      </button>
      <button
        type="button"
        class="filter-pill {statusFilter === 'connected' ? 'active' : ''}"
        onclick={() => statusFilter = 'connected'}
      >
        Connected ({statsMeta.connectedProviders})
      </button>
      <button
        type="button"
        class="filter-pill {statusFilter === 'not_configured' ? 'active' : ''}"
        onclick={() => statusFilter = 'not_configured'}
      >
        Not Configured ({providers.length - statsMeta.connectedProviders})
      </button>
      <button
        type="button"
        class="filter-pill {statusFilter === 'oauth' ? 'active' : ''}"
        onclick={() => statusFilter = 'oauth'}
      >
        OAuth ({providers.filter(p => p.isOAuth).length})
      </button>
    </div>

    <!-- Search Input -->
    <div class="relative" style="width: 100%; max-width: 320px;">
      <Search size={15} class="absolute" style="left: 12px; top: 50%; transform: translateY(-50%); color: var(--color-fg-3); pointer-events: none;" />
      <input
        class="input-field"
        placeholder="Search providers, models, accounts..."
        bind:value={search}
        style="padding-left: 36px; font-size: 13px;"
      />
      {#if search}
        <button
          type="button"
          class="absolute"
          style="right: 10px; top: 50%; transform: translateY(-50%); color: var(--color-fg-3); font-size: 16px; line-height: 1; padding: 2px 6px; border-radius: 4px; opacity: 0.6;"
          onclick={() => search = ''}
          aria-label="Clear search"
        >×</button>
      {/if}
    </div>
  </div>

  {#if loading}
    <Spinner />
  {:else if error}
    <div class="card text-center" style="color: var(--color-error); padding: 40px;">
      <AlertCircle size={32} style="margin: 0 auto 12px; opacity: 0.7;" />
      Failed to load providers: {error}
    </div>
  {:else if filtered.length === 0}
    <EmptyState
      icon={Server}
      title="No providers found"
      description={search ? `No providers or models matching "${search}"` : 'No providers match the selected filter'}
    />
  {:else}
    <!-- Providers Grid ala 9router -->
    <div class="grid grid-cols-1 lg:grid-cols-2" style="gap: 16px;">
      {#each filtered as provider (provider.id)}
        {@const isModelsOpen = expanded.has(provider.id)}
        {@const isAccountsOpen = expandedAccounts.has(provider.id)}
        {@const hasConnected = provider.activeCount > 0}
        {@const hasAccounts = provider.accounts.length > 0}

        <div class="card provider-card" style="padding: 0; overflow: hidden; display: flex; flex-direction: column;">
          <!-- Card Header -->
          <div style="padding: 16px 20px; border-bottom: 1px solid var(--color-border-light);">
            <div class="flex items-center justify-between gap-3">
              <div class="flex items-center gap-3">
                <div
                  class="flex items-center justify-center font-bold"
                  style="{providerStyle(provider.id)} width: 42px; height: 42px; border-radius: 12px; font-size: 14px; flex-shrink: 0;"
                >
                  {provider.name.charAt(0)}
                </div>
                <div>
                  <div class="flex items-center gap-2 mb-1 flex-wrap">
                    <span style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">{provider.name}</span>
                    <span class="badge" style="font-size: 10px; padding: 2px 6px; background: var(--color-border-light); color: var(--color-fg-2); text-transform: uppercase;">
                      {provider.format}
                    </span>
                    {#if provider.isOAuth}
                      <span class="badge" style="font-size: 10px; padding: 2px 6px; background: rgba(139, 92, 246, 0.12); color: #8b5cf6; border: 1px solid rgba(139, 92, 246, 0.25);">
                        OAuth
                      </span>
                    {/if}
                  </div>
                  <div style="font-size: 11px; color: var(--color-fg-3); font-family: var(--font-mono);">
                    {provider.base_url.replace('https://', '')}
                  </div>
                </div>
              </div>

              <!-- Status Badge (ala 9router getStatusDisplay) -->
              <div class="flex items-center gap-2">
                {#if hasConnected}
                  <span class="badge badge-success flex items-center gap-1.5" style="font-size: 11px; font-weight: 600; padding: 4px 10px;">
                    <span class="status-dot-active"></span>
                    <span>{provider.activeCount} Connected</span>
                  </span>
                {:else if hasAccounts}
                  <span class="badge badge-warning flex items-center gap-1.5" style="font-size: 11px; font-weight: 600; padding: 4px 10px;">
                    <span class="status-dot-disabled"></span>
                    <span>{provider.accounts.length} Disabled</span>
                  </span>
                {:else}
                  <span class="badge badge-muted flex items-center gap-1" style="font-size: 11px; padding: 4px 8px;">
                    <span>No connections</span>
                  </span>
                {/if}
              </div>
            </div>

            {#if provider.description}
              <div style="margin-top: 10px; font-size: 12px; color: var(--color-fg-2); line-height: 1.4;">
                {provider.description}
              </div>
            {/if}
          </div>

          <!-- Card Body: Connected Accounts Preview -->
          <div style="padding: 14px 20px; flex: 1; background: var(--color-bg-1);">
            <div class="flex items-center justify-between mb-2">
              <span style="font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.6px; color: var(--color-fg-3);">
                Connected Accounts ({provider.accounts.length})
              </span>
              {#if provider.accounts.length > 2}
                <button
                  type="button"
                  class="btn-text"
                  style="font-size: 11px; color: var(--color-primary); cursor: pointer; padding: 0; background: none; border: none;"
                  onclick={() => toggleExpandAccounts(provider.id)}
                >
                  {isAccountsOpen ? 'Show less' : `Show all (${provider.accounts.length})`}
                </button>
              {/if}
            </div>

            {#if provider.accounts.length === 0}
              <div class="empty-account-box flex items-center justify-between">
                <span style="font-size: 12px; color: var(--color-fg-3);">No active credentials configured yet.</span>
                <button
                  type="button"
                  class="btn-secondary flex items-center gap-1"
                  style="font-size: 11px; padding: 4px 8px;"
                  onclick={() => openAddModal(provider)}
                >
                  <Plus size={12} />
                  <span>Connect</span>
                </button>
              </div>
            {:else}
              {@const visibleAccounts = isAccountsOpen ? provider.accounts : provider.accounts.slice(0, 2)}
              <div class="flex flex-col gap-1.5">
                {#each visibleAccounts as acc (acc.id)}
                  <div class="account-item-row flex items-center justify-between">
                    <div class="flex items-center gap-2 min-w-0">
                      <span class={acc.is_active === 1 ? 'dot-active' : 'dot-disabled'}></span>
                      <span style="font-size: 12px; font-weight: 500; color: var(--color-fg-1); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 260px;" title={acc.name}>
                        {acc.name}
                      </span>
                    </div>
                    <div class="flex items-center gap-2 flex-shrink-0">
                      <span style="font-size: 10px; font-family: var(--font-mono); color: var(--color-fg-3); background: var(--color-border-light); padding: 1px 5px; border-radius: 4px;">
                        P:{acc.priority}
                      </span>
                      <a href="/dashboard/connections?search={encodeURIComponent(acc.name)}" class="account-link" title="Manage in Connections">
                        <Sliders size={12} />
                      </a>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>

          <!-- Card Actions Toolbar -->
          <div class="card-footer-toolbar flex items-center justify-between" style="padding: 10px 20px; border-top: 1px solid var(--color-border-light); background: var(--color-bg-card);">
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="btn-primary flex items-center gap-1.5"
                style="font-size: 11px; padding: 6px 11px;"
                onclick={() => openAddModal(provider)}
              >
                <Plus size={13} />
                <span>Add Account</span>
              </button>
              <a
                href="/dashboard/connections"
                class="btn-secondary flex items-center gap-1"
                style="font-size: 11px; padding: 6px 10px; text-decoration: none;"
              >
                <span>Manage</span>
                <ExternalLink size={11} />
              </a>
            </div>

            <button
              type="button"
              class="btn-secondary flex items-center gap-1.5"
              style="font-size: 11px; padding: 6px 11px;"
              onclick={() => toggleExpandModels(provider.id)}
            >
              <span>{provider.models.length} Models</span>
              {#if isModelsOpen}
                <ChevronDown size={14} />
              {:else}
                <ChevronRight size={14} />
              {/if}
            </button>
          </div>

          <!-- Models List (Collapsible) -->
          {#if isModelsOpen}
            <div style="border-top: 1px solid var(--color-border); background: var(--color-bg-card); animation: fadeInScale 0.2s ease-out;">
              <div class="overflow-x-auto">
                <table style="width: 100%; border-collapse: collapse; font-size: 12px;">
                  <thead>
                    <tr style="border-bottom: 1px solid var(--color-border); background: var(--color-bg-1);">
                      <th style="text-align: left; padding: 8px 16px; font-weight: 500; color: var(--color-fg-3); font-size: 10px; text-transform: uppercase;">Model</th>
                      <th style="text-align: center; padding: 8px 10px; font-weight: 500; color: var(--color-fg-3); font-size: 10px; text-transform: uppercase;">Context</th>
                      <th style="text-align: right; padding: 8px 10px; font-weight: 500; color: var(--color-fg-3); font-size: 10px; text-transform: uppercase;">Input /1M</th>
                      <th style="text-align: right; padding: 8px 10px; font-weight: 500; color: var(--color-fg-3); font-size: 10px; text-transform: uppercase;">Output /1M</th>
                      <th style="text-align: center; padding: 8px 16px; font-weight: 500; color: var(--color-fg-3); font-size: 10px; text-transform: uppercase;">Features</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each provider.models as model, i}
                      <tr
                        style="border-bottom: {i < provider.models.length - 1 ? '1px solid var(--color-border-light)' : 'none'};"
                        class="model-table-row"
                      >
                        <td style="padding: 8px 16px;">
                          <div style="font-weight: 600; color: var(--color-fg-0); font-size: 11px; font-family: var(--font-mono);">{model.name}</div>
                          <div style="color: var(--color-fg-3); font-size: 10px; font-family: var(--font-mono);">{model.id}</div>
                        </td>
                        <td style="text-align: center; padding: 8px 10px;">
                          <span style="color: var(--color-fg-1); font-family: var(--font-mono); font-size: 11px;">{fmtTokens(model.context_window)}</span>
                        </td>
                        <td style="text-align: right; padding: 8px 10px;">
                          <span style="color: var(--color-fg-1); font-family: var(--font-mono); font-size: 11px;">{fmtPrice(model.input_price_per_1m)}</span>
                        </td>
                        <td style="text-align: right; padding: 8px 10px;">
                          <span style="color: var(--color-fg-1); font-family: var(--font-mono); font-size: 11px;">{fmtPrice(model.output_price_per_1m)}</span>
                        </td>
                        <td style="padding: 8px 16px;">
                          <div class="flex justify-center gap-1">
                            {#each model.capabilities as cap}
                              <span
                                class="flex items-center gap-0.5"
                                style="padding: 1px 5px; border-radius: 4px; font-size: 9px; font-weight: 500; background: {capColor(cap)}15; color: {capColor(cap)};"
                                title={cap}
                              >
                                {#if cap === 'chat'}<Brain size={9} />{:else if cap === 'vision'}<Eye size={9} />{:else if cap === 'tools'}<Wrench size={9} />{:else if cap === 'streaming'}<Activity size={9} />{:else if cap === 'json_mode'}<FileJson size={9} />{:else}<Zap size={9} />{/if}
                                <span>{cap.replace('_', ' ')}</span>
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

  <!-- Quick Add Account Modal -->
  {#if showAddModal && modalProvider}
    <div
      class="modal-backdrop"
      onclick={(e) => { if (e.target === e.currentTarget) closeAddModal(); }}
      onkeydown={(e) => { if (e.key === 'Escape') closeAddModal(); }}
      tabindex="-1"
      role="dialog"
      aria-modal="true"
    >
      <div class="card modal-content" style="max-width: 480px; width: 92%; padding: 24px; position: relative;">
        <button
          type="button"
          class="modal-close-btn"
          onclick={closeAddModal}
          aria-label="Close dialog"
        >
          <X size={16} />
        </button>

        <div class="flex items-center gap-3 mb-4">
          <div class="flex items-center justify-center font-bold" style="{providerStyle(modalProvider.id)} width: 36px; height: 36px; border-radius: 10px;">
            {modalProvider.name.charAt(0)}
          </div>
          <div>
            <h3 style="font-size: 16px; font-weight: 700; color: var(--color-fg-0); margin: 0;">
              Connect {modalProvider.name}
            </h3>
            <p style="font-size: 12px; color: var(--color-fg-3); margin: 2px 0 0 0;">
              Add credentials to route traffic through {modalProvider.name}.
            </p>
          </div>
        </div>

        {#if modalProvider.isOAuth}
          <div class="oauth-quick-connect-box mb-4">
            <div class="flex items-center gap-2 mb-2">
              <ShieldCheck size={18} style="color: #8b5cf6;" />
              <span style="font-size: 13px; font-weight: 600; color: var(--color-fg-0);">OAuth Sign-in Recommended</span>
            </div>
            <p style="font-size: 12px; color: var(--color-fg-2); line-height: 1.4; margin: 0 0 14px 0;">
              {modalProvider.name} supports secure one-click OAuth login without manually generating or pasting long-lived API keys.
            </p>
            <a
              href="/dashboard/oauth-ide"
              class="btn-primary flex items-center justify-center gap-2 w-full"
              style="padding: 9px; font-size: 13px; text-decoration: none;"
            >
              <ShieldCheck size={15} />
              <span>Open OAuth Providers Flow</span>
            </a>
          </div>

          <div class="text-center mb-3">
            <span style="font-size: 11px; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">Or Add Custom Endpoint via API Key</span>
          </div>
        {/if}

        <form onsubmit={(e) => { e.preventDefault(); handleSaveConnection(); }} class="flex flex-col gap-3">
          <div>
            <label for="acc-name" style="display: block; font-size: 11px; font-weight: 600; color: var(--color-fg-2); margin-bottom: 4px;">
              Connection Name
            </label>
            <input
              id="acc-name"
              class="input-field"
              bind:value={addFormName}
              placeholder="e.g. Primary Account"
              required
            />
          </div>

          <div>
            <label for="acc-key" style="display: block; font-size: 11px; font-weight: 600; color: var(--color-fg-2); margin-bottom: 4px;">
              API Key {modalProvider.isOAuth ? '(Optional if using OAuth)' : ''}
            </label>
            <input
              id="acc-key"
              type="password"
              class="input-field font-mono"
              bind:value={addFormApiKey}
              placeholder="sk-..."
            />
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div>
              <label for="acc-format" style="display: block; font-size: 11px; font-weight: 600; color: var(--color-fg-2); margin-bottom: 4px;">
                Format
              </label>
              <input
                id="acc-format"
                class="input-field"
                bind:value={addFormFormat}
              />
            </div>
            <div>
              <label for="acc-pri" style="display: block; font-size: 11px; font-weight: 600; color: var(--color-fg-2); margin-bottom: 4px;">
                Priority (Higher = first)
              </label>
              <input
                id="acc-pri"
                type="number"
                class="input-field"
                bind:value={addFormPriority}
                min="0"
                max="100"
              />
            </div>
          </div>

          <div>
            <label for="acc-url" style="display: block; font-size: 11px; font-weight: 600; color: var(--color-fg-2); margin-bottom: 4px;">
              Base URL
            </label>
            <input
              id="acc-url"
              class="input-field font-mono"
              bind:value={addFormBaseUrl}
              required
            />
          </div>

          <div class="flex items-center justify-end gap-2 mt-3 pt-3" style="border-top: 1px solid var(--color-border-light);">
            <button
              type="button"
              class="btn-secondary"
              style="padding: 8px 14px; font-size: 12px;"
              onclick={closeAddModal}
            >
              Cancel
            </button>
            <button
              type="submit"
              class="btn-primary"
              style="padding: 8px 16px; font-size: 12px;"
              disabled={isSavingConnection}
            >
              {isSavingConnection ? 'Saving...' : 'Save Connection'}
            </button>
          </div>
        </form>
      </div>
    </div>
  {/if}
</div>

<style>
  .stat-box {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 16px 20px;
    background: var(--color-bg-card);
  }

  .stat-label {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.8px;
    color: var(--color-fg-3);
    margin-bottom: 2px;
  }

  .stat-val {
    font-size: 22px;
    font-weight: 700;
    color: var(--color-fg-0);
    font-family: var(--font-mono);
    line-height: 1.1;
  }

  .filter-pill {
    padding: 6px 12px;
    font-size: 12px;
    font-weight: 500;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: var(--color-fg-2);
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }

  .filter-pill:hover {
    color: var(--color-fg-0);
    background: var(--color-bg-1);
  }

  .filter-pill.active {
    background: var(--color-primary);
    color: #ffffff;
    font-weight: 600;
  }

  .provider-card {
    transition: box-shadow 0.2s, border-color 0.2s;
  }

  .provider-card:hover {
    box-shadow: var(--shadow-md);
  }

  .badge-success {
    background: rgba(16, 185, 129, 0.12);
    color: var(--color-success);
    border: 1px solid rgba(16, 185, 129, 0.25);
  }

  .badge-warning {
    background: rgba(245, 158, 11, 0.12);
    color: var(--color-warning);
    border: 1px solid rgba(245, 158, 11, 0.25);
  }

  .badge-muted {
    background: var(--color-border-light);
    color: var(--color-fg-3);
  }

  .status-dot-active {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--color-success);
    box-shadow: 0 0 6px rgba(16, 185, 129, 0.5);
  }

  .status-dot-disabled {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--color-warning);
  }

  .empty-account-box {
    padding: 10px 14px;
    background: var(--color-bg-card);
    border: 1px dashed var(--color-border);
    border-radius: 8px;
  }

  .account-item-row {
    padding: 7px 10px;
    background: var(--color-bg-card);
    border: 1px solid var(--color-border-light);
    border-radius: 6px;
  }

  .dot-active {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--color-success);
    flex-shrink: 0;
  }

  .dot-disabled {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--color-fg-3);
    flex-shrink: 0;
  }

  .account-link {
    color: var(--color-fg-3);
    padding: 3px;
    border-radius: 4px;
    display: inline-flex;
    align-items: center;
    transition: color 0.15s, background 0.15s;
  }

  .account-link:hover {
    color: var(--color-primary);
    background: var(--color-border-light);
  }

  .model-table-row:hover {
    background: var(--color-bg-1);
  }

  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(2px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1200;
  }

  .modal-content {
    box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
    animation: fadeInScale 0.2s ease-out;
  }

  .modal-close-btn {
    position: absolute;
    top: 16px;
    right: 16px;
    background: none;
    border: none;
    color: var(--color-fg-3);
    cursor: pointer;
    padding: 4px;
    border-radius: 6px;
    display: flex;
    align-items: center;
  }

  .modal-close-btn:hover {
    color: var(--color-fg-0);
    background: var(--color-border-light);
  }

  .oauth-quick-connect-box {
    padding: 14px;
    background: rgba(139, 92, 246, 0.08);
    border: 1px solid rgba(139, 92, 246, 0.2);
    border-radius: 8px;
  }

  :global(.input-field) {
    width: 100%;
    padding: 9px 12px;
    border-radius: 7px;
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
