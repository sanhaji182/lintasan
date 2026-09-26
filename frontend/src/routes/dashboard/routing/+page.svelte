<script lang="ts">
  import TabNav from '$lib/components/TabNav.svelte';
  const __tabs = [
    { label: 'Routing', path: '/dashboard/routing' },
    { label: 'Fallback', path: '/dashboard/fallback' }
  ];
  import { onMount } from 'svelte';
  import { beforeNavigate } from '$app/navigation';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { showToast } from '$lib/toast';
  import { routingDirtyState, buildPolicyPayload, buildQuotaPayload } from '$lib/workflow-consolidation';
  import { comboProviderOptions, comboModelsForConnection, buildPinnedComboEntries } from '$lib/combo-entry-selector';
  import {
    GitBranch, GripVertical, Plus, Trash2, Save,
    Server, Tag, Shuffle, RotateCw, CircleDot,
    BrainCircuit, DollarSign, Gauge, Layers, ToggleLeft, ToggleRight, X, Sparkles
  } from 'lucide-svelte/icons';

  interface Combo {
    id: string;
    provider: string;
    strategy: string;
    keys: string[];
    models?: string[];
    description?: string;
    order: number;
    entries?: Array<{ model: string; connection_id?: string; connection_ids?: string[] }>;
  }

  interface Alias {
    id: string;
    alias: string;
    target: string;
  }
  const BUILT_IN_ALIAS_IDS = new Set(['auto', 'auto/coding', 'auto/fast', 'auto/cheap']);

  function isBuiltInAlias(id: string) {
    return BUILT_IN_ALIAS_IDS.has(id);
  }

  let combos = $state<Combo[]>([]);
  let aliases = $state<Alias[]>([]);
  let loadBalancerStrategy = $state<string>('priority');
  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let advertisedModels = $state<any[]>([]);
  let draggedIndex = $state<number | null>(null);
  let dragOverIndex = $state<number | null>(null);
  let orderDirty = $state(false);

  // New alias form
  let showAliasForm = $state(false);
  let newAlias = $state('');
  let newTarget = $state('');
  let aliasFeedback = $state('');
  let aliasFeedbackKind = $state<'success' | 'error'>('success');

  // New combo form
  let showComboForm = $state(false);
  let newComboName = $state('');
  let newComboStrategy = $state('priority');
  let newComboDescription = $state('');
  let comboConnections = $state<any[]>([]);
  let comboDiscoveredModels = $state<any[]>([]);
  let comboCatalogLoading = $state(false);
  let comboCatalogError = $state('');
  let selectedComboConnection = $state('');
  let selectedComboModel = $state('');
  let newComboEntries = $state<Array<{ model: string; connectionId: string }>>([]);
  let showAdvancedModelInput = $state(false);
  let advancedModel = $state('');
  let syncingConnection = $state('');
  let comboCreating = $state(false);
  const comboProviders = $derived(comboProviderOptions(comboConnections));
  const selectedProvider = $derived(comboProviders.find(option => option.id === selectedComboConnection));
  const selectedConnectionModels = $derived(comboModelsForConnection(comboDiscoveredModels, selectedComboConnection));

  // Smart Routing Intelligence config (ML routing, cost, quota)
  interface SmartConfig {
    ml_router_enabled: boolean;
    ml_router_cheap_model: string;
    ml_router_expensive_model: string;
    ml_router_threshold: string;
    cost_quality_floor: string;
    cost_expensive_anchor: string;
    quota_limits: Record<string, { max_tokens_per_day?: number }>;
  }
  let smart = $state<SmartConfig>({
    ml_router_enabled: false,
    ml_router_cheap_model: 'gpt-4o-mini',
    ml_router_expensive_model: 'gpt-4o',
    ml_router_threshold: '0.5',
    cost_quality_floor: '0.3',
    cost_expensive_anchor: '0.02',
    quota_limits: {}
  });
  let savingSmart = $state(false);
  // Quota limit editor rows derived from the quota_limits map
  let quotaRows = $state<Array<{ connId: string; maxPerDay: string }>>([]);

  // ── Explicit sections + save scopes ────────────────────────────────────
  // The page is split into three independently-savable scopes. Policy, combo,
  // and quota mutations are staged locally; aliases are explicitly immediate.
  type Section = 'policies' | 'combos' | 'quotas';
  let section = $state<Section>('policies');
  const SECTIONS: { key: Section; label: string; help: string }[] = [
    { key: 'policies', label: 'Policies', help: 'Global load-balancer strategy, ML routing and cost ranking.' },
    { key: 'combos', label: 'Combos', help: 'Ordered failover chains and per-combo strategy.' },
    { key: 'quotas', label: 'Quotas', help: 'Per-connection daily token ceilings.' },
  ];
  let savedPolicy = $state('');
  let savedQuotas = $state('');
  // Per-combo strategy edits are staged until the Combos scope is saved, so the
  // page never PATCHes a single combo behind the operator's back.
  let stagedStrategies = $state<Record<string, string>>({});
  let baseStrategies = $state<Record<string, string>>({});
  let stagedLbStrategy = $state<string>('priority');

  const policyFingerprint = $derived(JSON.stringify([buildPolicyPayload(smart), stagedLbStrategy]));
  const quotasFingerprint = $derived(JSON.stringify(buildQuotaPayload(quotaRows)));
  const combosDirtyCount = $derived(Object.keys(stagedStrategies).length);
  const dirty = $derived(routingDirtyState({
    policy: savedPolicy !== '' && policyFingerprint !== savedPolicy,
    combos: combosDirtyCount > 0 || orderDirty,
    quotas: savedQuotas !== '' && quotasFingerprint !== savedQuotas,
  }));

  const unsavedWarning = 'You have unsaved routing changes. Leave this page and discard them?';

  beforeNavigate((navigation) => {
    // Native unload confirmation is handled by beforeunload below. Prompt here
    // only for client-side navigation so users never see two dialogs.
    if (!navigation.willUnload && dirty.count > 0 && !window.confirm(unsavedWarning)) navigation.cancel();
  });

  onMount(() => {
    const protectRefresh = (event: BeforeUnloadEvent) => {
      if (dirty.count === 0) return;
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', protectRefresh);
    return () => window.removeEventListener('beforeunload', protectRefresh);
  });

  function discardPolicies() {
    if (!savedPolicy) return;
    const [policy, strategy] = JSON.parse(savedPolicy) as [Omit<SmartConfig, 'quota_limits'>, string];
    // Restore only the Policies scope. Quotas share the `smart` object in the UI,
    // but are independently staged and must survive a Policies discard.
    smart = { ...smart, ...policy };
    stagedLbStrategy = strategy;
  }

  function discardCombos() {
    if (orderDirty) { loadCombos(); orderDirty = false; return; }
    for (const id of Object.keys(stagedStrategies)) {
      const combo = combos.find(c => c.id === id);
      if (combo) combo.strategy = baseStrategies[id] ?? combo.strategy;
    }
    stagedStrategies = {};
  }

  function discardQuotas() {
    quotaRows = Object.entries(smart.quota_limits || {}).map(([connId, v]) => ({ connId, maxPerDay: String((v as any)?.max_tokens_per_day ?? '') }));
    savedQuotas = quotasFingerprint;
  }

  async function loadSmart() {
    try {
      const res = await api.get<{ data: SmartConfig }>('/api/smart-routing');
      if (res?.data) {
        smart = { ...smart, ...res.data, quota_limits: res.data.quota_limits || {} };
        quotaRows = Object.entries(smart.quota_limits || {}).map(([connId, v]) => ({
          connId,
          maxPerDay: String((v as any)?.max_tokens_per_day ?? '')
        }));
      }
    } catch {
      // leave defaults
    }
  }

  async function saveSmart(scope: 'policies' | 'quotas' = section === 'quotas' ? 'quotas' : 'policies') {
    savingSmart = true;
    try {
      if (scope === 'quotas') {
        const payload = buildQuotaPayload(quotaRows);
        await api.post('/api/smart-routing', payload);
        smart.quota_limits = payload.quota_limits;
        savedQuotas = JSON.stringify(payload);
        showToast('Quota scope saved (applied live, no restart)', 'success');
      } else {
        await api.post('/api/smart-routing', buildPolicyPayload(smart));
        savedPolicy = policyFingerprint;
        showToast('Policy intelligence saved (applied live, no restart)', 'success');
      }
    } catch (e: any) {
      showToast(e.message || `Failed to save ${scope} config`, 'error');
      throw e;
    } finally { savingSmart = false; }
  }

  function addQuotaRow() {
    quotaRows = [...quotaRows, { connId: '', maxPerDay: '' }];
  }
  function removeQuotaRow(i: number) {
    quotaRows = quotaRows.filter((_, idx) => idx !== i);
  }

  const strategies = [
    { value: 'priority', label: 'Priority / Fallback', icon: Layers },
    { value: 'auto', label: '🤖 Auto (Smart)', icon: Shuffle },
    { value: 'round-robin', label: 'Round Robin', icon: RotateCw },
    { value: 'least-latency', label: 'Least Latency', icon: CircleDot },
    { value: 'random', label: 'Random', icon: Shuffle },
  ];

  async function loadCombos() {
    try {
      const res = await api.get<any>('/api/combos');
      const raw = res?.data || res?.combos || [];
      combos = Array.isArray(raw) ? raw.map((c: any, i: number) => ({
        id: c.id || c.name || `combo-${i}`,
        provider: c.name || c.provider || 'Unknown',
        strategy: c.strategy || 'priority',
        keys: Array.isArray(c.keys) ? c.keys : [],
        models: Array.isArray(c.models) && c.models.length > 0 ? c.models : (Array.isArray(c.entries) ? c.entries.map((e: any) => e.model) : []),
        description: c.description || '',
        order: c.order ?? i,
        entries: Array.isArray(c.entries) ? c.entries : [],
      })) : [];
      baseStrategies = Object.fromEntries(combos.map(combo => [combo.id, combo.strategy]));
      stagedStrategies = {};
    } catch {
      combos = [];
    }
  }

  async function loadComboCatalog() {
    comboCatalogLoading = true;
    comboCatalogError = '';
    try {
      const [connectionsResponse, modelsResponse] = await Promise.all([
        api.get<any>('/api/connections'),
        api.get<any>('/api/models/discovered'),
      ]);
      comboConnections = Array.isArray(connectionsResponse?.data) ? connectionsResponse.data : [];
      comboDiscoveredModels = Array.isArray(modelsResponse?.data) ? modelsResponse.data : [];
    } catch (e: any) {
      comboConnections = [];
      comboDiscoveredModels = [];
      comboCatalogError = e.message || 'Provider catalog could not be loaded.';
    } finally {
      comboCatalogLoading = false;
    }
  }

  function addPinnedComboEntry() {
    if (!selectedComboConnection || !selectedComboModel) return;
    newComboEntries = [...newComboEntries, { model: selectedComboModel, connectionId: selectedComboConnection }];
    selectedComboModel = '';
  }

  function addAdvancedComboEntry() {
    const model = advancedModel.trim();
    if (!model || !selectedComboConnection) return;
    newComboEntries = [...newComboEntries, { model, connectionId: selectedComboConnection }];
    advancedModel = '';
  }

  function removeComboEntry(index: number) {
    newComboEntries = newComboEntries.filter((_, entryIndex) => entryIndex !== index);
  }

  async function syncComboModels() {
    if (!selectedComboConnection) return;
    syncingConnection = selectedComboConnection;
    comboCatalogError = '';
    try {
      await api.post(`/api/models/sync/${encodeURIComponent(selectedComboConnection)}`, {});
      const modelsResponse = await api.get<any>('/api/models/discovered');
      comboDiscoveredModels = Array.isArray(modelsResponse?.data) ? modelsResponse.data : [];
      showToast(`Models synced for ${selectedProvider?.name || selectedComboConnection}`, 'success');
    } catch (e: any) {
      comboCatalogError = e.message || 'Model sync failed.';
      showToast(comboCatalogError, 'error');
    } finally {
      syncingConnection = '';
    }
  }

  async function createCombo() {
    const name = newComboName.trim();
    if (!name) {
      showToast('Combo name is required', 'error');
      return;
    }
    const entries = buildPinnedComboEntries(newComboEntries);

    if (entries.length === 0) {
      showToast('At least one pinned provider and model entry is required', 'error');
      return;
    }

    comboCreating = true;
    try {
      const payload = {
        name,
        strategy: newComboStrategy,
        description: newComboDescription.trim(),
        models: entries.map(entry => entry.model),
        entries,
      };
      await api.post('/api/combos', payload);
      showToast(`Combo "${name}" created successfully`, 'success');
      showComboForm = false;
      newComboName = '';
      newComboDescription = '';
      newComboEntries = [];
      selectedComboConnection = '';
      selectedComboModel = '';
      advancedModel = '';
      showAdvancedModelInput = false;
      await loadCombos();
    } catch (e: any) {
      showToast(e.message || 'Failed to create combo', 'error');
    } finally {
      comboCreating = false;
    }
  }

  async function deleteCombo(id: string, name: string) {
    if (!confirm(`Delete combo "${name}"?`)) return;
    try {
      await api.delete(`/api/combos?id=${encodeURIComponent(id)}`);
      showToast(`Combo "${name}" deleted`, 'success');
      await loadCombos();
    } catch (e: any) {
      showToast(e.message || 'Failed to delete combo', 'error');
    }
  }

  async function loadAliases() {
    try {
      const res = await api.get<{ data: Record<string, { model?: string } | string> }>('/api/aliases');
      const raw = res.data || {};
      aliases = Object.entries(raw).map(([name, cfg]) => ({
        id: name,
        alias: name,
        target: typeof cfg === 'string' ? cfg : (cfg.model || JSON.stringify(cfg))
      }));
    } catch {
      aliases = [];
    }
    // Always include built-in auto aliases
    const autoAliases = [
      { id: 'auto', alias: 'auto', target: 'Smart routing — picks best available provider' },
      { id: 'auto/coding', alias: 'auto/coding', target: 'Optimized for code generation (high success rate)' },
      { id: 'auto/fast', alias: 'auto/fast', target: 'Optimized for speed (lowest latency)' },
      { id: 'auto/cheap', alias: 'auto/cheap', target: 'Optimized for cost (cheapest provider)' },
    ];
    const existingIds = new Set(aliases.map(a => a.id));
    for (const aa of autoAliases) {
      if (!existingIds.has(aa.id)) {
        aliases = [...aliases, aa];
      }
    }
  }

  async function loadLoadBalancer() {
    try {
      const res = await api.get<{ data: { strategy: string } }>('/api/load-balancer');
      loadBalancerStrategy = res.data?.strategy || 'priority';
      stagedLbStrategy = loadBalancerStrategy;
    } catch {
      loadBalancerStrategy = 'priority'; stagedLbStrategy = 'priority';
    }
  }

  async function loadAdvertisedModels() {
    try {
      const response = await api.get<any>('/v1/models');
      advertisedModels = Array.isArray(response?.data) ? response.data : [];
    } catch {
      advertisedModels = [];
    }
  }

  onMount(async () => {
    loading = true;
    await Promise.all([loadCombos(), loadAliases(), loadLoadBalancer(), loadSmart(), loadAdvertisedModels()]);
    savedPolicy = policyFingerprint;
    savedQuotas = quotasFingerprint;
    loading = false;
    void loadComboCatalog();
  });

  function updateStrategy(comboId: string, strategy: string) {
    const combo = combos.find(c => c.id === comboId);
    if (combo) combo.strategy = strategy;
    stagedStrategies = { ...stagedStrategies, [comboId]: strategy };
  }

  async function saveComboStrategies() {
    for (const [comboId, strategy] of Object.entries(stagedStrategies)) {
      await api.patch(`/api/routing/combos/${comboId}`, { strategy });
    }
    baseStrategies = { ...baseStrategies, ...stagedStrategies };
    stagedStrategies = {};
  }

  async function saveCombos() {
    saving = true;
    try {
      // Fail closed: reorder is never attempted after any strategy PATCH fails.
      await saveComboStrategies();
      if (orderDirty) await saveOrder();
      showToast('Combo scope saved', 'success');
    } catch (e: any) {
      error = e.message || 'Failed to save combo scope';
      showToast('Combo save stopped; unsaved changes remain', 'error');
    } finally { saving = false; }
  }

  async function savePolicy() {
    savingSmart = true;
    const previousStrategy = loadBalancerStrategy;
    try {
      await api.post('/api/load-balancer', { strategy: stagedLbStrategy });
      loadBalancerStrategy = stagedLbStrategy;
      await saveSmart('policies');
      savedPolicy = policyFingerprint;
      showToast('Policy scope saved', 'success');
    } catch (e: any) {
      if (loadBalancerStrategy !== previousStrategy) {
        try {
          await api.post('/api/load-balancer', { strategy: previousStrategy });
          loadBalancerStrategy = previousStrategy;
        } catch (rollbackError: any) {
          error = `Policy save failed and load-balancer rollback failed: ${rollbackError.message || 'unknown error'}`;
          showToast(error, 'error');
          savingSmart = false;
          return;
        }
      }
      error = e.message || 'Failed to save policy';
      showToast(`Policy save failed; previous load-balancer strategy restored. ${error}`, 'error');
    } finally { savingSmart = false; }
  }

  async function saveOrder() {
    const ordered = combos.map((c, i) => ({ id: c.id, order: i }));
    await api.put('/api/routing/combos/reorder', { combos: ordered });
    combos = combos.map((c, i) => ({ ...c, order: i }));
    orderDirty = false;
  }

  function handleDragStart(index: number) {
    draggedIndex = index;
  }

  function handleDragOver(e: DragEvent, index: number) {
    e.preventDefault();
    dragOverIndex = index;
  }

  function handleDragEnd() {
    if (draggedIndex !== null && dragOverIndex !== null && draggedIndex !== dragOverIndex) {
      const items = [...combos];
      const [moved] = items.splice(draggedIndex, 1);
      items.splice(dragOverIndex, 0, moved);
      combos = items.map((c, i) => ({ ...c, order: i }));
      orderDirty = true;
    }
    draggedIndex = null;
    dragOverIndex = null;
  }

  async function addAlias() {
    if (!newAlias.trim() || !newTarget.trim()) return;
    const aliasName = newAlias.trim();
    error = '';
    aliasFeedback = '';
    try {
      const data = await api.post<{ alias: Alias }>('/api/routing/aliases', {
        alias: aliasName,
        target: newTarget.trim()
      });
      const savedAlias = data.alias;
      const existingIndex = aliases.findIndex(alias => alias.id === savedAlias.id);
      const wasUpdate = existingIndex !== -1;
      aliases = existingIndex === -1
        ? [...aliases, savedAlias]
        : aliases.map((alias, index) => index === existingIndex ? savedAlias : alias);
      aliasFeedback = `Alias “${savedAlias.alias}” ${wasUpdate ? 'updated' : 'created'} and applied immediately`;
      aliasFeedbackKind = 'success';
      showToast(aliasFeedback, 'success');
      newAlias = '';
      newTarget = '';
      showAliasForm = false;
    } catch (e: any) {
      const reason = e.message || 'Request failed';
      aliasFeedback = `Alias “${aliasName}” was not created: ${reason}`;
      aliasFeedbackKind = 'error';
      showToast(aliasFeedback, 'error');
    }
  }

  async function deleteAlias(id: string) {
    if (isBuiltInAlias(id)) return;
    error = '';
    aliasFeedback = '';
    try {
      await api.delete(`/api/routing/aliases/${id}`);
      aliases = aliases.filter(a => a.id !== id);
      aliasFeedback = `Alias “${id}” deleted immediately`;
      aliasFeedbackKind = 'success';
      showToast(aliasFeedback, 'success');
    } catch (e: any) {
      const reason = e.message || 'Request failed';
      aliasFeedback = `Alias “${id}” was not deleted: ${reason}`;
      aliasFeedbackKind = 'error';
      showToast(aliasFeedback, 'error');
    }
  }
</script>

<TabNav tabs={__tabs} />


<div class="routing-section-nav" aria-label="Routing configuration sections">
  {#each SECTIONS as item}
    <button class:active={section === item.key} onclick={() => section = item.key}><span>{item.label}</span><small>{item.help}</small></button>
  {/each}
</div>
{#if dirty.count > 0}
  <div class="dirty-bar" role="status"><strong>{dirty.count} unsaved {dirty.count === 1 ? 'scope' : 'scopes'}:</strong> {dirty.scopes.join(', ')}. Changes are local until you use the Save button in that section.</div>
{/if}

<div style="display: flex; flex-direction: column; gap: 24px;">
  {#if section === 'policies' || section === 'quotas'}
  <!-- Smart Routing Intelligence Section (ML routing, cost, quota) -->
  <div class="card">
    <div class="flex items-center justify-between" style="margin-bottom: 20px;">
      <div class="flex items-center gap-2.5">
        <div
          class="flex items-center justify-center rounded-xl"
          style="width: 40px; height: 40px; background: var(--color-primary-light);"
        >
          <BrainCircuit size={20} style="color: var(--color-primary);" stroke-width={1.8} />
        </div>
        <div>
          <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">Smart Routing Intelligence</div>
          <div style="font-size: 12px; color: var(--color-fg-3);">ML model selection, cost-aware ranking, and per-connection quota. Applied live, no restart.</div>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <button class="btn-secondary" onclick={section === 'quotas' ? discardQuotas : discardPolicies} disabled={savingSmart}>Discard</button>
        <button class="btn-primary flex items-center gap-1.5" onclick={() => section === 'quotas' ? saveSmart('quotas') : savePolicy()} disabled={savingSmart}>
          <Save size={14} stroke-width={2} />
          {savingSmart ? 'Saving...' : `Save ${section === 'quotas' ? 'Quotas' : 'Policies'}`}
        </button>
      </div>
    </div>

    {#if section === 'policies'}
    <!-- ML Routing -->
    <div class="smart-block">
      <div class="flex items-center justify-between" style="margin-bottom: 12px;">
        <div class="flex items-center gap-2">
          <BrainCircuit size={16} style="color: var(--color-primary);" />
          <span style="font-size: 13px; font-weight: 600; color: var(--color-fg-0);">ML Routing</span>
          <span style="font-size: 11px; color: var(--color-fg-3);">— auto cheap-vs-expensive by 15-feature complexity score</span>
        </div>
        <button
          class="flex items-center gap-1.5"
          style="background: none; border: none; cursor: pointer; color: {smart.ml_router_enabled ? 'var(--color-success)' : 'var(--color-fg-3)'};"
          onclick={() => smart.ml_router_enabled = !smart.ml_router_enabled}
        >
          {#if smart.ml_router_enabled}
            <ToggleRight size={28} stroke-width={1.6} />
          {:else}
            <ToggleLeft size={28} stroke-width={1.6} />
          {/if}
          <span style="font-size: 12px; font-weight: 600;">{smart.ml_router_enabled ? 'Enabled' : 'Disabled'}</span>
        </button>
      </div>
      <div class="smart-grid">
        <label class="smart-field">
          <span>Cheap model</span>
          <input class="input-field" bind:value={smart.ml_router_cheap_model} placeholder="gpt-4o-mini" />
        </label>
        <label class="smart-field">
          <span>Expensive model</span>
          <input class="input-field" bind:value={smart.ml_router_expensive_model} placeholder="gpt-4o" />
        </label>
        <label class="smart-field">
          <span>Threshold <span style="color: var(--color-fg-3);">(0–1, higher = prefer cheap)</span></span>
          <input class="input-field" bind:value={smart.ml_router_threshold} placeholder="0.5" />
        </label>
      </div>
      <div style="font-size: 11px; color: var(--color-fg-3); margin-top: 8px;">
        Tip: call model <span class="font-mono" style="color: var(--color-primary);">ml-auto</span> or <span class="font-mono" style="color: var(--color-primary);">smart</span> to force ML routing per-request, regardless of the toggle.
      </div>
    </div>

    <!-- Cost-based Routing -->
    <div class="smart-block">
      <div class="flex items-center gap-2" style="margin-bottom: 12px;">
        <DollarSign size={16} style="color: var(--color-success);" />
        <span style="font-size: 13px; font-weight: 600; color: var(--color-fg-0);">Cost-based Routing</span>
        <span style="font-size: 11px; color: var(--color-fg-3);">— used by the cost-first profile (translation tasks)</span>
      </div>
      <div class="smart-grid">
        <label class="smart-field">
          <span>Quality floor <span style="color: var(--color-fg-3);">(min success rate)</span></span>
          <input class="input-field" bind:value={smart.cost_quality_floor} placeholder="0.3" />
        </label>
        <label class="smart-field">
          <span>Expensive anchor <span style="color: var(--color-fg-3);">(USD / 2K tok)</span></span>
          <input class="input-field" bind:value={smart.cost_expensive_anchor} placeholder="0.02" />
        </label>
      </div>
    </div>
    {/if}

    {#if section === 'quotas'}
    <!-- Quota Limits -->
    <div class="smart-block">
      <div class="flex items-center justify-between" style="margin-bottom: 12px;">
        <div class="flex items-center gap-2">
          <Gauge size={16} style="color: var(--color-warning, #d97706);" />
          <span style="font-size: 13px; font-weight: 600; color: var(--color-fg-0);">Per-Connection Quota</span>
          <span style="font-size: 11px; color: var(--color-fg-3);">— exhausted connections are skipped before the upstream call</span>
        </div>
        <button class="btn-secondary flex items-center gap-1.5" onclick={addQuotaRow}>
          <Plus size={14} stroke-width={2} /> Add limit
        </button>
      </div>
      {#if quotaRows.length === 0}
        <div style="font-size: 12px; color: var(--color-fg-3); padding: 8px 0;">No quota limits set — all connections run unlimited.</div>
      {:else}
        <div style="display: flex; flex-direction: column; gap: 8px;">
          {#each quotaRows as row, i}
            <div class="flex items-center gap-3">
              <input class="input-field" style="flex: 1;" placeholder="connection id" bind:value={row.connId} />
              <input class="input-field" style="width: 200px;" placeholder="max tokens / day" bind:value={row.maxPerDay} />
              <button class="btn-icon" style="color: var(--color-error);" onclick={() => removeQuotaRow(i)} title="Remove">
                <Trash2 size={14} />
              </button>
            </div>
          {/each}
        </div>
      {/if}
    </div>
    {/if}
  </div>
  {/if}

  {#if section === 'policies'}
  <!-- Load Balancer Section -->
  <div class="card">
    <div class="flex items-center justify-between" style="margin-bottom: 20px;">
      <div class="flex items-center gap-2.5">
        <div
          class="flex items-center justify-center rounded-xl"
          style="width: 40px; height: 40px; background: var(--color-success-light);"
        >
          <Shuffle size={20} style="color: var(--color-success);" stroke-width={1.8} />
        </div>
        <div>
          <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">Load Balancer</div>
          <div style="font-size: 12px; color: var(--color-fg-3);">Current strategy: <span class="font-mono" style="color: var(--color-primary);">{loadBalancerStrategy}</span></div>
        </div>
      </div>
    </div>
    <div class="flex items-center gap-2 flex-wrap">
      {#each strategies as s}
        <button
          class="badge"
          style="font-size: 12px; padding: 6px 14px; cursor: pointer; border: 1px solid {stagedLbStrategy === s.value ? 'var(--color-primary)' : 'var(--color-border)'}; background: {stagedLbStrategy === s.value ? 'var(--color-primary-light)' : 'var(--color-bg-body)'}; color: {stagedLbStrategy === s.value ? 'var(--color-primary)' : 'var(--color-fg-2)'}; border-radius: var(--radius-sm); transition: var(--transition);"
          onclick={() => stagedLbStrategy = s.value}
        >
          <s.icon size={14} style="display: inline; vertical-align: -2px; margin-right: 4px;" />
          {s.label}
        </button>
      {/each}
    </div>
  </div>
  {/if}

  {#if section === 'combos'}
  <!-- Combos Section -->
  <div class="card">
    <div class="flex items-center justify-between" style="margin-bottom: 20px;">
      <div class="flex items-center gap-2.5">
        <div
          class="flex items-center justify-center rounded-xl"
          style="width: 40px; height: 40px; background: var(--color-primary-light);"
        >
          <GitBranch size={20} style="color: var(--color-primary);" stroke-width={1.8} />
        </div>
        <div>
          <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">Routing Combos</div>
          <div style="font-size: 12px; color: var(--color-fg-3);">Drag to reorder priority. Top combo is tried first.</div>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <button
          class="btn-secondary flex items-center gap-1.5"
          onclick={() => showComboForm = !showComboForm}
        >
          <Plus size={14} stroke-width={2} />
          Add Combo
        </button>
        {#if combosDirtyCount > 0 || orderDirty}<button class="btn-secondary" onclick={discardCombos}>Discard strategy edits</button>{/if}
        <button class="btn-primary flex items-center gap-1.5" onclick={saveCombos} disabled={saving}>
          <Save size={14} stroke-width={2} />
          {saving ? 'Saving...' : `Save Combos${combosDirtyCount + (orderDirty ? 1 : 0) ? ` (${combosDirtyCount + (orderDirty ? 1 : 0)})` : ''}`}
        </button>
      </div>
    </div>

    {#if showComboForm}
      <div class="alias-form" style="margin-bottom: 20px;">
        <div style="font-size: 13px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 10px;">
          Create New Combo
        </div>
        <div style="display: flex; flex-direction: column; gap: 12px;">
          <div class="flex items-center gap-3 flex-wrap">
            <input
              class="input-field"
              style="width: 180px;"
              placeholder="Combo name (e.g. fast-code)"
              bind:value={newComboName}
            />
            <select
              class="input-field"
              style="width: 170px;"
              bind:value={newComboStrategy}
            >
              {#each strategies as s}
                <option value={s.value}>{s.label}</option>
              {/each}
            </select>
            <input
              class="input-field"
              style="flex: 1; min-width: 220px;"
              placeholder="Description (optional)"
              bind:value={newComboDescription}
            />
          </div>
          <div class="combo-entry-builder">
            <div class="combo-entry-heading">
              <div><strong>Failover entries</strong><span>Each entry is pinned to the exact connection you select.</span></div>
              <span class="entry-count">{newComboEntries.length} configured</span>
            </div>

            {#if comboCatalogLoading}
              <div class="catalog-state"><Spinner /> Loading provider catalog…</div>
            {:else if comboCatalogError}
              <div class="catalog-state error" role="alert">{comboCatalogError}<button type="button" class="btn-secondary" onclick={loadComboCatalog}>Retry</button></div>
            {:else if comboProviders.length === 0}
              <div class="catalog-state">No active provider connections are available. Add or enable a connection first.</div>
            {:else}
              <div class="selector-steps">
                <label class="selector-step">
                  <span><b>1</b> Provider / Connection</span>
                  <input class="input-field provider-search" list="combo-provider-options" placeholder="Search provider or connection…" bind:value={selectedComboConnection} oninput={() => selectedComboModel = ''} />
                  <datalist id="combo-provider-options">
                    {#each comboProviders as provider}<option value={provider.id}>{provider.provider} — {provider.name}</option>{/each}
                  </datalist>
                  {#if selectedProvider}<small>{selectedProvider.provider} · {selectedProvider.name} · <code>{selectedProvider.id}</code></small>{/if}
                </label>
                <label class="selector-step">
                  <span><b>2</b> Model</span>
                  <input class="input-field" list="combo-model-options" placeholder={selectedComboConnection ? 'Search discovered models…' : 'Choose a provider first'} bind:value={selectedComboModel} disabled={!selectedComboConnection || selectedConnectionModels.length === 0} />
                  <datalist id="combo-model-options">{#each selectedConnectionModels as model}<option value={model}></option>{/each}</datalist>
                  {#if selectedComboConnection && selectedConnectionModels.length === 0}<small>No active discovered models for this connection.</small>{/if}
                </label>
                <div class="entry-actions">
                  <button type="button" class="btn-primary" onclick={addPinnedComboEntry} disabled={!selectedComboConnection || !selectedConnectionModels.includes(selectedComboModel)}>Add entry</button>
                  <button type="button" class="btn-secondary" onclick={syncComboModels} disabled={!selectedProvider?.canSync || Boolean(syncingConnection)}><RotateCw size={13} /> {syncingConnection ? 'Syncing…' : 'Sync Models'}</button>
                </div>
              </div>

              <button type="button" class="advanced-toggle" onclick={() => showAdvancedModelInput = !showAdvancedModelInput}>Advanced: manually enter a model ID</button>
              {#if showAdvancedModelInput}
                <div class="advanced-row">
                  <input class="input-field" placeholder="Exact model ID" bind:value={advancedModel} disabled={!selectedComboConnection} />
                  <button type="button" class="btn-secondary" onclick={addAdvancedComboEntry} disabled={!selectedComboConnection || !advancedModel.trim()}>Add manual entry</button>
                  <small>Compatibility fallback only. The entry remains pinned to the selected connection.</small>
                </div>
              {/if}
            {/if}

            {#if newComboEntries.length > 0}
              <ol class="draft-chain">
                {#each newComboEntries as entry, index}
                  {@const provider = comboProviders.find(option => option.id === entry.connectionId)}
                  <li><span class="chain-step-num">{index + 1}</span><div><code>{entry.model}</code><small>{provider?.provider || 'Connection'} · {provider?.name || entry.connectionId}</small></div><button type="button" class="btn-icon" aria-label={`Remove ${entry.model} from combo`} onclick={() => removeComboEntry(index)}><X size={14} /></button></li>
                {/each}
              </ol>
            {/if}
          </div>
          <div class="flex items-center gap-2">
            <button class="btn-primary" onclick={createCombo} disabled={comboCreating}>
              {comboCreating ? 'Creating...' : 'Create Combo'}
            </button>
            <button class="btn-secondary" onclick={() => { showComboForm = false; }}>
              Cancel
            </button>
          </div>
        </div>
      </div>
    {/if}


    {#if loading}
      <Spinner />
    {:else if combos.length === 0}
      <EmptyState
        icon={GitBranch}
        title="No routing combos"
        description="Combos will appear here once configured."
      />
    {:else}
      <div style="display: flex; flex-direction: column; gap: 10px;">
        {#each combos as combo, i (combo.id)}
          <div
            class="combo-card"
            class:drag-over={dragOverIndex === i && draggedIndex !== i}
            class:dragging={draggedIndex === i}
            draggable="true"
            ondragstart={() => handleDragStart(i)}
            ondragover={(e) => handleDragOver(e, i)}
            ondragend={handleDragEnd}
            role="listitem"
          >
            <div class="flex items-center gap-3" style="flex: 1; min-width: 0;">
              <!-- Drag Handle -->
              <div class="drag-handle" title="Drag to reorder">
                <GripVertical size={16} style="color: var(--color-fg-3);" />
              </div>

              <!-- Order Number -->
              <div
                class="flex items-center justify-center rounded-lg font-mono font-bold"
                style="
                  width: 28px; height: 28px; font-size: 12px;
                  background: var(--color-primary-light); color: var(--color-primary);
                  flex-shrink: 0;
                "
              >{i + 1}</div>

              <!-- Combo Info -->
              <div style="flex: 1; min-width: 0;">
                <div class="flex items-center gap-2" style="margin-bottom: 6px; flex-wrap: wrap;">
                  <div class="flex items-center gap-1.5">
                    <Server size={14} style="color: var(--color-primary);" />
                    <span style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">{combo.provider}</span>
                  </div>
                  {#if combo.description}
                    <span style="font-size: 11px; color: var(--color-fg-3);">({combo.description})</span>
                  {/if}
                </div>

                <!-- Visual Failover Chain -->
                {#if combo.models && combo.models.length > 0}
                  <div class="flex items-center gap-1.5 flex-wrap" style="margin-top: 4px;">
                    {#each combo.models as model, mIdx}
                      <span class="chain-chip" class:primary-target={mIdx === 0}>
                        <span class="chain-step-num">{mIdx + 1}</span>
                        <span class="chain-model-name">{model}</span>
                        {#if mIdx === 0}
                          <span class="chain-badge primary">primary</span>
                        {:else}
                          <span class="chain-badge fallback">fallback</span>
                        {/if}
                      </span>
                      {#if mIdx < combo.models.length - 1}
                        <span class="chain-arrow">&rarr;</span>
                      {/if}
                    {/each}
                  </div>
                {:else if combo.keys && combo.keys.length > 0}
                  <div class="flex items-center gap-2 flex-wrap">
                    {#each combo.keys as key}
                      <span class="badge" style="background: var(--color-border-light); color: var(--color-fg-2); font-size: 10px;">
                        {key.slice(0, 8)}...
                      </span>
                    {/each}
                  </div>
                {/if}
              </div>
            </div>

            <!-- Strategy Selector & Delete -->
            <div class="flex items-center gap-2" style="flex-shrink: 0;">
              <select
                class="input-field"
                style="width: 160px; font-size: 12px; padding: 6px 10px;"
                value={combo.strategy}
                onchange={(e) => updateStrategy(combo.id, (e.target as HTMLSelectElement).value)}
              >
                {#each strategies as s}
                  <option value={s.value}>{s.label}</option>
                {/each}
              </select>

              <button
                class="btn-icon"
                style="color: var(--color-error);"
                onclick={() => deleteCombo(combo.id, combo.provider)}
                title="Delete combo"
                aria-label={`Delete combo ${combo.provider}`}
              >
                <Trash2 size={14} />
              </button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Aliases Section -->
  <div class="card">
    <div class="flex items-center justify-between" style="margin-bottom: 20px;">
      <div class="flex items-center gap-2.5">
        <div
          class="flex items-center justify-center rounded-xl"
          style="width: 40px; height: 40px; background: var(--color-purple-light);"
        >
          <Tag size={20} style="color: var(--color-purple);" stroke-width={1.8} />
        </div>
        <div>
          <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">Model Aliases</div>
          <div style="font-size: 12px; color: var(--color-fg-3);">Map friendly names to actual model identifiers. Creating or deleting an alias applies immediately and does not use Save Combos.</div>
        </div>
      </div>
      <button
        class="btn-secondary flex items-center gap-1.5"
        onclick={() => showAliasForm = !showAliasForm}
      >
        <Plus size={14} stroke-width={2} />
        Add Alias
      </button>
    </div>

    {#if aliasFeedback}
      <div class="alias-feedback" class:error={aliasFeedbackKind === 'error'} role={aliasFeedbackKind === 'error' ? 'alert' : 'status'}>
        {aliasFeedback}
      </div>
    {/if}

    {#if showAliasForm}
      <div class="alias-form">
        <div class="flex items-center gap-3 flex-wrap">
          <input
            class="input-field"
            style="width: 200px;"
            placeholder="Alias name (e.g. gpt-4)"
            bind:value={newAlias}
          />
          <span style="color: var(--color-fg-3); font-size: 18px; font-weight: 300;">&rarr;</span>
          <input
            class="input-field"
            style="width: 280px;"
            placeholder="Target model (e.g. gpt-4-turbo-preview)"
            bind:value={newTarget}
          />
          <button class="btn-primary" onclick={addAlias} aria-label="Create alias">
            <Plus size={14} stroke-width={2} />
          </button>
          <button class="btn-secondary" onclick={() => { showAliasForm = false; newAlias = ''; newTarget = ''; }}>
            Cancel
          </button>
        </div>
      </div>
    {/if}

    {#if loading}
      <Spinner />
    {:else if aliases.length === 0 && !showAliasForm}
      <EmptyState
        icon={Tag}
        title="No aliases configured"
        description="Create aliases to map short names to full model identifiers."
      />
    {:else}
      <div style="display: flex; flex-direction: column; gap: 8px;">
        {#each aliases as alias (alias.id)}
          <div class="alias-row flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div
                class="flex items-center justify-center rounded-lg"
                style="width: 32px; height: 32px; background: var(--color-purple-light);"
              >
                <Tag size={14} style="color: var(--color-purple);" />
              </div>
              <div>
                <span class="font-mono font-semibold" style="font-size: 13px; color: var(--color-fg-0);">{alias.alias}</span>
                <span style="color: var(--color-fg-3); margin: 0 8px;">&rarr;</span>
                <span class="font-mono" style="font-size: 13px; color: var(--color-fg-2);">{alias.target}</span>
              </div>
            </div>
            {#if !isBuiltInAlias(alias.id)}
              <button
                class="btn-icon"
                style="color: var(--color-error);"
                onclick={() => deleteAlias(alias.id)}
                title="Delete alias"
                aria-label={`Delete alias ${alias.alias}`}
              >
                <Trash2 size={14} />
              </button>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>
  {/if}

  {#if error}
    <div
      role="alert"
      class="flex items-center gap-2"
      style="
        padding: 12px 16px; border-radius: var(--radius-sm);
        background: var(--color-error-light); color: var(--color-error);
        font-size: 13px; font-weight: 500;
      "
    >
      {error}
      <button style="margin-left: auto; cursor: pointer; color: var(--color-error); background: none; border: none;" onclick={() => error = ''}>&times;</button>
    </div>
  {/if}
</div>

<style>
  .routing-section-nav { display:grid; grid-template-columns:repeat(3,1fr); gap:8px; margin-bottom:12px; }
  .combo-entry-builder { padding:14px; border:1px solid var(--color-border); border-radius:10px; background:var(--color-bg-body); }
  .combo-entry-heading { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; margin-bottom:12px; }
  .combo-entry-heading strong,.combo-entry-heading span { display:block; }
  .combo-entry-heading strong { font-size:13px; color:var(--color-fg-0); }
  .combo-entry-heading span,.selector-step small,.advanced-row small,.draft-chain small { font-size:10px; color:var(--color-fg-3); margin-top:2px; }
  .entry-count { padding:3px 8px; border-radius:999px; background:var(--color-primary-light); color:var(--color-primary) !important; white-space:nowrap; }
  .selector-steps { display:grid; grid-template-columns:minmax(220px,1fr) minmax(220px,1fr) auto; gap:10px; align-items:end; }
  .selector-step { display:flex; flex-direction:column; gap:5px; min-width:0; }
  .selector-step>span { font-size:11px; font-weight:650; color:var(--color-fg-2); }
  .selector-step b { display:inline-grid; place-items:center; width:18px; height:18px; border-radius:50%; background:var(--color-primary); color:white; margin-right:4px; }
  .entry-actions { display:flex; gap:6px; flex-wrap:wrap; }
  .entry-actions button { display:inline-flex; align-items:center; gap:5px; }
  .catalog-state { display:flex; align-items:center; gap:9px; padding:14px; border:1px dashed var(--color-border); border-radius:8px; font-size:12px; color:var(--color-fg-2); }
  .catalog-state.error { border-color:var(--color-error); color:var(--color-error); background:var(--color-error-light); }
  .catalog-state button { margin-left:auto; }
  .advanced-toggle { margin-top:10px; border:0; background:transparent; color:var(--color-primary); font-size:11px; cursor:pointer; padding:3px 0; }
  .advanced-row { display:grid; grid-template-columns:minmax(220px,1fr) auto; gap:8px; align-items:center; margin-top:7px; }
  .advanced-row small { grid-column:1/-1; }
  .draft-chain { list-style:none; margin:12px 0 0; padding:0; display:flex; flex-direction:column; gap:6px; }
  .draft-chain li { display:flex; align-items:center; gap:9px; padding:8px 10px; border:1px solid var(--color-border); border-radius:8px; background:var(--color-bg-card); }
  .draft-chain li>div { flex:1; min-width:0; display:flex; flex-direction:column; }
  .draft-chain code { font-size:11px; overflow:hidden; text-overflow:ellipsis; }
  @media (max-width: 760px) { .selector-steps { grid-template-columns:1fr; } .entry-actions { justify-content:flex-start; } }
  .routing-section-nav button { text-align:left; border:1px solid var(--color-border); background:var(--color-bg-card); border-radius:10px; padding:11px 13px; cursor:pointer; color:var(--color-fg-1); }
  .routing-section-nav button.active { border-color:var(--color-primary); background:var(--color-primary-light); color:var(--color-primary); }
  .routing-section-nav span { display:block; font-size:13px; font-weight:700; }
  .routing-section-nav small { display:block; margin-top:3px; font-size:10px; color:var(--color-fg-3); line-height:1.35; }
  .dirty-bar { position:sticky; top:calc(var(--header-h) + 8px); z-index:20; padding:9px 13px; margin-bottom:16px; border:1px solid color-mix(in srgb,var(--color-warning) 35%,transparent); border-radius:9px; background:color-mix(in srgb,var(--color-warning) 10%,var(--color-bg-card)); color:var(--color-fg-1); font-size:11px; box-shadow:var(--shadow-sm); }
  .alias-feedback { margin-bottom:14px; padding:9px 12px; border:1px solid color-mix(in srgb,var(--color-success) 35%,transparent); border-radius:8px; background:var(--color-success-light); color:var(--color-success); font-size:12px; font-weight:600; }
  .alias-feedback.error { border-color:color-mix(in srgb,var(--color-error) 35%,transparent); background:var(--color-error-light); color:var(--color-error); }
  .smart-block {
    padding: 16px;
    background: var(--color-bg-body);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    margin-bottom: 14px;
  }
  .smart-block:last-child {
    margin-bottom: 0;
  }
  .smart-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 12px;
  }
  .smart-field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 12px;
    color: var(--color-fg-2);
    font-weight: 500;
  }
  .combo-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 14px 16px;
    background: var(--color-bg-body);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    cursor: grab;
    transition: var(--transition);
  }
  .combo-card:hover {
    border-color: var(--color-primary);
    box-shadow: 0 0 0 3px var(--color-primary-glow);
  }
  .combo-card.dragging {
    opacity: 0.5;
    transform: scale(0.98);
  }
  .combo-card.drag-over {
    border-color: var(--color-primary);
    border-style: dashed;
    background: var(--color-primary-light);
  }
  .drag-handle {
    cursor: grab;
    padding: 4px;
    border-radius: 4px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .drag-handle:hover {
    background: var(--color-border);
  }
  .alias-form {
    padding: 16px;
    background: var(--color-bg-body);
    border: 1px dashed var(--color-border);
    border-radius: var(--radius-sm);
    margin-bottom: 16px;
  }
  .alias-row {
    padding: 10px 14px;
    background: var(--color-bg-body);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    transition: var(--transition);
  }
  .alias-row:hover {
    border-color: var(--color-border);
    box-shadow: var(--shadow-sm);
  }
  .btn-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border-radius: var(--radius-sm);
    border: none;
    background: transparent;
    cursor: pointer;
    transition: var(--transition);
  }
  .btn-icon:hover {
    background: var(--color-error-light);
  }
  @media (max-width: 768px) {
    .routing-section-nav { grid-template-columns:1fr; }
    .combo-card {
      flex-direction: column;
      align-items: flex-start;
    }
  }

  .chain-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    background: var(--color-bg-body);
    border: 1px solid var(--color-border);
    border-radius: 6px;
    padding: 2px 7px;
    font-size: 11px;
    font-family: var(--font-mono);
  }
  .chain-chip.primary-target {
    border-color: rgba(59, 130, 246, 0.4);
    background: rgba(59, 130, 246, 0.05);
  }
  .chain-step-num {
    font-size: 10px;
    font-weight: 700;
    color: var(--color-fg-3);
  }
  .chain-model-name {
    color: var(--color-fg-1);
  }
  .chain-badge {
    font-size: 9px;
    padding: 1px 4px;
    border-radius: 3px;
    text-transform: uppercase;
    letter-spacing: 0.4px;
    font-weight: 600;
  }
  .chain-badge.primary {
    background: var(--color-primary-light);
    color: var(--color-primary);
  }
  .chain-badge.fallback {
    background: rgba(245, 158, 11, 0.12);
    color: #f59e0b;
  }
  .chain-arrow {
    color: var(--color-fg-3);
    font-size: 13px;
    font-weight: 600;
  }
</style>
