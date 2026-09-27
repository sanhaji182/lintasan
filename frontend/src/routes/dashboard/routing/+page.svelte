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
  import { comboProviderOptions, comboModelsForProvider, comboProviderForEntry, buildComboEntries, filterComboModels, comboCatalogAvailability, comboMatchesSavedPayload } from '$lib/combo-entry-selector';
  import {
    GitBranch, GripVertical, Plus, Trash2, Save,
    Server, Tag, Shuffle, RotateCw, CircleDot,
    BrainCircuit, DollarSign, Gauge, Layers, ToggleLeft, ToggleRight, X, Sparkles, Check, Search, AlertTriangle, Pencil, ArrowUp, ArrowDown
  } from 'lucide-svelte/icons';

  interface Combo {
    id: string;
    provider: string;
    strategy: string;
    keys: string[];
    models?: string[];
    description?: string;
    order: number;
    revision?: string;
    entries?: Array<{ model: string; provider_id?: string; connection_id?: string; connection_ids?: string[] }>;
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
  let comboProviderCatalog = $state<any[]>([]);
  let comboCatalogLoading = $state(false);
  let comboCatalogError = $state('');
  let selectedComboProvider = $state('');
  let pinnedComboConnection = $state('');
  let selectedComboModel = $state('');
  let comboModelQuery = $state('');
  type DraftEntry = { model: string; providerId: string; connectionId?: string; preservedEntry?: { model: string; provider_id?: string; connection_id?: string; connection_ids?: string[] } };
  let newComboEntries = $state<DraftEntry[]>([]);
  let showAdvancedModelInput = $state(false);
  let advancedModel = $state('');
  let syncingConnection = $state('');
  let comboCreating = $state(false);
  let editingComboId = $state('');
  let editingComboRevision = $state('');
  let comboFormBaseline = $state('');
  let comboSaveState = $state<'idle' | 'conflict' | 'reload-failed'>('idle');
  let comboSaveMessage = $state('');
  let savedComboPayload = $state<{ name: string; strategy: string; description: string; entries: any[] } | null>(null);
  const comboProviders = $derived(comboProviderOptions(comboConnections, comboProviderCatalog));
  const selectedProvider = $derived(comboProviders.find(option => option.id === selectedComboProvider));
  const selectedProviderModels = $derived(comboModelsForProvider(comboDiscoveredModels, selectedProvider));
  const filteredProviderModels = $derived(filterComboModels(selectedProviderModels, comboModelQuery));
  const comboCatalogState = $derived(comboCatalogAvailability(comboConnections));
  const comboFormFingerprint = $derived(JSON.stringify({ name: newComboName, strategy: newComboStrategy, description: newComboDescription, entries: newComboEntries }));
  const comboFormDirty = $derived(Boolean(showComboForm && comboFormBaseline && comboFormFingerprint !== comboFormBaseline));

  $effect(() => {
    if (selectedComboModel && !filteredProviderModels.includes(selectedComboModel)) {
      selectedComboModel = '';
    }
  });

  function selectComboProvider(providerId: string) {
    selectedComboProvider = providerId;
    selectedComboModel = '';
    pinnedComboConnection = '';
    comboModelQuery = '';
  }

  function handleModelSearchKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && comboModelQuery) {
      event.preventDefault();
      event.stopPropagation();
      comboModelQuery = '';
    }
  }

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
    combos: combosDirtyCount > 0 || orderDirty || comboFormDirty,
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

  function normalizeCombos(raw: any): Combo[] {
    return Array.isArray(raw) ? raw.map((c: any, i: number) => ({
      id: c.id || c.name || `combo-${i}`,
      provider: c.name || c.provider || 'Unknown',
      strategy: c.strategy || 'priority',
      keys: Array.isArray(c.keys) ? c.keys : [],
      models: Array.isArray(c.models) && c.models.length > 0 ? c.models : (Array.isArray(c.entries) ? c.entries.map((e: any) => e.model) : []),
      description: c.description || '',
      order: c.order ?? i,
      entries: Array.isArray(c.entries) ? c.entries : [],
      revision: c.revision || '',
    })) : [];
  }

  async function loadCombos(strict = false) {
    try {
      const res = await api.get<any>('/api/combos');
      const raw = res?.data || res?.combos || [];
      combos = normalizeCombos(raw);
      baseStrategies = Object.fromEntries(combos.map(combo => [combo.id, combo.strategy]));
      stagedStrategies = {};
      return combos;
    } catch (e) {
      // A transport failure is not an authoritative empty collection.
      if (strict) throw e;
      return combos;
    }
  }

  async function loadComboCatalog() {
    comboCatalogLoading = true;
    comboCatalogError = '';
    try {
      const [connectionsResponse, modelsResponse, presetsResponse] = await Promise.all([
        api.get<any>('/api/connections'),
        api.get<any>('/api/models/discovered'),
        api.get<any>('/api/presets').catch(() => ({ data: [] })),
      ]);
      comboConnections = Array.isArray(connectionsResponse?.data) ? connectionsResponse.data : [];
      comboDiscoveredModels = Array.isArray(modelsResponse?.data) ? modelsResponse.data : [];
      comboProviderCatalog = Array.isArray(presetsResponse?.data) ? presetsResponse.data : [];
    } catch (e: any) {
      comboConnections = [];
      comboDiscoveredModels = [];
      comboProviderCatalog = [];
      comboCatalogError = e.message || 'Provider catalog could not be loaded.';
    } finally {
      comboCatalogLoading = false;
    }
  }

  function addPinnedComboEntry() {
    if (!selectedProvider || !selectedComboModel || !filteredProviderModels.includes(selectedComboModel)) return;
    newComboEntries = [...newComboEntries, {
      model: selectedComboModel,
      providerId: selectedProvider.id,
      ...(pinnedComboConnection ? { connectionId: pinnedComboConnection } : {}),
    }];
    selectedComboModel = '';
  }

  function addAdvancedComboEntry() {
    const model = advancedModel.trim();
    if (!model || !selectedProvider) return;
    newComboEntries = [...newComboEntries, {
      model, providerId: selectedProvider.id,
      ...(pinnedComboConnection ? { connectionId: pinnedComboConnection } : {}),
    }];
    advancedModel = '';
  }

  function removeComboEntry(index: number) {
    newComboEntries = newComboEntries.filter((_, entryIndex) => entryIndex !== index);
  }

  function moveComboEntry(index: number, direction: -1 | 1) {
    const target = index + direction;
    if (target < 0 || target >= newComboEntries.length) return;
    const entries = [...newComboEntries];
    [entries[index], entries[target]] = [entries[target], entries[index]];
    newComboEntries = entries;
  }

  function resetComboForm() {
    showComboForm = false;
    editingComboId = '';
    editingComboRevision = '';
    newComboName = '';
    newComboStrategy = 'priority';
    newComboDescription = '';
    newComboEntries = [];
    selectedComboProvider = '';
    pinnedComboConnection = '';
    selectedComboModel = '';
    comboModelQuery = '';
    advancedModel = '';
    showAdvancedModelInput = false;
    comboFormBaseline = '';
    comboSaveState = 'idle';
    comboSaveMessage = '';
    savedComboPayload = null;
  }

  function openCreateCombo() {
    if (comboFormDirty && !confirm('Discard unsaved combo edits?')) return;
    resetComboForm();
    showComboForm = true;
    comboFormBaseline = JSON.stringify({ name: '', strategy: 'priority', description: '', entries: [] });
  }

  function openEditCombo(combo: Combo) {
    if ((combosDirtyCount > 0 || orderDirty) && !confirm('Discard staged Combo strategy/order changes before editing?')) return;
    if (combosDirtyCount > 0 || orderDirty) discardCombos();
    if (comboFormDirty && !confirm('Discard unsaved combo edits?')) return;
    editingComboId = combo.id;
    editingComboRevision = combo.revision || '';
    newComboName = combo.provider;
    newComboStrategy = combo.strategy;
    newComboDescription = combo.description || '';
    const source: Array<{ model: string; provider_id?: string; connection_id?: string; connection_ids?: string[] }> = (combo.models?.length ? combo.models : (combo.entries || []).map(entry => entry.model))
      .map((model, index) => combo.entries?.[index] || { model });
    newComboEntries = source.map(entry => {
      const provider = comboProviderForEntry(entry, comboProviders);
      return {
        model: entry.model,
        providerId: entry.provider_id || provider?.id || '',
        ...(entry.connection_id ? { connectionId: entry.connection_id } : {}),
        preservedEntry: { ...entry },
      };
    });
    showComboForm = true;
    comboFormBaseline = JSON.stringify({ name: newComboName, strategy: newComboStrategy, description: newComboDescription, entries: newComboEntries });
    comboSaveState = 'idle';
    comboSaveMessage = '';
  }

  function cancelComboForm() {
    if (comboFormDirty && !confirm('Discard unsaved combo edits?')) return;
    resetComboForm();
  }

  function handleComboFormKeydown(event: KeyboardEvent) {
    if (event.key !== 'Escape' || event.defaultPrevented || comboCreating) return;
    if ((event.target as HTMLElement).closest('[role="dialog"]')) return;
    event.preventDefault();
    event.stopPropagation();
    cancelComboForm();
  }

  async function reloadLatestCombo() {
    if (comboFormDirty && !confirm('Discard this draft and reload the latest saved combo?')) return;
    comboCreating = true;
    try {
      const id = editingComboId;
      const loaded = await loadCombos(true);
      const latest = loaded.find(combo => combo.id === id);
      if (!latest) throw new Error('The combo no longer exists.');
      comboFormBaseline = '';
      openEditCombo(latest);
    } catch (e: any) {
      comboSaveMessage = e.message || 'Failed to reload the latest combo.';
      showToast(comboSaveMessage, 'error');
    } finally { comboCreating = false; }
  }

  async function retryComboReload() {
    comboCreating = true;
    try {
      const id = editingComboId;
      const loaded = await loadCombos(true);
      const authoritative = loaded.find(combo => combo.id === id);
      if (!authoritative) throw new Error('Saved combo was not found after refresh.');
      if (!savedComboPayload || !comboMatchesSavedPayload(authoritative, savedComboPayload)) throw new Error('Refresh did not confirm the saved combo state.');
      const name = newComboName.trim();
      resetComboForm();
      showToast(`Combo "${name}" updated successfully`, 'success');
    } catch (e: any) {
      comboSaveMessage = `Combo was saved, but refresh failed: ${e.message || 'unknown error'}`;
      showToast(comboSaveMessage, 'error');
    } finally { comboCreating = false; }
  }


  async function syncComboModels() {
    const connectionID = pinnedComboConnection || selectedProvider?.connectionIds[0];
    if (!connectionID) return;
    syncingConnection = connectionID;
    comboCatalogError = '';
    try {
      await api.post(`/api/models/sync/${encodeURIComponent(connectionID)}`, {});
      const modelsResponse = await api.get<any>('/api/models/discovered');
      comboDiscoveredModels = Array.isArray(modelsResponse?.data) ? modelsResponse.data : [];
      showToast(`Models synced for ${selectedProvider?.name || connectionID}`, 'success');
    } catch (e: any) {
      comboCatalogError = e.message || 'Model sync failed.';
      showToast(comboCatalogError, 'error');
    } finally {
      syncingConnection = '';
    }
  }

  async function saveComboForm() {
    const name = newComboName.trim();
    if (!name) {
      showToast('Combo name is required', 'error');
      return;
    }
    const entries = buildComboEntries(newComboEntries);

    if (entries.length === 0) {
      showToast('At least one provider and model entry is required', 'error');
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
      if (editingComboId) {
        await api.put(`/api/combos?id=${encodeURIComponent(editingComboId)}`, { ...payload, expected_revision: editingComboRevision });
        savedComboPayload = payload;
        try {
          const editedID = editingComboId;
          const loaded = await loadCombos(true);
          const authoritative = loaded.find(combo => combo.id === editedID);
          if (!authoritative) throw new Error('Saved combo was not found after refresh.');
          if (!comboMatchesSavedPayload(authoritative, payload)) throw new Error('Refresh did not confirm the saved combo state.');
          resetComboForm();
          showToast(`Combo "${name}" updated successfully`, 'success');
        } catch (reloadError: any) {
          comboSaveState = 'reload-failed';
          comboSaveMessage = `Combo was saved, but refresh failed: ${reloadError.message || 'unknown error'}`;
          showToast(comboSaveMessage, 'error');
        }
      } else {
        await api.post('/api/combos', payload);
        await loadCombos(true);
        resetComboForm();
        showToast(`Combo "${name}" created successfully`, 'success');
      }
    } catch (e: any) {
      if (e.status === 409 && editingComboId) {
        comboSaveState = 'conflict';
        comboSaveMessage = 'This combo changed after you opened it. Your draft is preserved; reload the latest version before reapplying your edits.';
      } else {
        comboSaveMessage = e.message || `Failed to ${editingComboId ? 'update' : 'create'} combo`;
      }
      showToast(comboSaveMessage, 'error');
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
          onclick={openCreateCombo}
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
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <form class="alias-form" style="margin-bottom: 20px;" aria-label="Combo editor" onkeydown={handleComboFormKeydown} onsubmit={(event) => event.preventDefault()}>
        <div style="font-size: 13px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 10px;">
          {editingComboId ? 'Edit Combo' : 'Create New Combo'}
        </div>
        <div style="display: flex; flex-direction: column; gap: 12px;">
          {#if comboSaveState !== 'idle'}
            <div class="catalog-state error" role="alert">
              <AlertTriangle size={18} />
              <span><strong>{comboSaveState === 'conflict' ? 'Edit conflict' : 'Saved, refresh failed'}</strong>{comboSaveMessage}</span>
              <button type="button" class="btn-secondary" onclick={comboSaveState === 'conflict' ? reloadLatestCombo : retryComboReload} disabled={comboCreating}>
                {comboSaveState === 'conflict' ? 'Reload latest' : 'Retry reload'}
              </button>
            </div>
          {/if}
          <div class="flex items-center gap-3 flex-wrap">
            <input
              class="input-field"
              style="width: 180px;"
              placeholder="Combo name (e.g. fast-code)"
              bind:value={newComboName}
              disabled={Boolean(editingComboId)}
            />
            {#if editingComboId}<small class="edit-name-help">Name is fixed while editing because rename uniqueness is not validated by the update contract.</small>{/if}
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
              <div><strong>Failover entries</strong><span>Each entry targets a provider pool unless you explicitly pin an account.</span></div>
              <span class="entry-count">{newComboEntries.length} configured</span>
            </div>

            {#if comboCatalogLoading}
              <div class="catalog-state" role="status" aria-live="polite"><Spinner /> Loading provider catalog…</div>
            {:else if comboCatalogError}
              <div class="catalog-state error" role="alert"><AlertTriangle size={18} /><span><strong>Provider catalog unavailable</strong>{comboCatalogError}</span><button type="button" class="btn-secondary" onclick={loadComboCatalog}>Retry</button></div>
            {:else if comboCatalogState === 'empty'}
              <div class="catalog-state"><Server size={18} /><span><strong>No provider connections</strong>Add a connection before creating a routed entry.</span></div>
            {:else if comboCatalogState === 'inactive'}
              <div class="catalog-state warning" role="status"><AlertTriangle size={18} /><span><strong>Providers unavailable</strong>Your connections exist but none are active. Enable one to continue.</span></div>
            {:else}
              <div class="provider-picker" aria-label="Available providers">
                <div class="picker-label"><span><b>1</b> Provider</span><small>Choose the provider pool. Account identity stays visible below the canonical name.</small></div>
                <div class="provider-grid">
                  {#each comboProviders as provider}
                    {@const providerModels = comboModelsForProvider(comboDiscoveredModels, provider)}
                    <button
                      type="button"
                      class="provider-option"
                      class:selected={provider.id === selectedComboProvider}
                      aria-pressed={provider.id === selectedComboProvider}
                      aria-label={`${provider.name}, ${provider.accounts.length} account${provider.accounts.length === 1 ? '' : 's'}, ${providerModels.length} model${providerModels.length === 1 ? '' : 's'}`}
                      onclick={() => selectComboProvider(provider.id)}
                    >
                      <span class="provider-mark"><Server size={17} /></span>
                      <span class="provider-copy"><strong>{provider.name}</strong><small>{provider.accounts.map(account => account.name).join(', ')}</small><span>{provider.accounts.length} account{provider.accounts.length === 1 ? '' : 's'} · {providerModels.length} model{providerModels.length === 1 ? '' : 's'}</span></span>
                      {#if provider.id === selectedComboProvider}<span class="selected-label"><Check size={13} /> Selected</span>{/if}
                    </button>
                  {/each}
                </div>
              </div>

              {#if selectedProvider}
                <div class="model-picker-panel">
                  <div class="picker-label"><span><b>2</b> Model</span><small>{selectedProvider.name} · {selectedProvider.accounts.map(account => account.name).join(', ')}</small></div>
                  {#if selectedProviderModels.length === 0}
                    <div class="catalog-state no-models" role="status"><Search size={18} /><span><strong>No synced models</strong>No active discovered models are available for {selectedProvider.name}. Sync the provider to refresh its catalog.</span></div>
                  {:else}
                    <label class="model-search"><Search size={16} /><span class="sr-only">Search {selectedProvider.name} models</span><input role="combobox" aria-label={`Search ${selectedProvider.name} models`} aria-controls="combo-model-options" aria-expanded="true" placeholder="Search model ID…" bind:value={comboModelQuery} onkeydown={handleModelSearchKeydown} /></label>
                    <div id="combo-model-options" class="model-options" role="listbox" aria-label={`${selectedProvider.name} models`}>
                      {#each filteredProviderModels as model}
                        <button type="button" role="option" aria-selected={selectedComboModel === model} class:selected={selectedComboModel === model} onclick={() => selectedComboModel = model}><code>{model}</code>{#if selectedComboModel === model}<span><Check size={13} /> Selected</span>{/if}</button>
                      {:else}
                        <div class="model-empty">No models match “{comboModelQuery}”. Press Escape to clear.</div>
                      {/each}
                    </div>
                  {/if}
                  <div class="entry-actions">
                    <button type="button" class="btn-primary" onclick={addPinnedComboEntry} disabled={!filteredProviderModels.includes(selectedComboModel)}>Add entry</button>
                    <button type="button" class="btn-secondary" onclick={syncComboModels} disabled={!selectedProvider.canSync || Boolean(syncingConnection)} aria-busy={Boolean(syncingConnection)}><span class:spinning={Boolean(syncingConnection)}><RotateCw size={13} /></span> {syncingConnection ? 'Syncing…' : 'Sync Models'}</button>
                  </div>
                </div>
              {/if}

              <button type="button" class="advanced-toggle" aria-expanded={showAdvancedModelInput} onclick={() => showAdvancedModelInput = !showAdvancedModelInput}>Advanced: pin to specific account or enter a model ID</button>
              {#if showAdvancedModelInput}
                <div class="advanced-row">
                  <label><span>Account / connection</span><select class="input-field" aria-label="Pin to specific account" bind:value={pinnedComboConnection} disabled={!selectedProvider}>
                    <option value="">Any healthy account in provider</option>
                    {#each selectedProvider?.accounts || [] as account}<option value={account.id}>{account.name} — {account.id}</option>{/each}
                  </select></label>
                  <label><span>Exact model ID</span><input class="input-field" placeholder="Exact model ID (optional)" bind:value={advancedModel} disabled={!selectedProvider} /></label>
                  <button type="button" class="btn-secondary" onclick={addAdvancedComboEntry} disabled={!selectedProvider || !advancedModel.trim()}>Add manual entry</button>
                  <small>Pinning stores the exact account ID. Without a pin, runtime may use any active account in this provider.</small>
                </div>
              {/if}
            {/if}

            {#if newComboEntries.length > 0}
              <section class="draft-summary" aria-label="Pre-save route summary">
                <div class="summary-heading"><div><strong>Route summary</strong><span>Saved in this failover order</span></div><span>{newComboEntries.length} {newComboEntries.length === 1 ? 'entry' : 'entries'}</span></div>
                <ol class="draft-chain">
                  {#each newComboEntries as entry, index}
                    {@const provider = comboProviders.find(option => option.id === entry.providerId) || (entry.preservedEntry ? comboProviderForEntry(entry.preservedEntry, comboProviders) : undefined)}
                    {@const account = provider?.accounts.find(option => option.id === entry.connectionId)}
                    <li><span class="chain-step-num">{index + 1}</span><div class="summary-route"><strong>{provider?.name || (entry.preservedEntry && !entry.preservedEntry.provider_id && !entry.preservedEntry.connection_id ? 'Legacy model-only' : entry.providerId)}</strong><span class="route-arrow" aria-hidden="true">→</span><small>{entry.preservedEntry && !entry.preservedEntry.provider_id && !entry.preservedEntry.connection_id ? 'Identity unchanged' : (account?.name || 'Provider pool')}</small><span class="route-arrow" aria-hidden="true">→</span><code>{entry.model}</code></div><button type="button" class="btn-icon" aria-label={`Move ${entry.model} up`} disabled={index === 0} onclick={() => moveComboEntry(index, -1)}><ArrowUp size={14} /></button><button type="button" class="btn-icon" aria-label={`Move ${entry.model} down`} disabled={index === newComboEntries.length - 1} onclick={() => moveComboEntry(index, 1)}><ArrowDown size={14} /></button><button type="button" class="btn-icon" aria-label={`Remove ${entry.model} from combo`} onclick={() => removeComboEntry(index)}><X size={14} /></button></li>
                  {/each}
                </ol>
              </section>
            {/if}
          </div>
          <div class="flex items-center gap-2">
            <button type="button" class="btn-primary" onclick={saveComboForm} disabled={comboCreating || comboSaveState !== 'idle'}>
              {comboCreating ? (editingComboId ? 'Saving...' : 'Creating...') : (editingComboId ? 'Save Combo' : 'Create Combo')}
            </button>
            <button type="button" class="btn-secondary" onclick={cancelComboForm}>
              Cancel
            </button>
          </div>
        </div>
      </form>
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
            draggable={!showComboForm}
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
                      {@const entryProvider = combo.entries?.[mIdx] ? comboProviderForEntry(combo.entries[mIdx], comboProviders) : undefined}
                      <span class="chain-chip" class:primary-target={mIdx === 0}>
                        <span class="chain-step-num">{mIdx + 1}</span>
                        <span class="chain-model-name">{entryProvider ? `${entryProvider.name} · ${model}` : model}</span>
                        {#if entryProvider && combo.entries?.[mIdx]?.connection_id}
                          <span class="chain-badge account">{entryProvider.accounts.find(account => account.id === combo.entries?.[mIdx]?.connection_id)?.name || combo.entries[mIdx].connection_id}</span>
                        {/if}
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
                disabled={showComboForm}
              >
                {#each strategies as s}
                  <option value={s.value}>{s.label}</option>
                {/each}
              </select>

              <button
                class="btn-icon edit-button"
                onclick={() => openEditCombo(combo)}
                disabled={showComboForm}
                title="Edit combo"
                aria-label={`Edit combo ${combo.provider}`}
              >
                <Pencil size={14} />
              </button>

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
  .edit-name-help { max-width:260px; color:var(--color-fg-3); font-size:10px; line-height:1.35; }
  .btn-icon.edit-button { color:var(--color-primary); }
  .btn-icon.edit-button:hover { background:var(--color-primary-light); }
  .btn-icon:disabled { opacity:.35; cursor:not-allowed; }
  .routing-section-nav { display:grid; grid-template-columns:repeat(3,1fr); gap:8px; margin-bottom:12px; }
  .combo-entry-builder { min-width:0; padding:16px; border:1px solid var(--color-border); border-radius:12px; background:var(--color-bg-body); overflow:hidden; }
  .combo-entry-heading { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; margin-bottom:14px; }
  .combo-entry-heading strong,.combo-entry-heading span,.catalog-state strong { display:block; }
  .combo-entry-heading strong { font-size:13px; color:var(--color-fg-0); }
  .combo-entry-heading span,.advanced-row small,.draft-chain small,.picker-label small { font-size:11px; color:var(--color-fg-3); margin-top:2px; line-height:1.4; }
  .entry-count { padding:3px 8px; border-radius:999px; background:var(--color-primary-light); color:var(--color-primary) !important; white-space:nowrap; }
  .provider-picker,.model-picker-panel { min-width:0; }
  .picker-label { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; margin-bottom:8px; }
  .picker-label>span { font-size:12px; font-weight:700; color:var(--color-fg-1); white-space:nowrap; }
  .picker-label b { display:inline-grid; place-items:center; width:20px; height:20px; border-radius:50%; background:var(--color-primary); color:white; margin-right:5px; }
  .picker-label small { text-align:right; }
  .provider-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(min(240px,100%),1fr)); gap:8px; }
  .provider-option { min-width:0; min-height:68px; display:flex; align-items:center; gap:10px; padding:10px 11px; text-align:left; color:var(--color-fg-1); border:1px solid var(--color-border); border-radius:9px; background:var(--color-bg-card); cursor:pointer; transition:var(--transition); }
  .provider-option:hover { border-color:var(--color-primary); }
  .provider-option.selected { border:2px solid var(--color-primary); padding:9px 10px; background:var(--color-primary-light); box-shadow:0 0 0 3px var(--color-primary-glow); }
  .provider-mark { width:34px; height:34px; flex:0 0 34px; display:grid; place-items:center; border-radius:8px; background:var(--color-primary-light); color:var(--color-primary); }
  .provider-copy { min-width:0; flex:1; display:flex; flex-direction:column; }
  .provider-copy strong { color:var(--color-fg-0); font-size:13px; line-height:1.25; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .provider-copy small { margin-top:2px; color:var(--color-fg-3); font-size:10px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .provider-copy>span { margin-top:5px; color:var(--color-fg-2); font-size:10px; font-weight:600; }
  .selected-label,.model-options button>span { display:inline-flex; align-items:center; gap:3px; color:var(--color-primary); font-size:10px; font-weight:750; }
  .model-picker-panel { margin-top:12px; padding:12px; border:1px solid var(--color-border); border-radius:10px; background:var(--color-bg-card); }
  .model-search { min-height:44px; display:flex; align-items:center; gap:8px; padding:0 12px; border:1px solid var(--color-border); border-radius:8px; color:var(--color-fg-3); background:var(--color-bg-body); }
  .model-search:focus-within { border-color:var(--color-primary); box-shadow:0 0 0 3px var(--color-primary-glow); }
  .model-search input { min-width:0; width:100%; border:0; outline:0; background:transparent; color:var(--color-fg-0); font-size:12px; }
  .model-options { max-height:220px; margin-top:7px; padding:4px; overflow:auto; border:1px solid var(--color-border); border-radius:8px; background:var(--color-bg-body); }
  .model-options button { width:100%; min-height:44px; display:flex; align-items:center; justify-content:space-between; gap:10px; padding:8px 10px; border:1px solid transparent; border-radius:6px; background:transparent; color:var(--color-fg-1); cursor:pointer; text-align:left; }
  .model-options button:hover,.model-options button.selected { background:var(--color-primary-light); border-color:color-mix(in srgb,var(--color-primary) 35%,transparent); }
  .model-options code { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:11px; }
  .model-empty { padding:18px; text-align:center; color:var(--color-fg-3); font-size:11px; }
  .entry-actions { display:flex; gap:8px; flex-wrap:wrap; margin-top:10px; }
  .entry-actions button { min-height:44px; display:inline-flex; align-items:center; gap:5px; }
  .catalog-state { min-width:0; display:flex; align-items:center; gap:10px; padding:14px; border:1px dashed var(--color-border); border-radius:8px; font-size:12px; color:var(--color-fg-2); }
  .catalog-state>span { min-width:0; flex:1; }
  .catalog-state strong { margin-bottom:2px; color:var(--color-fg-0); }
  .catalog-state.error { border-color:var(--color-error); color:var(--color-error); background:var(--color-error-light); }
  .catalog-state.warning { border-color:color-mix(in srgb,var(--color-warning) 50%,var(--color-border)); background:color-mix(in srgb,var(--color-warning) 9%,var(--color-bg-card)); }
  .catalog-state.no-models { margin-top:4px; }
  .catalog-state button { min-height:44px; margin-left:auto; }
  .advanced-toggle { min-height:44px; margin-top:8px; border:0; background:transparent; color:var(--color-primary); font-size:11px; font-weight:650; cursor:pointer; padding:8px 0; }
  .advanced-row { display:grid; grid-template-columns:minmax(180px,1fr) minmax(180px,1fr) auto; gap:8px; align-items:end; margin-top:4px; }
  .advanced-row label { min-width:0; display:flex; flex-direction:column; gap:5px; color:var(--color-fg-2); font-size:10px; font-weight:650; }
  .advanced-row button { min-height:44px; }
  .advanced-row small { grid-column:1/-1; }
  .draft-summary { margin-top:12px; padding:11px; border:1px solid color-mix(in srgb,var(--color-primary) 30%,var(--color-border)); border-radius:10px; background:color-mix(in srgb,var(--color-primary) 5%,var(--color-bg-card)); }
  .summary-heading { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; }
  .summary-heading div,.summary-heading strong,.summary-heading span { display:block; }
  .summary-heading strong { color:var(--color-fg-0); font-size:12px; }
  .summary-heading div>span { color:var(--color-fg-3); font-size:10px; margin-top:2px; }
  .summary-heading>span { padding:3px 7px; border-radius:999px; background:var(--color-primary-light); color:var(--color-primary); font-size:10px; font-weight:700; }
  .draft-chain { list-style:none; margin:9px 0 0; padding:0; display:flex; flex-direction:column; gap:6px; }
  .draft-chain li { min-width:0; display:flex; align-items:center; gap:9px; padding:8px 10px; border:1px solid var(--color-border); border-radius:8px; background:var(--color-bg-card); }
  .summary-route { min-width:0; flex:1; display:flex; align-items:center; gap:7px; flex-wrap:wrap; }
  .summary-route strong { color:var(--color-fg-0); font-size:11px; }
  .summary-route small { margin:0; }
  .summary-route code { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:11px; }
  .route-arrow { color:var(--color-fg-3); }
  .spinning { animation:spin .8s linear infinite; }
  .sr-only { position:absolute; width:1px; height:1px; padding:0; margin:-1px; overflow:hidden; clip:rect(0,0,0,0); white-space:nowrap; border:0; }
  .provider-option:focus-visible,.model-options button:focus-visible,.advanced-toggle:focus-visible,.entry-actions button:focus-visible { outline:2px solid var(--color-primary); outline-offset:2px; }
  @keyframes spin { to { transform:rotate(360deg); } }
  @media (max-width: 760px) {
    .picker-label { flex-direction:column; gap:2px; }
    .picker-label small { text-align:left; }
    .provider-grid { grid-template-columns:1fr; }
    .entry-actions { align-items:stretch; }
    .entry-actions button { flex:1; justify-content:center; }
    .advanced-row { grid-template-columns:1fr; }
    .summary-route { display:grid; grid-template-columns:auto 14px minmax(0,1fr); }
    .summary-route .route-arrow:nth-of-type(2) { display:none; }
    .summary-route code { grid-column:1/-1; padding-left:21px; }
  }
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
