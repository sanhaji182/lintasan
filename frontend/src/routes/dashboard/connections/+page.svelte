<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import StatusBadge from '$lib/components/StatusBadge.svelte';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { showToast } from '$lib/toast';
  import { OAUTH_IDE_PRESETS, type OAuthIdePreset } from '$lib/oauthIdePresets';
  import { brandForProvider, logoPaths } from '$lib/oauthIdeBrands';
  import { Link2, Plus, TestTube2, RefreshCw, Trash2, ToggleLeft, ToggleRight, X, Search, Check, Sparkles, Settings, Edit2, Pencil, Save, FolderTree, Eye, EyeOff, Copy, Box, Cpu, ShieldAlert, Layers, ChevronRight } from 'lucide-svelte';

  let connections = $state<any[]>([]);
  let loading = $state(true);
  let error = $state('');
  let showForm = $state(false);
  let testing = $state<string | null>(null);
  let syncing = $state<string | null>(null);
  let presetSearch = $state('');

  // Curl import state
  let showCurlImport = $state(false);
  let curlText = $state('');
  let curlImporting = $state(false);
  let curlResult = $state<{success: boolean; id?: string; name?: string; base_url?: string; discovery?: any; error?: string} | null>(null);

  // Preset management state
  let presets = $state<any[]>([]);
  let presetsLoading = $state(true);
  let showManageModal = $state(false);
  let manageTab = $state<'presets' | 'categories'>('presets');
  let editingPreset = $state<any | null>(null);
  let showEditForm = $state(false);
  let presetForm = $state({ name: '', domain: '', base_url: '', format: 'openai', key_label: 'API Key', category: 'foundation' });
  let savingPreset = $state(false);

  // Category management state
  let categories = $state<any[]>([]);
  let categoriesLoading = $state(true);
  let editingCategory = $state<any | null>(null);
  let showCategoryForm = $state(false);
  let categoryForm = $state({ key: '', label: '', icon: '📦', color: '#8b5cf6', sort_order: 100 });
  let savingCategory = $state(false);

  // Models viewer state
  let viewingModelsOf = $state<any | null>(null); // connection being inspected
  let modelsList = $state<any[]>([]);
  let modelsLoading = $state(false);
  let modelsSearch = $state('');
  let modelSyncing = $state(false);
  let togglingModelId = $state<string | null>(null);

  let oauthIdeEnabled = $state(false);
  let oauthSessions = $state<{ provider: string; status: string }[]>([]);
  let wiringOAuth = $state('');
  let oauthExpanded = $state(false);
  let presetsExpanded = $state(false);

  // Connection search/filter state
  let searchQuery = $state('');
  let openMenuConnId = $state<string | null>(null);

  // Pool editing state
  let poolEditText = $state('');

  const filteredConnections = $derived.by(() => {
    if (!searchQuery.trim()) return connections;
    const q = searchQuery.toLowerCase();
    return connections.filter(c =>
      c.name?.toLowerCase().includes(q) ||
      c.format?.toLowerCase().includes(q) ||
      c.pool_id?.toLowerCase().includes(q)
    );
  });
  // Sort state for connections table
  type SortKey = 'name' | 'format' | 'priority' | 'pool' | 'models';
  let sortKey = $state<SortKey>('priority');
  let sortAsc = $state(false); // default: highest priority first

  function toggleSort(key: SortKey) {
    if (sortKey === key) { sortAsc = !sortAsc; } else { sortKey = key; sortAsc = true; }
  }

  const sortedConnections = $derived.by(() => {
    const list = [...filteredConnections];
    list.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case 'name': cmp = (a.name || '').localeCompare(b.name || ''); break;
        case 'format': cmp = (a.format || '').localeCompare(b.format || ''); break;
        case 'priority': cmp = (a.priority || 0) - (b.priority || 0); break;
        case 'pool': cmp = (a.pool_id || '').localeCompare(b.pool_id || 'zzz'); break;
        case 'models': cmp = (a.models_count || 0) - (b.models_count || 0); break;
      }
      return sortAsc ? cmp : -cmp;
    });
    return list;
  });

  // Group connections by provider (base_url domain or pool_id)
  let collapsedGroups = $state<Set<string>>(new Set());

  function extractProvider(url: string): string {
    try {
      const host = new URL(url).hostname.replace(/^www\./, '');
      // e.g. "api.commandcode.ai" → "commandcode", "api.deepseek.com" → "deepseek"
      const parts = host.split('.');
      if (parts.length >= 3 && parts[0] === 'api') return parts[1];
      if (parts.length >= 2) return parts[parts.length - 2];
      return host;
    } catch { return url; }
  }

  function providerDisplayName(key: string, conns: any[]): string {
    // Try to get a clean name: if all names share a prefix, use that
    const names = conns.map(c => c.name || '');
    if (names.length === 1) return names[0];
    // Find common prefix (e.g., "CommandCode" from "CommandCode (sanhaji)", "CommandCode (alex)")
    const first = names[0];
    let prefix = first;
    for (const n of names) {
      while (!n.startsWith(prefix) && prefix.length > 0) {
        prefix = prefix.slice(0, -1);
      }
    }
    prefix = prefix.replace(/[\s(-_]+$/, '').trim();
    return prefix || key.charAt(0).toUpperCase() + key.slice(1);
  }

  const groupedConnections = $derived.by(() => {
    const groups = new Map<string, { key: string; label: string; connections: any[]; active: number; totalModels: number }>();
    for (const conn of sortedConnections) {
      const key = conn.pool_id || extractProvider(conn.base_url || '');
      if (!groups.has(key)) {
        groups.set(key, { key, label: '', connections: [], active: 0, totalModels: 0 });
      }
      const g = groups.get(key)!;
      g.connections.push(conn);
      if (conn.is_active) g.active++;
      g.totalModels += conn.models_count || 0;
    }
    // Set labels after grouping
    for (const g of groups.values()) {
      g.label = providerDisplayName(g.key, g.connections);
    }
    return [...groups.values()];
  });

  function toggleGroup(key: string) {
    const s = new Set(collapsedGroups);
    s.has(key) ? s.delete(key) : s.add(key);
    collapsedGroups = s;
  }

  // Close dropdown menu on outside click
  $effect(() => {
    if (!openMenuConnId) return;
    function handleClick(e: MouseEvent) {
      const target = e.target as HTMLElement;
      if (!target.closest('.kebab-menu-container')) {
        openMenuConnId = null;
      }
    }
    document.addEventListener('click', handleClick, true);
    return () => document.removeEventListener('click', handleClick, true);
  });

  // Pool management state
  let pools = $state<{
    pool_id: string; num_accounts: number;
    success_count: number; fail_count: number;
    total_requests: number; success_rate: number;
    rate_limited: number; available_count: number;
  }[]>([]);
  let poolsLoading = $state(true);
  let editingPool = $state<string | null>(null); // connection id being pool-edited
  let reassigningPool = $state<string | null>(null); // connection id being reassigned
  let poolReassignTarget = $state(''); // target pool_id for reassign

  let form = $state({ name: '', base_url: '', api_key: '', format: 'openai', priority: 1, oauth_provider: '' as string, pool_id: '' as string });

  // Test-before-save state for the new-connection form
  let testingForm = $state(false);
  // Rich result envelope: backend returns {data, error, hint, latency_ms, success, message}
  // plus the error object with {message, type, code, param}. We surface all of it in the UI.
  let testResult = $state<{
    success: boolean;
    message?: string;
    error?: { message: string; type: string; code: string; param?: string };
    hint?: string;
    latency_ms?: number;
    models_count?: number;
    fallback?: string;
  } | null>(null);
  let testBounce = $state(false); // triggers bounce animation on Test button

  const summary = $derived({
    total: connections.length,
    active: connections.filter(c => c.is_active).length,
    formats: [...new Set(connections.map(c => c.format))].length,
    pools: pools.length
  });

  // Categories as a key→{label,icon,color} map (derived from API state)
  const categoryMap = $derived(
    categories.reduce((acc, c) => {
      acc[c.key] = { label: c.label, icon: c.icon, color: c.color, is_builtin: c.is_builtin, sort_order: c.sort_order };
      return acc;
    }, {} as Record<string, any>)
  );

  // Get connected preset names
  const connectedNames = $derived(new Set(connections.map(c => c.name?.toLowerCase())));

  const filteredPresets = $derived(
    presetSearch
      ? presets.filter((p: any) =>
          p.name.toLowerCase().includes(presetSearch.toLowerCase()) ||
          p.domain.toLowerCase().includes(presetSearch.toLowerCase())
        )
      : presets
  );

  const groupedPresets = $derived(
    categories.reduce((acc, cat) => {
      acc[cat.key] = filteredPresets.filter((p: any) => p.category === cat.key);
      return acc;
    }, {} as Record<string, any[]>)
  );

  const activeCategories = $derived(
    categories
      .map(c => ({ ...c, count: groupedPresets[c.key]?.length || 0 }))
      .filter(c => c.count > 0)
      .sort((a, b) => a.sort_order - b.sort_order)
  );

  const categoryPresetCounts = $derived(
    categories.reduce((acc, c) => {
      acc[c.key] = presets.filter(p => p.category === c.key).length;
      return acc;
    }, {} as Record<string, number>)
  );

  function faviconUrl(domain: string, size = 32) {
    return `https://www.google.com/s2/favicons?domain=${domain}&sz=${size}`;
  }

  function pickOAuthIdePreset(p: OAuthIdePreset) {
    form = {
      name: p.name,
      base_url: p.base_url,
      api_key: '',
      format: p.format,
      priority: 5,
      oauth_provider: p.oauth_provider,
      pool_id: ''
    };
    testResult = null;
    showForm = true;
    setTimeout(() => {
      document.getElementById('add-connection-form')?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 50);
  }

  const formUsesOAuth = $derived(!!form.oauth_provider?.trim());

  function oauthSessionActive(provider: string) {
    return oauthSessions.some((s) => s.provider === provider && s.status === 'active');
  }

  async function fetchOAuthLab() {
    try {
      const st = await api.get<{ enabled?: boolean }>('/api/oauth/status');
      oauthIdeEnabled = !!st.enabled;
      if (st.enabled) {
        const sess = await api.get<{ data: { provider: string; status: string }[] }>('/api/oauth/sessions');
        oauthSessions = sess.data ?? [];
      } else {
        oauthSessions = [];
      }
    } catch {
      oauthIdeEnabled = false;
      oauthSessions = [];
    }
  }

  async function quickWireOAuth(provider: string) {
    wiringOAuth = provider;
    try {
      const res = await api.post<any>('/api/oauth/provision-connection', { provider });
      showToast(`${res.action === 'created' ? 'Wired' : 'Updated'} ${res.name}`, 'success');
      await fetchConnections();
    } catch (e: any) {
      showToast(e?.message || 'Wire failed', 'error');
    } finally {
      wiringOAuth = '';
    }
  }

  function pickPreset(preset: any) {
    form = {
      name: preset.name.toLowerCase().replace(/\s+/g, '-'),
      base_url: preset.base_url,
      api_key: '',
      format: preset.format,
      priority: 1,
      oauth_provider: '',
      pool_id: ''
    };
    testResult = null; // reset prior test result when picking a new preset
    showForm = true;
    setTimeout(() => {
      const formEl = document.getElementById('add-connection-form');
      formEl?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 50);
  }

  async function fetchCategories() {
    categoriesLoading = true;
    try {
      const res = await api.get<any>('/api/preset-categories');
      const data = res.data || [];
      if (data.length === 0) {
        await api.post<any>('/api/preset-categories/seed', {});
        const res2 = await api.get<any>('/api/preset-categories');
        categories = res2.data || [];
      } else {
        categories = data;
      }
    } catch (e: any) {
      categories = [];
    } finally {
      categoriesLoading = false;
    }
  }

  async function fetchPresets() {
    presetsLoading = true;
    try {
      const res = await api.get<any>('/api/presets');
      const data = res.data || [];
      if (data.length === 0) {
        await api.post<any>('/api/presets/seed', {});
        const res2 = await api.get<any>('/api/presets');
        presets = res2.data || [];
      } else {
        presets = data;
      }
    } catch (e: any) {
      presets = [];
    } finally {
      presetsLoading = false;
    }
  }

  // Preset CRUD
  function startNewPreset() {
    editingPreset = null;
    presetForm = { name: '', domain: '', base_url: '', format: 'openai', key_label: 'API Key', category: categories[0]?.key || 'foundation' };
    showEditForm = true;
  }

  function startEditPreset(preset: any) {
    if (preset.is_builtin === 1) {
      showToast('Built-in presets are read-only', 'error');
      return;
    }
    editingPreset = preset;
    presetForm = {
      name: preset.name,
      domain: preset.domain,
      base_url: preset.base_url,
      format: preset.format,
      key_label: preset.key_label,
      category: preset.category
    };
    showEditForm = true;
  }

  async function savePreset() {
    if (!presetForm.name || !presetForm.domain || !presetForm.base_url) {
      showToast('Name, domain, and base URL are required', 'error');
      return;
    }
    savingPreset = true;
    try {
      if (editingPreset) {
        await api.put<any>(`/api/presets/${editingPreset.id}`, presetForm);
        showToast('Preset updated', 'success');
      } else {
        await api.post<any>('/api/presets', presetForm);
        showToast('Preset added', 'success');
      }
      showEditForm = false;
      await fetchPresets();
    } catch (e: any) {
      showToast('Failed to save: ' + e.message, 'error');
    } finally {
      savingPreset = false;
    }
  }

  async function deletePreset(preset: any) {
    if (preset.is_builtin === 1) {
      showToast('Built-in presets cannot be deleted', 'error');
      return;
    }
    if (!confirm(`Delete preset "${preset.name}"?`)) return;
    try {
      await api.delete<any>(`/api/presets/${preset.id}`);
      showToast('Preset deleted', 'success');
      await fetchPresets();
    } catch (e: any) {
      showToast('Failed to delete: ' + e.message, 'error');
    }
  }

  // Category CRUD
  function startNewCategory() {
    editingCategory = null;
    categoryForm = { key: '', label: '', icon: '📦', color: '#8b5cf6', sort_order: (categories.length + 1) * 10 };
    showCategoryForm = true;
  }

  function startEditCategory(cat: any) {
    editingCategory = cat;
    categoryForm = {
      key: cat.key,
      label: cat.label,
      icon: cat.icon,
      color: cat.color,
      sort_order: cat.sort_order
    };
    showCategoryForm = true;
  }

  function slugifyKey(s: string) {
    return s.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 31);
  }

  async function saveCategory() {
    if (!categoryForm.label) {
      showToast('Label is required', 'error');
      return;
    }
    if (!editingCategory && !categoryForm.key) {
      categoryForm.key = slugifyKey(categoryForm.label);
    }
    if (!editingCategory && !categoryForm.key) {
      showToast('Key is required (use letters, numbers, hyphens)', 'error');
      return;
    }
    savingCategory = true;
    try {
      if (editingCategory) {
        await api.put<any>(`/api/preset-categories/${editingCategory.key}`, {
          label: categoryForm.label,
          icon: categoryForm.icon,
          color: categoryForm.color,
          sort_order: categoryForm.sort_order
        });
        showToast('Category updated', 'success');
      } else {
        await api.post<any>('/api/preset-categories', categoryForm);
        showToast('Category added', 'success');
      }
      showCategoryForm = false;
      await fetchCategories();
    } catch (e: any) {
      showToast('Failed to save: ' + e.message, 'error');
    } finally {
      savingCategory = false;
    }
  }

  async function deleteCategory(cat: any) {
    if (cat.is_builtin === 1) {
      showToast('Built-in categories cannot be deleted', 'error');
      return;
    }
    if (categoryPresetCounts[cat.key] > 0) {
      showToast(`Cannot delete: ${categoryPresetCounts[cat.key]} preset(s) still use this category`, 'error');
      return;
    }
    if (!confirm(`Delete category "${cat.label}"?`)) return;
    try {
      await api.delete<any>(`/api/preset-categories/${cat.key}`);
      showToast('Category deleted', 'success');
      await fetchCategories();
    } catch (e: any) {
      showToast('Failed to delete: ' + e.message, 'error');
    }
  }

  onMount(async () => {
    await Promise.all([fetchPresets(), fetchConnections(), fetchCategories(), fetchOAuthLab(), fetchPools()]);
  });

  async function fetchConnections() {
    loading = true;
    try {
      const res = await api.get<any>('/api/connections');
      connections = res.data || [];
    } catch (e: any) { error = e.message; }
    finally { loading = false; }
  }

  async function fetchPools() {
    poolsLoading = true;
    try {
      const res = await api.get<any>('/api/connections/pools');
      pools = (res.data || []).map((p: any) => ({
        pool_id: p.pool_id, num_accounts: p.num_accounts,
        success_count: p.success_count || 0, fail_count: p.fail_count || 0,
        total_requests: p.total_requests || 0, success_rate: p.success_rate || 0,
        rate_limited: p.rate_limited || 0, available_count: p.available_count || 0
      }));
    } catch {
      pools = [];
    }
    poolsLoading = false;
  }

  async function setPoolId(connId: string, poolId: string) {
    try {
      await api.patch('/api/connections', { id: connId, pool_id: poolId });
      connections = connections.map(c => c.id === connId ? { ...c, pool_id: poolId } : c);
      editingPool = null;
      await fetchPools();
    } catch (e: any) {
      showToast('Failed to update pool: ' + e.message, 'error');
    }
  }

  async function toggleActive(conn: any) {
    try {
      await api.patch('/api/connections', { id: conn.id, is_active: !conn.is_active });
      connections = connections.map(c => c.id === conn.id ? { ...c, is_active: !c.is_active } : c);
    } catch (e: any) { error = e.message; }
  }

  async function testConn(id: string) {
    testing = id;
    try {
      const res = await api.post<any>('/api/connections/test', { id });
      if (res.success) {
        const msg = res.models_count != null
          ? `Connection OK · ${res.models_count} model(s) · ${res.latency_ms ?? 0}ms`
          : (res.message || 'Connection OK');
        showToast(msg, 'success', 3000, res.fallback ? { message: `via ${res.fallback}`, code: 'reachable' } : undefined);
      } else {
        // Pass the full envelope to the rich toast renderer
        const err = res.error && typeof res.error === 'object' ? res.error : { message: String(res.error || 'unknown'), type: 'unknown', code: 'unknown' };
        showToast('Connection test failed', 'error', 6000, {
          code: err.code,
          type: err.type,
          param: err.param,
          message: err.message,
          hint: res.hint,
        });
      }
    } catch (e: any) {
      // The api wrapper throws a rich Error with .envelope/.detail when the
      // server returned a non-2xx envelope. Only treat it as a network error
      // when there's no envelope (true transport failure).
      const env = e?.envelope;
      if (env && env.error) {
        const err = (typeof env.error === 'object') ? env.error : { message: String(env.error), type: 'unknown', code: 'unknown' };
        showToast('Connection test failed', 'error', 6000, {
          code: err.code, type: err.type, param: err.param,
          message: err.message, hint: env.hint,
        });
      } else {
        showToast('Test request failed', 'error', 6000, {
          code: 'request_failed', type: 'network_error',
          message: e.message || 'unknown',
        });
      }
    } finally { testing = null; }
  }

  async function syncModels(id: string) {
    syncing = id;
    try {
      await api.post('/api/models/sync/' + id);
      await fetchConnections();
    } catch (e: any) { error = e.message; }
    finally { syncing = null; }
  }

  // --- Models viewer (per-connection modal) ---
  async function openModelsViewer(conn: any) {
    viewingModelsOf = conn;
    modelsList = [];
    modelsSearch = '';
    // If connection is part of a group, load models from ALL connections in the group
    const group = groupedConnections.find(g => g.connections.some(c => c.id === conn.id));
    if (group && group.connections.length > 1) {
      await loadModelsForGroup(group.connections.map(c => c.id));
    } else {
      await loadModelsForConnection(conn.id);
    }
  }

  function closeModelsViewer() {
    viewingModelsOf = null;
    modelsList = [];
    modelsSearch = '';
    modelSyncing = false;
  }

  async function loadModelsForGroup(connIds: string[]) {
    modelsLoading = true;
    try {
      // Load models from all connections in parallel, merge and deduplicate
      const results = await Promise.allSettled(
        connIds.map(id => api.get<any>(`/api/models/discovered?connection_id=${encodeURIComponent(id)}`))
      );
      const seen = new Map<string, any>();
      for (const r of results) {
        if (r.status === 'fulfilled') {
          const models = r.value?.data || [];
          for (const m of models) {
            if (!seen.has(m.model_id)) {
              seen.set(m.model_id, m);
            } else {
              // Keep the one that's active, or the newer one
              const existing = seen.get(m.model_id);
              if (m.is_active === 1 && existing.is_active !== 1) {
                seen.set(m.model_id, m);
              }
            }
          }
        }
      }
      modelsList = [...seen.values()];
    } catch (e: any) {
      modelsList = [];
      showToast('Failed to load models: ' + (e.message || 'unknown'), 'error');
    } finally {
      modelsLoading = false;
    }
  }

  async function loadModelsForConnection(connId: string) {
    modelsLoading = true;
    try {
      const res = await api.get<any>(`/api/models/discovered?connection_id=${encodeURIComponent(connId)}`);
      modelsList = res.data || [];
    } catch (e: any) {
      // Fall back to the sync endpoint shape (sometimes the GET handler returns the same data)
      try {
        const res2 = await api.get<any>(`/api/models/sync?connection_id=${encodeURIComponent(connId)}`);
        modelsList = res2.data || [];
      } catch (e2: any) {
        modelsList = [];
        showToast('Failed to load models: ' + (e.message || 'unknown'), 'error');
      }
    } finally {
      modelsLoading = false;
    }
  }

  async function syncModelsInViewer(connId: string) {
    modelSyncing = true;
    try {
      // Find all connections in the same group
      const group = groupedConnections.find(g => g.connections.some(c => c.id === connId));
      const connIds = (group && group.connections.length > 1)
        ? group.connections.filter(c => c.is_active).map(c => c.id)
        : [connId];

      // Sync all connections in the group (parallel)
      const results = await Promise.allSettled(
        connIds.map(id => api.post<any>('/api/models/sync/' + id, {}))
      );
      const totalSynced = results.reduce((sum, r) => {
        if (r.status === 'fulfilled') {
          return sum + (r.value?.synced ?? r.value?.data?.models_count ?? 0);
        }
        return sum;
      }, 0);

      // Refresh: viewer list (from group) + connection card counts
      if (group && group.connections.length > 1) {
        await Promise.all([loadModelsForGroup(group.connections.map(c => c.id)), fetchConnections()]);
      } else {
        await Promise.all([loadModelsForConnection(connId), fetchConnections()]);
      }

      const syncedCount = results.filter(r => r.status === 'fulfilled').length;
      if (connIds.length > 1) {
        showToast(`Synced ${totalSynced || 'models'} across ${syncedCount}/${connIds.length} accounts`, 'success');
      } else if (totalSynced > 0) {
        showToast(`Synced ${totalSynced} model(s)`, 'success');
      } else {
        showToast('Sync complete', 'success');
      }
    } catch (e: any) {
      // The sync endpoint requires the connection to be active. Show a helpful hint.
      const env = e?.envelope;
      const msg = env?.error?.message || e.message || 'unknown';
      showToast('Sync failed: ' + msg, 'error', 5000, {
        code: env?.error?.code || 'sync_failed',
        type: env?.error?.type || 'sync_error',
        message: msg,
        hint: env?.hint || 'connection must be active to sync models',
      });
    } finally {
      modelSyncing = false;
    }
  }

  async function toggleModelActive(connId: string, modelId: string, currentActive: number) {
    togglingModelId = modelId;
    // Optimistic update
    const next = currentActive === 1 ? 0 : 1;
    modelsList = modelsList.map(m =>
      m.model_id === modelId ? { ...m, is_active: next } : m
    );
    try {
      await api.post<any>('/api/models/sync/' + connId, {
        action: 'toggle',
        modelId,
        active: next === 1
      });
    } catch (e: any) {
      // Revert
      modelsList = modelsList.map(m =>
        m.model_id === modelId ? { ...m, is_active: currentActive } : m
      );
      showToast('Failed to toggle model: ' + (e.message || 'unknown'), 'error');
    } finally {
      togglingModelId = null;
    }
  }

  async function copyModelId(modelId: string) {
    try {
      await navigator.clipboard.writeText(modelId);
      showToast(`Copied: ${modelId}`, 'success', 2000);
    } catch {
      showToast('Copy failed (browser blocked clipboard)', 'error');
    }
  }

  const filteredModels = $derived(
    modelsSearch
      ? modelsList.filter((m: any) =>
          m.model_id?.toLowerCase().includes(modelsSearch.toLowerCase()) ||
          m.model_name?.toLowerCase().includes(modelsSearch.toLowerCase()) ||
          m.owned_by?.toLowerCase().includes(modelsSearch.toLowerCase())
        )
      : modelsList
  );

  const modelsStats = $derived({
    total: modelsList.length,
    active: modelsList.filter((m: any) => m.is_active === 1).length,
    inactive: modelsList.filter((m: any) => m.is_active !== 1).length,
    owners: new Set(modelsList.map((m: any) => m.owned_by).filter(Boolean)).size
  });

  async function deleteConn(id: string) {
    if (!confirm('Delete this connection?')) return;
    try {
      await api.delete('/api/connections/' + id);
      connections = connections.filter(c => c.id !== id);
    } catch (e: any) { error = e.message; }
  }

  async function testNewConnection() {
    if (!form.base_url) {
      showToast('Base URL is required to test', 'error');
      return;
    }
    // Bounce animation: trigger and clear after animation completes
    testBounce = true;
    setTimeout(() => { testBounce = false; }, 600);
    testingForm = true;
    testResult = null;
    try {
      const res = await api.post<any>('/api/connections/test', {
        base_url: form.base_url,
        api_key: formUsesOAuth ? '' : form.api_key,
        format: form.format,
        oauth_provider: form.oauth_provider || undefined
      });
      // Capture the full standard envelope
      testResult = {
        success: !!res.success,
        message: res.message,
        error: res.error && typeof res.error === 'object' ? res.error : (res.error ? { message: String(res.error), type: 'unknown', code: 'unknown' } : undefined),
        hint: res.hint,
        latency_ms: res.latency_ms,
        models_count: res.models_count,
        fallback: res.fallback
      };
      if (res.success) {
        showToast(res.message || 'Connection test passed!', 'success');
        // Auto-fill name from URL host if user hasn't typed one
        if (!form.name) {
          try {
            const u = new URL(form.base_url);
            form.name = u.hostname.replace(/^api\./, '').replace(/\./g, '-');
          } catch { /* ignore */ }
        }
      } else {
        // Toast gets a compact summary; the inline banner shows the full details
        const e = testResult.error;
        const summary = e ? `${e.code || 'error'}: ${(e.message || '').slice(0, 80)}` : 'Test failed';
        showToast(summary, 'error');
      }
    } catch (e: any) {
      // The api wrapper throws a rich Error with .envelope (full body) and .detail (error obj).
      // When the request itself failed (network/CORS), .envelope will be undefined.
      const env = e?.envelope;
      if (env && env.error) {
        const err = (typeof env.error === 'object') ? env.error : { message: String(env.error), type: 'unknown', code: 'unknown' };
        testResult = {
          success: false,
          error: err,
          hint: env.hint,
          latency_ms: env.latency_ms,
        };
        const summary = `${err.code || 'error'}: ${(err.message || '').slice(0, 80)}`;
        showToast(summary, 'error');
      } else {
        // Real network/transport failure (CORS, DNS, offline, etc.)
        testResult = {
          success: false,
          error: { message: e.message || 'Test request failed', type: 'network_error', code: 'request_failed' }
        };
        showToast('Test request failed: ' + (e.message || 'unknown'), 'error');
      }
    } finally {
      testingForm = false;
    }
  }

  async function createConn() {
    // Save is blocked until test passes. Clicking the disabled-looking Save
    // button triggers a bounce on the Test button to draw the user's eye.
    if (!testResult?.success) {
      // Trigger bounce on Test button
      testBounce = true;
      setTimeout(() => { testBounce = false; }, 600);
      return;
    }
    try {
      await api.post<any>('/api/connections', form);
      await fetchConnections();
      showForm = false;
      form = { name: '', base_url: '', api_key: '', format: 'openai', priority: 1, oauth_provider: '', pool_id: '' };
      testResult = null;
    } catch (e: any) { error = e.message; }
  }

  function cancelForm() {
    showForm = false;
    form = { name: '', base_url: '', api_key: '', format: 'openai', priority: 1, oauth_provider: '', pool_id: '' };
    testResult = null;
    testingForm = false;
    testBounce = false;
  }

  // --- Curl Import ---
  async function importFromCurl() {
    if (!curlText.trim()) return;
    curlImporting = true;
    curlResult = null;
    try {
      const res = await api.post<any>('/api/connections/import-curl', { curl: curlText.trim() });
      const d = res.data || res;
      curlResult = { success: true, ...d };
      showToast(`Imported: ${d.name} · ${d.discovery?.models_count ?? 0} models`, 'success');
      await fetchConnections();
      showCurlImport = false;
      curlText = '';
    } catch (e: any) {
      const msg = e?.envelope?.error || e.message || 'Import failed';
      curlResult = { success: false, error: msg };
      showToast('Curl import failed: ' + msg, 'error');
    } finally {
      curlImporting = false;
    }
  }

  function openManage(tab: 'presets' | 'categories') {
    manageTab = tab;
    showManageModal = true;
  }
</script>

<div style="animation: fadeInUp 0.4s ease-out;">
  <!-- Summary strip -->
  <div class="card mb-5" style="padding: 0; overflow: hidden;">
    <div class="grid grid-cols-4" style="gap: 1px; background: var(--color-border);">
      {#each [
        { label: 'TOTAL', value: summary.total },
        { label: 'ACTIVE', value: summary.active },
        { label: 'FORMATS', value: summary.formats },
        { label: 'POOLS', value: summary.pools }
      ] as stat}
        <div class="text-center" style="padding: 16px 20px; background: var(--color-bg-card);">
          <div style="font-size: 11px; font-weight: 500; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 4px;">{stat.label}</div>
          <div style="font-size: 20px; font-weight: 700; color: var(--color-fg-0); font-family: var(--font-mono);">{stat.value}</div>
        </div>
      {/each}
      </div>
      </div>

  <!-- Toolbar: Title + Search + Actions -->
  <div class="conn-toolbar">
    <div class="conn-toolbar-left">
      <h2 class="conn-toolbar-title">Connections</h2>
      {#if searchQuery.trim()}
        <span class="conn-toolbar-count">{filteredConnections.length}/{connections.length}</span>
      {/if}
    </div>
    <div class="conn-toolbar-center">
      <div class="conn-search-wrap">
        <Search size={14} class="conn-search-icon" />
        <input
          class="input-field conn-search-input"
          placeholder="Search connections..."
          bind:value={searchQuery}
        />
        {#if searchQuery}
          <button class="conn-search-clear" onclick={() => searchQuery = ''} aria-label="Clear search">
            <X size={12} />
          </button>
        {/if}
      </div>
    </div>
    <div class="conn-toolbar-right">
      <button class="btn-secondary conn-toolbar-btn" onclick={() => { loading = true; fetchConnections(); fetchPools(); }} title="Refresh connections" aria-label="Refresh">
        <RefreshCw size={15} />
      </button>
      <button class="btn-secondary conn-toolbar-btn" onclick={() => { showCurlImport = !showCurlImport; curlResult = null; }} title="Import from curl">
        <Copy size={15} />
      </button>
      <button class="btn-primary conn-toolbar-btn" onclick={() => showForm = !showForm}>
        {#if showForm}<X size={15} />{:else}<Plus size={15} />{/if}
        <span class="conn-toolbar-btn-label">{showForm ? 'Cancel' : 'Add'}</span>
      </button>
    </div>
  </div>

  <!-- Create form -->
  {#if showForm}
    <div id="add-connection-form" class="card mb-5" style="animation: fadeInScale 0.3s ease-out;">
      <div class="flex items-center justify-between mb-4">
        <h3 style="font-size: 15px; font-weight: 600; color: var(--color-fg-0); margin: 0;">
          {form.name ? `New: ${form.name}` : 'New Connection'}
        </h3>
        <button class="btn-secondary" style="padding: 4px 10px; font-size: 12px;" onclick={cancelForm}>
          <X size={14} /> Cancel
        </button>
      </div>
      <div class="grid grid-cols-1 md:grid-cols-2" style="gap: 12px;">
        <div>
          <label for="connection-name" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Name</label>
          <input id="connection-name" class="input-field" bind:value={form.name} placeholder="My Provider" />
        </div>
        <div>
          <label for="connection-base-url" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Base URL</label>
          <input id="connection-base-url" class="input-field" bind:value={form.base_url} placeholder="https://api.openai.com/v1" />
        </div>
        <div>
          <label for="connection-api-key" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">
            {formUsesOAuth ? 'API Key (OAuth session)' : 'API Key'}
          </label>
          {#if formUsesOAuth}
            <div class="input-field" style="font-size: 12px; color: var(--color-fg-2); padding: 10px 12px; background: var(--color-bg-body);">
              Uses OAuth IDE session for <code>{form.oauth_provider}</code> — authorize on <a href="/dashboard/oauth-ide">OAuth IDE</a> first.
              <button type="button" class="btn-secondary" style="margin-left: 8px; padding: 2px 8px; font-size: 11px;" onclick={() => { form.oauth_provider = ''; }}>Clear OAuth</button>
            </div>
          {:else}
            <input id="connection-api-key" class="input-field" type="password" bind:value={form.api_key} placeholder="sk-..." />
          {/if}
        </div>
        <div>
          <label for="connection-format" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Format</label>
          <select id="connection-format" class="input-field" bind:value={form.format}>
            <option value="openai">OpenAI</option>
            <option value="anthropic">Anthropic</option>
            <option value="gemini">Gemini</option>
          </select>
        </div>
        <div>
          <label for="connection-pool-id" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">
            Pool ID <span style="font-size: 10px; color: var(--color-fg-3); font-weight: 400;">(optional — group multiple API keys for load balancing)</span>
          </label>
          <div style="display: flex; align-items: center; gap: 6px; background: var(--color-bg-card); border: 1px solid var(--color-border); border-radius: 8px; padding: 2px 2px 2px 10px;">
            <Layers size={14} style="color: var(--color-fg-3); flex-shrink: 0;" />
            <input id="connection-pool-id" class="input-field" bind:value={form.pool_id} placeholder="e.g. openai-prod" style="border: none; padding: 6px 4px; background: transparent; flex: 1; min-width: 0;" />
            {#if pools.length > 0}
              <select
                style="font-size: 11px; padding: 6px 8px; border: none; border-left: 1px solid var(--color-border); border-radius: 0; background: transparent; color: var(--color-fg-2); cursor: pointer; font-family: var(--font-mono); width: auto; flex-shrink: 0;"
                onchange={(e: Event) => { form.pool_id = (e.target as HTMLSelectElement).value; }}
              >
                <option value="">pick existing…</option>
                {#each pools as p}
                  <option value={p.pool_id}>{p.pool_id}</option>
                {/each}
              </select>
            {/if}
          </div>
        </div>
      </div>
      <!-- Test result banner — rich OpenAI-standard error display -->
      {#if testResult}
        <div style="margin-top: 12px; border-radius: 10px; overflow: hidden; border: 1px solid {testResult.success ? 'rgba(16,185,129,0.25)' : 'rgba(239,68,68,0.25)'}; background: {testResult.success ? 'rgba(16,185,129,0.04)' : 'rgba(239,68,68,0.04)'};">
          {#if testResult.success}
            <!-- Success header -->
            <div style="padding: 10px 12px; display: flex; align-items: center; gap: 10px;">
              <div style="width: 24px; height: 24px; border-radius: 50%; background: var(--color-success); display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                <Check size={14} color="white" />
              </div>
              <div style="flex: 1; min-width: 0;">
                <div style="font-size: 12px; font-weight: 600; color: var(--color-success);">Connection verified</div>
                <div style="font-size: 11px; color: var(--color-fg-2);">
                  {testResult.models_count ?? 0} model(s) available · {testResult.latency_ms ?? 0}ms{#if testResult.fallback} · via {testResult.fallback}{/if}
                </div>
              </div>
            </div>
          {:else}
            {@const err = testResult.error}
            <!-- Error header -->
            <div style="padding: 10px 12px; display: flex; align-items: center; gap: 10px; border-bottom: 1px solid rgba(239,68,68,0.15);">
              <div style="width: 24px; height: 24px; border-radius: 50%; background: var(--color-error, #ef4444); display: flex; align-items: center; justify-content: center; flex-shrink: 0; color: white; font-size: 14px; font-weight: 700;">×</div>
              <div style="flex: 1; min-width: 0;">
                <div style="font-size: 12px; font-weight: 600; color: var(--color-error, #ef4444);">Connection failed</div>
                <div style="font-size: 11px; color: var(--color-fg-2);">
                  {testResult.latency_ms ?? 0}ms{#if err?.code} · {err.code}{/if}
                </div>
              </div>
            </div>
            <!-- Error details -->
            {#if err}
              <div style="padding: 10px 12px; font-size: 12px; line-height: 1.5;">
                <!-- Type + code badges -->
                <div style="display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 8px;">
                  {#if err.code}
                    <span style="padding: 2px 8px; border-radius: 4px; font-size: 10px; font-weight: 600; font-family: ui-monospace, SFMono-Regular, monospace; background: rgba(239,68,68,0.12); color: #dc2626; border: 1px solid rgba(239,68,68,0.2);">
                      {err.code}
                    </span>
                  {/if}
                  {#if err.type}
                    <span style="padding: 2px 8px; border-radius: 4px; font-size: 10px; font-weight: 500; font-family: ui-monospace, SFMono-Regular, monospace; background: rgba(107,114,128,0.1); color: var(--color-fg-2); border: 1px solid var(--color-border);">
                      {err.type}
                    </span>
                  {/if}
                  {#if err.param}
                    <span style="padding: 2px 8px; border-radius: 4px; font-size: 10px; font-weight: 500; font-family: ui-monospace, SFMono-Regular, monospace; background: rgba(245,158,11,0.1); color: #d97706; border: 1px solid rgba(245,158,11,0.2);">
                      param: {err.param}
                    </span>
                  {/if}
                </div>
                <!-- Upstream message -->
                {#if err.message}
                  <div style="color: var(--color-fg); word-wrap: break-word; max-height: 80px; overflow-y: auto; padding: 6px 8px; background: rgba(0,0,0,0.03); border-radius: 4px; font-family: ui-monospace, SFMono-Regular, monospace; font-size: 11px;">
                    {err.message}
                  </div>
                {/if}
                <!-- Hint (actionable suggestion) -->
                {#if testResult.hint}
                  <div style="margin-top: 8px; display: flex; align-items: flex-start; gap: 6px; padding: 6px 8px; background: rgba(59,130,246,0.06); border-radius: 4px; border-left: 3px solid #3b82f6;">
                    <span style="font-size: 12px; line-height: 1.4;">💡</span>
                    <span style="font-size: 11px; color: var(--color-fg); line-height: 1.4;">{testResult.hint}</span>
                  </div>
                {/if}
              </div>
            {/if}
          {/if}
        </div>
      {/if}

      <div class="flex justify-end gap-2 mt-4">
        <button
          class="btn-secondary flex items-center gap-1 {testBounce ? 'bounce-highlight' : ''}"
          style="padding: 8px 14px; font-size: 12px;"
          onclick={testNewConnection}
          disabled={testingForm || !form.base_url}
        >
          {#if testingForm}
            <span class="inline-block" style="width: 12px; height: 12px; border: 2px solid var(--color-border); border-top-color: var(--color-primary); border-radius: 50%; animation: spin 0.8s linear infinite;"></span>
            Testing...
          {:else}
            <TestTube2 size={14} /> Test Connection
          {/if}
        </button>
        <button
          class="btn-primary {testResult?.success ? '' : 'btn-disabled-look'}"
          onclick={createConn}
          style={!testResult?.success ? 'opacity: 0.5; cursor: not-allowed;' : ''}
          title={!testResult?.success ? 'Click Test Connection first' : ''}
        >
          {testResult?.success ? 'Save Connection' : 'Save'}
        </button>
      </div>
    </div>
  {/if}

  <!-- Curl Import -->
  {#if showCurlImport}
    <div class="card mb-5" style="animation: fadeInScale 0.3s ease-out;">
      <div class="flex items-center justify-between mb-4">
        <div class="flex items-center gap-2">
          <Copy size={16} style="color: var(--color-primary);" />
          <h3 style="font-size: 15px; font-weight: 600; color: var(--color-fg-0); margin: 0;">Import from Curl</h3>
          <span style="font-size: 11px; color: var(--color-fg-3);">Paste a curl command from any provider docs</span>
        </div>
      </div>
      <textarea
        class="input-field"
        bind:value={curlText}
        placeholder="curl https://api.example.com/v1/chat/completions -H Authorization: Bearer *** -H Content-Type: application/json -d 'JSON_BODY'"
        style="width: 100%; min-height: 100px; font-family: ui-monospace, SFMono-Regular, monospace; font-size: 11px; resize: vertical;"
        rows="4"
      ></textarea>
      {#if curlResult?.success}
        <div style="margin-top: 10px; padding: 8px 12px; border-radius: 8px; background: rgba(16,185,129,0.06); border: 1px solid rgba(16,185,129,0.2);">
          <div style="font-size: 12px; font-weight: 600; color: var(--color-success);">✅ Imported: {curlResult.name}</div>
          <div style="font-size: 11px; color: var(--color-fg-2); margin-top: 2px;">
            Base URL: {curlResult.base_url}{#if curlResult.discovery} · {curlResult.discovery.models_count} models discovered{/if}
          </div>
        </div>
      {:else if curlResult?.error}
        <div style="margin-top: 10px; padding: 8px 12px; border-radius: 8px; background: rgba(239,68,68,0.06); border: 1px solid rgba(239,68,68,0.2);">
          <div style="font-size: 12px; font-weight: 600; color: var(--color-error);">❌ Import failed</div>
          <div style="font-size: 11px; color: var(--color-fg-2); margin-top: 2px;">{curlResult.error}</div>
        </div>
      {/if}
      <div class="flex justify-end gap-2 mt-3">
        <button class="btn-primary flex items-center gap-2" onclick={importFromCurl} disabled={curlImporting || !curlText.trim()}>
          {#if curlImporting}
            <span class="inline-block" style="width: 12px; height: 12px; border: 2px solid var(--color-border); border-top-color: white; border-radius: 50%; animation: spin 0.8s linear infinite;"></span>
            Importing...
          {:else}
            <Copy size={14} /> Import & Discover
          {/if}
        </button>
      </div>
    </div>
  {/if}

  <!-- OAuth IDE lab presets (proxy wiring) -->
  <div class="card mb-5 oauth-lab-card">
    <button
      class="flex items-center justify-between gap-3 flex-wrap w-full"
      style="all: unset; cursor: pointer; padding: 0; width: 100%;"
      onclick={() => oauthExpanded = !oauthExpanded}
    >
      <div class="flex items-center gap-2">
        <ShieldAlert size={16} style="color: #a78bfa;" />
        <h3 style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin: 0;">OAuth IDE (lab)</h3>
        {#if oauthIdeEnabled}
          <span class="badge" style="background: rgba(34,197,94,0.12); color: #22c55e;">enabled</span>
        {:else}
          <span class="badge" style="background: var(--color-bg-3); color: var(--color-fg-3);">server off</span>
        {/if}
      </div>
      <div style="display: flex; align-items: center; gap: 8px;">
        <a href="/dashboard/oauth-ide" onclick={(e) => e.stopPropagation()} style="font-size: 12px; color: var(--color-primary);">Open OAuth IDE →</a>
        <span style="transform: rotate({oauthExpanded ? '180deg' : '0deg'}); transition: transform 0.2s; font-size: 12px; color: var(--color-fg-3);">▾</span>
      </div>
    </button>
    {#if oauthExpanded}
      <p style="font-size: 11px; color: var(--color-fg-3); margin: 12px 0 12px;">
        No API key — token from OAuth session. <strong>Add</strong> opens the form; <strong>Wire</strong> upserts the Accounts connection (needs active session).
      </p>
      <div class="oauth-preset-grid">
        {#each OAUTH_IDE_PRESETS as op}
          {@const brand = brandForProvider(op.oauth_provider)}
          {@const hasSession = oauthSessionActive(op.oauth_provider)}
          <div
            class="oauth-preset-tile"
            style="border-color: {brand.border}; background: linear-gradient(145deg, {brand.bg}, var(--color-bg-card));"
          ><div class="flex items-center gap-2 mb-1">
            <div style="width:22px;height:22px;border-radius:5px;background:{brand.bg};display:flex;align-items:center;justify-content:center;flex-shrink:0;">
              <svg viewBox="0 0 24 24" width="14" height="14">
                {#each logoPaths(brand) as path}
                  <path d={path.d} fill={path.fill} />
                {/each}
              </svg>
            </div>
            <div style="font-size: 12px; font-weight: 650; color: var(--color-fg-0);">{op.label}</div>
          </div>
            <div style="font-size: 10px; color: {brand.color}; margin-top: 2px;">{brand.company}</div>
            {#if hasSession}
              <span style="font-size: 9px; color: #22c55e; font-weight: 600; margin-top: 6px; display: block;">● session</span>
            {/if}
            <div class="oauth-tile-actions">
              <button type="button" class="btn-secondary" style="padding: 5px 8px; font-size: 10px;" onclick={() => pickOAuthIdePreset(op)}>
                Add
              </button>
              <button
                type="button"
                class="btn-primary"
                style="padding: 5px 8px; font-size: 10px;"
                disabled={!oauthIdeEnabled || !hasSession || wiringOAuth === op.oauth_provider}
                title={!hasSession ? 'Authorize in OAuth IDE first' : 'Upsert connection'}
                onclick={() => quickWireOAuth(op.oauth_provider)}
              >
                {wiringOAuth === op.oauth_provider ? '…' : 'Wire'}
              </button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Provider Preset Catalog -->
  <div class="card mb-5" style="padding: 0; overflow: hidden;">
    <!-- Preset Header -->
    <button
      style="all: unset; cursor: pointer; padding: 16px 20px; border-bottom: 1px solid {presetsExpanded ? 'var(--color-border)' : 'transparent'}; display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; width: 100%; box-sizing: border-box;"
      onclick={() => presetsExpanded = !presetsExpanded}
    >
      <div class="flex items-center gap-2">
        <Sparkles size={16} style="color: var(--color-primary);" />
        <h3 style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin: 0;">Provider Presets</h3>
        <span style="font-size: 11px; color: var(--color-fg-3);">Click to add · Manage to customize</span>
      </div>
      <span style="transform: rotate({presetsExpanded ? '180deg' : '0deg'}); transition: transform 0.2s; font-size: 12px; color: var(--color-fg-3);">▾</span>
    </button>
    {#if presetsExpanded}
      <!-- Search + action bar -->
      <div style="padding: 12px 20px; border-bottom: 1px solid var(--color-border); display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap;">
        <div class="flex items-center gap-2">
          <div class="relative" style="width: 240px;">
            <Search size={14} class="absolute" style="left: 10px; top: 50%; transform: translateY(-50%); color: var(--color-fg-3); pointer-events: none;" />
            <input
              class="input-field"
              placeholder="Search providers..."
              bind:value={presetSearch}
              style="padding-left: 32px; font-size: 12px; padding-top: 6px; padding-bottom: 6px;"
            />
          </div>
          <button class="btn-secondary flex items-center gap-1" style="padding: 6px 12px; font-size: 12px;" onclick={() => openManage('categories')} title="Manage Categories">
            <FolderTree size={14} />
            Categories
          </button>
          <button class="btn-secondary flex items-center gap-1" style="padding: 6px 12px; font-size: 12px;" onclick={() => openManage('presets')} title="Manage Presets">
            <Settings size={14} />
            Presets
          </button>
          <button class="btn-primary flex items-center gap-1" style="padding: 6px 12px; font-size: 12px;" onclick={startNewPreset}>
            <Plus size={14} />
            Add
          </button>
        </div>
      </div>

    {#if presetsLoading || categoriesLoading}
      <div class="flex justify-center" style="padding: 40px;">
        <Spinner />
      </div>
    {:else if activeCategories.length === 0}
      <div style="padding: 30px 20px; text-align: center;">
        <Sparkles size={28} style="color: var(--color-fg-3); opacity: 0.4; margin: 0 auto 8px;" />
        <div style="font-size: 14px; color: var(--color-fg-1); font-weight: 500;">No presets yet</div>
        <div style="font-size: 12px; color: var(--color-fg-3); margin-top: 4px;">Add a provider preset to get started</div>
        <button class="btn-primary" style="margin-top: 12px;" onclick={startNewPreset}>
          <Plus size={14} /> Add First Preset
        </button>
      </div>
    {:else}
      <!-- Preset Grid - Categorized -->
      <div style="padding: 12px 20px 16px;">
        {#each activeCategories as cat}
          {#if groupedPresets[cat.key]?.length > 0}
            <div style="margin-bottom: 14px;">
              <!-- Category Header -->
              <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px; padding: 0 2px;">
                <span style="font-size: 13px;">{cat.icon}</span>
                <span style="font-size: 11px; font-weight: 700; color: var(--color-fg-2); text-transform: uppercase; letter-spacing: 0.8px;">{cat.label}</span>
                <span style="font-size: 10px; color: {cat.color}; background: {cat.color}15; padding: 1px 6px; border-radius: 8px; font-weight: 600;">{groupedPresets[cat.key].length}</span>
                <div style="flex: 1; height: 1px; background: var(--color-border);"></div>
              </div>
              <!-- Category Grid -->
              <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 8px;">
                {#each groupedPresets[cat.key] as preset (preset.id)}
                  {@const isConnected = connectedNames.has(preset.name.toLowerCase().replace(/\s+/g, '-'))}
                  <div
                    class="preset-card"
                    style="position: relative; {isConnected ? 'opacity: 0.6;' : ''} border-color: {isConnected ? cat.color + '40' : 'var(--color-border)'};"
                  >
                    <button
                      onclick={() => pickPreset(preset)}
                      title={isConnected ? `${preset.name} — already added` : `Add ${preset.name}`}
                      style="all: unset; display: flex; align-items: center; gap: 10px; padding: 10px 12px; cursor: pointer; width: 100%; box-sizing: border-box;"
                    >
                      {#if isConnected}
                        <div style="position: absolute; top: 6px; right: 6px; width: 16px; height: 16px; border-radius: 50%; background: {cat.color}; display: flex; align-items: center; justify-content: center;">
                          <Check size={10} color="white" />
                        </div>
                      {/if}
                      <img
                        src={faviconUrl(preset.domain, 32)}
                        alt={preset.name}
                        width="22" height="22"
                        style="border-radius: 5px; flex-shrink: 0;"
                        onerror={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none'; }}
                      />
                      <div style="min-width: 0; flex: 1;">
                        <div style="font-size: 12px; font-weight: 600; color: var(--color-fg-0); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{preset.name}</div>
                        <div style="font-size: 9px; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">{preset.format}</div>
                      </div>
                    </button>
                    <div style="position: absolute; bottom: 4px; right: 4px; display: flex; gap: 2px;">
                      {#if preset.is_builtin === 1}
                        <span style="font-size: 8px; color: var(--color-fg-3); padding: 1px 4px; background: var(--color-bg-3); border-radius: 3px; text-transform: uppercase; letter-spacing: 0.3px;">built-in</span>
                      {:else}
                        <button
                          onclick={(e) => { e.stopPropagation(); startEditPreset(preset); }}
                          style="all: unset; display: flex; align-items: center; justify-content: center; width: 20px; height: 20px; border-radius: 4px; cursor: pointer; color: var(--color-fg-3);"
                          title="Edit preset"
                        >
                          <Pencil size={11} />
                        </button>
                        <button
                          onclick={(e) => { e.stopPropagation(); deletePreset(preset); }}
                          style="all: unset; display: flex; align-items: center; justify-content: center; width: 20px; height: 20px; border-radius: 4px; cursor: pointer; color: var(--color-error, #ef4444);"
                          title="Delete preset"
                        >
                          <Trash2 size={11} />
                        </button>
                      {/if}
                    </div>
                  </div>
                {/each}
              </div>
            </div>
          {/if}
        {/each}
      </div>
    {/if}
    {/if}
  </div>

  <!-- Add/Edit Preset Modal -->
  {#if showEditForm}
    <div style="position: fixed; inset: 0; background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 1000;" onclick={() => showEditForm = false}>
      <div class="card" style="width: 90%; max-width: 480px; padding: 24px;" onclick={(e) => e.stopPropagation()}>
        <div class="flex items-center justify-between mb-4">
          <h3 style="font-size: 16px; font-weight: 600; color: var(--color-fg-0); margin: 0;">
            {editingPreset ? 'Edit Preset' : 'New Provider Preset'}
          </h3>
          <button onclick={() => showEditForm = false} style="all: unset; cursor: pointer; color: var(--color-fg-3); padding: 4px;">
            <X size={18} />
          </button>
        </div>
        <div style="display: flex; flex-direction: column; gap: 12px;">
          <div>
            <label for="preset-name" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Name *</label>
            <input id="preset-name" class="input-field" bind:value={presetForm.name} placeholder="My Provider" />
          </div>
          <div>
            <label for="preset-domain" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Domain (for favicon) *</label>
            <input id="preset-domain" class="input-field" bind:value={presetForm.domain} placeholder="myprovider.com" />
          </div>
          <div>
            <label for="preset-base-url" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Base URL *</label>
            <input id="preset-base-url" class="input-field" bind:value={presetForm.base_url} placeholder="https://api.myprovider.com/v1" />
          </div>
          <div class="grid grid-cols-2" style="gap: 12px;">
            <div>
              <label for="preset-format" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Format</label>
              <select id="preset-format" class="input-field" bind:value={presetForm.format}>
                <option value="openai">OpenAI</option>
                <option value="anthropic">Anthropic</option>
                <option value="gemini">Gemini</option>
              </select>
            </div>
            <div>
              <label for="preset-category" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Category</label>
              <select id="preset-category" class="input-field" bind:value={presetForm.category}>
                {#each categories as cat}
                  <option value={cat.key}>{cat.icon} {cat.label}</option>
                {/each}
              </select>
            </div>
          </div>
          <div>
            <label for="preset-key-label" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">API Key Label</label>
            <input id="preset-key-label" class="input-field" bind:value={presetForm.key_label} placeholder="API Key" />
          </div>
        </div>
        <div class="flex justify-end gap-2 mt-5">
          <button class="btn-secondary" onclick={() => showEditForm = false} disabled={savingPreset}>Cancel</button>
          <button class="btn-primary flex items-center gap-1" onclick={savePreset} disabled={savingPreset}>
            <Save size={14} />
            {savingPreset ? 'Saving...' : (editingPreset ? 'Update' : 'Add Preset')}
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Add/Edit Category Modal -->
  {#if showCategoryForm}
    <div style="position: fixed; inset: 0; background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 1000;" onclick={() => showCategoryForm = false}>
      <div class="card" style="width: 90%; max-width: 440px; padding: 24px;" onclick={(e) => e.stopPropagation()}>
        <div class="flex items-center justify-between mb-4">
          <h3 style="font-size: 16px; font-weight: 600; color: var(--color-fg-0); margin: 0;">
            {editingCategory ? 'Edit Category' : 'New Category'}
          </h3>
          <button onclick={() => showCategoryForm = false} style="all: unset; cursor: pointer; color: var(--color-fg-3); padding: 4px;">
            <X size={18} />
          </button>
        </div>
        <div style="display: flex; flex-direction: column; gap: 12px;">
          {#if !editingCategory}
            <div>
              <label for="cat-key" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Key (slug, auto-generated from label if empty)</label>
              <input id="cat-key" class="input-field" bind:value={categoryForm.key} placeholder="my-category" />
            </div>
          {:else}
            <div style="font-size: 12px; color: var(--color-fg-3);">
              Key: <code style="background: var(--color-bg-3); padding: 1px 6px; border-radius: 3px; font-family: var(--font-mono);">{editingCategory.key}</code>
              {#if editingCategory.is_builtin === 1}
                <span style="font-size: 9px; color: var(--color-fg-3); padding: 1px 5px; background: var(--color-bg-3); border-radius: 3px; text-transform: uppercase; letter-spacing: 0.3px; margin-left: 6px;">built-in</span>
              {/if}
            </div>
          {/if}
          <div>
            <label for="cat-label" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Label *</label>
            <input id="cat-label" class="input-field" bind:value={categoryForm.label} placeholder="My Category" />
          </div>
          <div class="grid grid-cols-2" style="gap: 12px;">
            <div>
              <label for="cat-icon" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Icon (emoji)</label>
              <input id="cat-icon" class="input-field" bind:value={categoryForm.icon} placeholder="📦" maxlength="4" style="font-size: 16px;" />
            </div>
            <div>
              <label for="cat-color" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Color</label>
              <div style="display: flex; gap: 6px; align-items: center;">
                <input id="cat-color" type="color" bind:value={categoryForm.color} style="width: 36px; height: 32px; padding: 0; border: 1px solid var(--color-border); border-radius: 6px; cursor: pointer;" />
                <input class="input-field" bind:value={categoryForm.color} placeholder="#8b5cf6" style="flex: 1; font-family: var(--font-mono); font-size: 12px;" />
              </div>
            </div>
          </div>
          <div>
            <label for="cat-order" style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 4px;">Sort Order (lower = first)</label>
            <input id="cat-order" type="number" class="input-field" bind:value={categoryForm.sort_order} />
          </div>
        </div>
        <div class="flex justify-end gap-2 mt-5">
          <button class="btn-secondary" onclick={() => showCategoryForm = false} disabled={savingCategory}>Cancel</button>
          <button class="btn-primary flex items-center gap-1" onclick={saveCategory} disabled={savingCategory}>
            <Save size={14} />
            {savingCategory ? 'Saving...' : (editingCategory ? 'Update' : 'Add Category')}
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Manage Modal (Tabbed: Presets | Categories) -->
  {#if showManageModal}
    <div style="position: fixed; inset: 0; background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 1000;" onclick={() => showManageModal = false}>
      <div class="card" style="width: 90%; max-width: 600px; max-height: 80vh; padding: 0; display: flex; flex-direction: column;" onclick={(e) => e.stopPropagation()}>
        <!-- Tabs -->
        <div style="display: flex; border-bottom: 1px solid var(--color-border);">
          <button
            onclick={() => manageTab = 'presets'}
            style="all: unset; flex: 1; padding: 14px 20px; cursor: pointer; font-size: 13px; font-weight: 600; text-align: center; color: {manageTab === 'presets' ? 'var(--color-primary)' : 'var(--color-fg-3)'}; border-bottom: 2px solid {manageTab === 'presets' ? 'var(--color-primary)' : 'transparent'};"
          >
            Presets ({presets.length})
          </button>
          <button
            onclick={() => manageTab = 'categories'}
            style="all: unset; flex: 1; padding: 14px 20px; cursor: pointer; font-size: 13px; font-weight: 600; text-align: center; color: {manageTab === 'categories' ? 'var(--color-primary)' : 'var(--color-fg-3)'}; border-bottom: 2px solid {manageTab === 'categories' ? 'var(--color-primary)' : 'transparent'};"
          >
            Categories ({categories.length})
          </button>
          <button onclick={() => showManageModal = false} style="all: unset; cursor: pointer; color: var(--color-fg-3); padding: 14px 16px;">
            <X size={16} />
          </button>
        </div>

        <div style="overflow-y: auto; flex: 1; padding: 8px 0;">
          {#if manageTab === 'presets'}
            {#each presets as preset (preset.id)}
              <div style="display: flex; align-items: center; gap: 12px; padding: 10px 20px; border-bottom: 1px solid var(--color-border-light);">
                <img src={faviconUrl(preset.domain, 32)} alt={preset.name} width="20" height="20" style="border-radius: 4px; flex-shrink: 0;" onerror={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none'; }} />
                <div style="flex: 1; min-width: 0;">
                  <div style="font-size: 13px; font-weight: 600; color: var(--color-fg-0);">{preset.name}</div>
                  <div style="font-size: 11px; color: var(--color-fg-3); font-family: var(--font-mono); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{preset.base_url}</div>
                </div>
                <div style="display: flex; align-items: center; gap: 6px;">
                  {#if preset.is_builtin === 1}
                    <span style="font-size: 9px; color: var(--color-fg-3); padding: 2px 6px; background: var(--color-bg-3); border-radius: 4px; text-transform: uppercase; letter-spacing: 0.3px;">built-in</span>
                  {/if}
                  <span style="font-size: 9px; color: {categoryMap[preset.category]?.color || 'var(--color-fg-3)'}; padding: 2px 6px; background: {(categoryMap[preset.category]?.color || 'var(--color-fg-3)')}15; border-radius: 4px; text-transform: uppercase; letter-spacing: 0.3px;">{categoryMap[preset.category]?.icon || ''} {categoryMap[preset.category]?.label || preset.category}</span>
                </div>
                <div style="display: flex; gap: 4px;">
                  {#if preset.is_builtin !== 1}
                    <button onclick={() => { showManageModal = false; startEditPreset(preset); }} style="all: unset; display: flex; align-items: center; justify-content: center; width: 26px; height: 26px; border-radius: 5px; cursor: pointer; color: var(--color-fg-2); background: var(--color-bg-3);" title="Edit">
                      <Pencil size={12} />
                    </button>
                    <button onclick={() => deletePreset(preset)} style="all: unset; display: flex; align-items: center; justify-content: center; width: 26px; height: 26px; border-radius: 5px; cursor: pointer; color: var(--color-error, #ef4444); background: rgba(239,68,68,0.1);" title="Delete">
                      <Trash2 size={12} />
                    </button>
                  {/if}
                </div>
              </div>
            {/each}
          {:else}
            {#each [...categories].sort((a, b) => a.sort_order - b.sort_order) as cat}
              <div style="display: flex; align-items: center; gap: 12px; padding: 10px 20px; border-bottom: 1px solid var(--color-border-light);">
                <div style="width: 32px; height: 32px; border-radius: 8px; background: {cat.color}20; display: flex; align-items: center; justify-content: center; flex-shrink: 0; font-size: 16px;">
                  {cat.icon}
                </div>
                <div style="flex: 1; min-width: 0;">
                  <div style="font-size: 13px; font-weight: 600; color: var(--color-fg-0); display: flex; align-items: center; gap: 6px;">
                    {cat.label}
                    {#if cat.is_builtin === 1}
                      <span style="font-size: 9px; color: var(--color-fg-3); padding: 1px 5px; background: var(--color-bg-3); border-radius: 3px; text-transform: uppercase; letter-spacing: 0.3px;">built-in</span>
                    {/if}
                  </div>
                  <div style="font-size: 11px; color: var(--color-fg-3); font-family: var(--font-mono);">
                    key: {cat.key} · order: {cat.sort_order} · {categoryPresetCounts[cat.key] || 0} preset(s)
                  </div>
                </div>
                <div style="display: flex; gap: 4px;">
                  <button onclick={() => { showManageModal = false; startEditCategory(cat); }} style="all: unset; display: flex; align-items: center; justify-content: center; width: 26px; height: 26px; border-radius: 5px; cursor: pointer; color: var(--color-fg-2); background: var(--color-bg-3);" title="Edit category">
                    <Pencil size={12} />
                  </button>
                  {#if cat.is_builtin !== 1}
                    <button onclick={() => deleteCategory(cat)} style="all: unset; display: flex; align-items: center; justify-content: center; width: 26px; height: 26px; border-radius: 5px; cursor: pointer; color: var(--color-error, #ef4444); background: rgba(239,68,68,0.1);" title="Delete category">
                      <Trash2 size={12} />
                    </button>
                  {/if}
                </div>
              </div>
            {/each}
          {/if}
        </div>

        <div style="padding: 12px 20px; border-top: 1px solid var(--color-border); display: flex; justify-content: space-between; align-items: center;">
          {#if manageTab === 'presets'}
            <span style="font-size: 11px; color: var(--color-fg-3);">{presets.length} presets ({presets.filter(p => p.is_builtin === 1).length} built-in, {presets.filter(p => p.is_builtin !== 1).length} custom)</span>
            <button class="btn-primary flex items-center gap-1" style="padding: 6px 12px; font-size: 12px;" onclick={() => { showManageModal = false; startNewPreset(); }}>
              <Plus size={14} /> Add New
            </button>
          {:else}
            <span style="font-size: 11px; color: var(--color-fg-3);">{categories.length} categories ({categories.filter(c => c.is_builtin === 1).length} built-in, {categories.filter(c => c.is_builtin !== 1).length} custom)</span>
            <button class="btn-primary flex items-center gap-1" style="padding: 6px 12px; font-size: 12px;" onclick={() => { showManageModal = false; startNewCategory(); }}>
              <Plus size={14} /> Add New
            </button>
          {/if}
        </div>
      </div>
    </div>
  {/if}

  <!-- Pool Health Section -->
  {#if pools.length > 0}
    <div style="margin-bottom: 16px;">
      <h3 style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 10px; display: flex; align-items: center; gap: 6px;">
        <Layers size={16} style="color: var(--color-primary);" />
        Connection Pools
        <span class="badge" style="font-size: 10px; background: var(--color-primary-light); color: var(--color-primary);">{pools.length}</span>
      </h3>
      <div class="flex flex-wrap" style="gap: 10px;">
        {#each pools as pool}
          <button
            class="card"
            style="padding: 12px 16px; min-width: 200px; width: auto; cursor: pointer; user-select: none; border-left: 3px solid {pool.available_count > 0 ? 'var(--color-success)' : pool.rate_limited > 0 ? 'var(--color-error)' : 'var(--color-warning)'}; transition: all 0.15s; text-align: left;"
            onclick={() => { document.getElementById('pool-' + pool.pool_id)?.scrollIntoView({ behavior: 'smooth', block: 'start' }); }}
            onmouseenter={(e) => { e.currentTarget.style.transform = 'translateY(-2px)'; e.currentTarget.style.boxShadow = '0 4px 12px rgba(99,102,241,0.1)'; }}
            onmouseleave={(e) => { e.currentTarget.style.transform = ''; e.currentTarget.style.boxShadow = ''; }}
            title="Click to jump to {pool.pool_id} connections"
          >
            <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
              <div style="width: 8px; height: 8px; border-radius: 50%; background: {pool.available_count > 0 ? 'var(--color-success)' : 'var(--color-error)'}; box-shadow: 0 0 8px {pool.available_count > 0 ? 'rgba(16,185,129,0.4)' : 'rgba(239,68,68,0.4)'};"></div>
              <span style="font-size: 13px; font-weight: 600; color: var(--color-fg-0); font-family: var(--font-mono);">{pool.pool_id}</span>
              <span style="font-size: 10px; color: var(--color-fg-3); margin-left: auto;">{pool.num_accounts} acct</span>
            </div>
            <!-- Health mini-bars -->
            <div style="display: flex; gap: 6px; align-items: center;">
              <div style="flex: 1; height: 4px; background: var(--color-border); border-radius: 2px; overflow: hidden; display: flex;">
                {#if pool.total_requests > 0}
                  <div style="width: {(pool.success_count / pool.total_requests * 100).toFixed(0)}%; height: 100%; background: var(--color-success); border-radius: 2px 0 0 2px; transition: width 0.3s;"></div>
                  <div style="width: {(pool.fail_count / pool.total_requests * 100).toFixed(0)}%; height: 100%; background: var(--color-error); border-radius: 0 2px 2px 0; transition: width 0.3s;"></div>
                {:else}
                  <div style="flex: 1; height: 100%; background: var(--color-border); border-radius: 2px;"></div>
                {/if}
              </div>
              <span style="font-size: 10px; font-weight: 600; color: {pool.total_requests === 0 ? 'var(--color-fg-3)' : 'var(--color-fg-1)'}; font-family: var(--font-mono); white-space: nowrap;">{pool.total_requests === 0 ? '—%' : (pool.success_rate * 100).toFixed(0) + '%'}</span>
            </div>
            <div style="display: flex; gap: 10px; margin-top: 6px; font-size: 10px; color: var(--color-fg-3);">
              <span title="Requests served">✓ {pool.success_count}</span>
              <span title="Failed requests">✗ {pool.fail_count}</span>
              <span title="Rate-limited accounts" class:rate-limited={pool.rate_limited > 0} style="color: {pool.rate_limited > 0 ? 'var(--color-error)' : 'var(--color-fg-3)'};">⚠ {pool.rate_limited}</span>
              <span title="Healthy accounts" style="margin-left: auto; color: var(--color-success);">{pool.available_count} active</span>
            </div>
          </button>
        {/each}
      </div>
    </div>
  {:else if connections.length > 1}
    <!-- No pools yet — show hint -->
    <div style="margin-bottom: 16px; padding: 16px; border: 1px dashed var(--color-border); border-radius: 12px; background: var(--color-bg-body);">
      <div style="display: flex; align-items: center; gap: 10px;">
        <div style="width: 32px; height: 32px; border-radius: 8px; background: var(--color-primary-light); display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
          <Layers size={16} style="color: var(--color-primary);" />
        </div>
        <div style="flex: 1;">
          <div style="font-size: 13px; font-weight: 600; color: var(--color-fg-0);">Create a pool to load balance</div>
          <div style="font-size: 11px; color: var(--color-fg-3); margin-top: 2px;">
            Click <code style="background: var(--color-border-light); padding: 1px 4px; border-radius: 3px;">+ pool</code> on any connection card below to group API keys for automatic round-robin and failover.
          </div>
        </div>
      </div>
    </div>
  {/if}

  {#if loading}
    <Spinner />
  {:else if connections.length === 0}
    <div class="card" style="text-align: center;">
      <EmptyState title="No connections" description="Pick a provider above to add your first LLM connection." />
      <div style="margin-top: 16px; padding: 14px 18px; background: var(--color-bg-body); border-radius: 10px; text-align: left; font-size: 12px; color: var(--color-fg-2); line-height: 1.6;">
        <div style="font-weight: 600; margin-bottom: 6px; color: var(--color-fg-0); display: flex; align-items: center; gap: 6px;"><Layers size={14} style="color: var(--color-primary);" /> Why use pools?</div>
        <ul style="margin: 0; padding-left: 18px; list-style: disc;">
          <li><strong>Load balance</strong> across multiple API keys for the same provider</li>
          <li><strong>Auto failover</strong> — if one key gets rate-limited, the next takes over</li>
          <li><strong>Priority draining</strong> — drain primary accounts first, fallback last</li>
          <li>Group connections by provider model line (e.g. <code style="background: var(--color-bg-3); padding: 1px 4px; border-radius: 3px;">openai-prod</code>, <code style="background: var(--color-bg-3); padding: 1px 4px; border-radius: 3px;">anthropic-us</code>)</li>
        </ul>
      </div>
    </div>
  {:else}
    <!-- Connection Cards -->
    {#if sortedConnections.length === 0}
      <div class="card">
        <EmptyState title="No matching connections" description="Try adjusting your search filter." />
      </div>
    {:else}
      <div class="conn-groups">
        {#each groupedConnections as group (group.key)}
          {@const isSingle = group.connections.length === 1}
          {@const isCollapsed = collapsedGroups.has(group.key)}

          {#if isSingle}
            <!-- Single connection: render card directly without group wrapper -->
            {@const conn = group.connections[0]}
            <div class="card conn-card" class:conn-inactive={!conn.is_active}>
              <div class="conn-card-header">
                <span class="conn-status-dot" class:active={conn.is_active} title={conn.is_active ? 'Active' : 'Inactive'}></span>
                <span class="conn-card-name" title={conn.name}>{conn.name}</span>
                <span class="badge conn-format-badge">{conn.format}</span>
                <span class="conn-card-priority" title="Priority">P{conn.priority}</span>
              </div>
              <div class="conn-card-meta">
                {#if conn.api_key}
                  <span class="conn-card-key" title={conn.api_key}>{conn.api_key}</span>
                {:else if conn.oauth_provider}
                  <span class="conn-card-oauth">OAuth:{conn.oauth_provider}</span>
                {/if}
                <div class="conn-card-meta-row">
                  <span class="conn-card-url" title={conn.base_url}>{conn.base_url}</span>
                  {#if conn.pool_id}
                    <span class="badge conn-pool-badge" title="Pool: {conn.pool_id}">{conn.pool_id}</span>
                  {/if}
                  <span class="conn-card-models" title="Models">{conn.models_count || 0} models</span>
                </div>
              </div>
              <div class="conn-card-actions">
                <button class="btn-secondary conn-action-btn" onclick={() => testConn(conn.id)} disabled={testing === conn.id} title="Test connection">
                  {#if testing === conn.id}<span class="conn-spinner"></span>{:else}<TestTube2 size={13} />{/if}
                  <span>Test</span>
                </button>
                <button class="btn-secondary conn-action-btn" onclick={() => toggleActive(conn)} title={conn.is_active ? 'Deactivate' : 'Activate'}>
                  {#if conn.is_active}<ToggleRight size={15} style="color: var(--color-success);" />{:else}<ToggleLeft size={15} />{/if}
                  <span>{conn.is_active ? 'On' : 'Off'}</span>
                </button>
                <button class="btn-secondary conn-action-btn" onclick={() => syncModels(conn.id)} disabled={syncing === conn.id} title="Sync models">
                  <RefreshCw size={13} class={syncing === conn.id ? 'animate-spin' : ''} />
                  <span>Sync</span>
                </button>
                <button class="btn-secondary conn-action-btn" onclick={() => openModelsViewer(conn)} title="View models">
                  <Cpu size={13} /><span>Models</span>
                </button>
                <div class="kebab-menu-container" style="position: relative;">
                  <button class="btn-secondary conn-action-btn conn-kebab-btn" onclick={(e) => { e.stopPropagation(); openMenuConnId = openMenuConnId === conn.id ? null : conn.id; }} aria-label="More actions">⋯</button>
                  {#if openMenuConnId === conn.id}
                    <div class="conn-dropdown" onclick={(e) => e.stopPropagation()}>
                      {#if !conn.pool_id}
                        <button class="conn-dropdown-item" onclick={() => { openMenuConnId = null; poolEditText = conn.pool_id || ''; editingPool = conn.id; }}><Layers size={14} /> Edit Pool</button>
                      {/if}
                      <button class="conn-dropdown-item" onclick={async () => { openMenuConnId = null; try { await navigator.clipboard.writeText(conn.api_key || ''); showToast('API key copied', 'success', 2000); } catch { showToast('Copy failed', 'error'); } }}><Copy size={14} /> Copy API Key</button>
                      <div class="conn-dropdown-divider"></div>
                      <button class="conn-dropdown-item conn-dropdown-danger" onclick={() => { openMenuConnId = null; deleteConn(conn.id); }}><Trash2 size={14} /> Delete</button>
                    </div>
                  {/if}
                </div>
              </div>
            </div>

          {:else}
            <!-- Multi-connection group -->
            <div class="conn-group" class:collapsed={isCollapsed}>
              <button class="conn-group-header" onclick={() => toggleGroup(group.key)}>
                <div class="conn-group-header-left">
                  <span class="conn-group-chevron" class:rotated={!isCollapsed}><ChevronRight size={16} /></span>
                  <span class="conn-group-label">{group.label}</span>
                  <span class="conn-group-count">{group.connections.length} accounts</span>
                </div>
                <div class="conn-group-header-right">
                  <span class="conn-group-stat">
                    <span class="conn-group-stat-dot active"></span>
                    {group.active}/{group.connections.length} active
                  </span>
                  {#if group.totalModels > 0}
                    <span class="conn-group-stat">
                      <Cpu size={12} />
                      {group.totalModels} models
                    </span>
                  {/if}
                </div>
              </button>

              {#if !isCollapsed}
                <div class="conn-group-body">
                  {#each group.connections as conn (conn.id)}
                    <div class="card conn-card conn-card-nested" class:conn-inactive={!conn.is_active}>
                      <div class="conn-card-header">
                        <span class="conn-status-dot" class:active={conn.is_active} title={conn.is_active ? 'Active' : 'Inactive'}></span>
                        <span class="conn-card-name" title={conn.name}>{conn.name}</span>
                        <span class="badge conn-format-badge">{conn.format}</span>
                        <span class="conn-card-priority" title="Priority">P{conn.priority}</span>
                      </div>
                      <div class="conn-card-meta">
                        {#if conn.api_key}
                          <span class="conn-card-key" title={conn.api_key}>{conn.api_key}</span>
                        {:else if conn.oauth_provider}
                          <span class="conn-card-oauth">OAuth:{conn.oauth_provider}</span>
                        {/if}
                        <div class="conn-card-meta-row">
                          {#if conn.pool_id}
                            <span class="badge conn-pool-badge" title="Pool: {conn.pool_id}">{conn.pool_id}</span>
                          {/if}
                          <span class="conn-card-models" title="Models">{conn.models_count || 0} models</span>
                        </div>
                      </div>
                      <div class="conn-card-actions">
                        <button class="btn-secondary conn-action-btn" onclick={() => testConn(conn.id)} disabled={testing === conn.id} title="Test">
                          {#if testing === conn.id}<span class="conn-spinner"></span>{:else}<TestTube2 size={13} />{/if}
                          <span>Test</span>
                        </button>
                        <button class="btn-secondary conn-action-btn" onclick={() => toggleActive(conn)} title={conn.is_active ? 'Deactivate' : 'Activate'}>
                          {#if conn.is_active}<ToggleRight size={15} style="color: var(--color-success);" />{:else}<ToggleLeft size={15} />{/if}
                          <span>{conn.is_active ? 'On' : 'Off'}</span>
                        </button>
                        <button class="btn-secondary conn-action-btn" onclick={() => syncModels(conn.id)} disabled={syncing === conn.id} title="Sync">
                          <RefreshCw size={13} class={syncing === conn.id ? 'animate-spin' : ''} />
                          <span>Sync</span>
                        </button>
                        <button class="btn-secondary conn-action-btn" onclick={() => openModelsViewer(conn)} title="Models">
                          <Cpu size={13} /><span>Models</span>
                        </button>
                        <div class="kebab-menu-container" style="position: relative;">
                          <button class="btn-secondary conn-action-btn conn-kebab-btn" onclick={(e) => { e.stopPropagation(); openMenuConnId = openMenuConnId === conn.id ? null : conn.id; }} aria-label="More">⋯</button>
                          {#if openMenuConnId === conn.id}
                            <div class="conn-dropdown" onclick={(e) => e.stopPropagation()}>
                              {#if !conn.pool_id}
                                <button class="conn-dropdown-item" onclick={() => { openMenuConnId = null; poolEditText = conn.pool_id || ''; editingPool = conn.id; }}><Layers size={14} /> Edit Pool</button>
                              {/if}
                              <button class="conn-dropdown-item" onclick={async () => { openMenuConnId = null; try { await navigator.clipboard.writeText(conn.api_key || ''); showToast('API key copied', 'success', 2000); } catch { showToast('Copy failed', 'error'); } }}><Copy size={14} /> Copy API Key</button>
                              <div class="conn-dropdown-divider"></div>
                              <button class="conn-dropdown-item conn-dropdown-danger" onclick={() => { openMenuConnId = null; deleteConn(conn.id); }}><Trash2 size={14} /> Delete</button>
                            </div>
                          {/if}
                        </div>
                      </div>
                    </div>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
        {/each}
      </div>
    {/if}

    <!-- Pool inline editing (shown when editingPool is set) -->
    {#if editingPool}
      <div style="margin-top: 12px; padding: 12px 16px; border: 1px solid var(--color-primary); border-radius: 10px; background: var(--color-bg-card); display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
        <Layers size={14} style="color: var(--color-primary); flex-shrink: 0;" />
        <span style="font-size: 12px; color: var(--color-fg-2);">Set pool for connection:</span>
        <input
          type="text"
          bind:value={poolEditText}
          placeholder="pool name"
          style="font-size: 12px; padding: 4px 8px; width: 120px; border: 1px solid var(--color-border); border-radius: 6px; font-family: var(--font-mono); background: var(--color-bg-body); color: var(--color-fg-0);"
          onkeydown={(e: KeyboardEvent) => {
            const conn = connections.find(c => c.id === editingPool);
            if (e.key === 'Enter' && conn) setPoolId(conn.id, poolEditText?.trim());
            if (e.key === 'Escape') editingPool = null;
          }}
        />
        {#if pools.length > 0}
          <select
            style="font-size: 11px; padding: 4px 6px; border: 1px solid var(--color-border); border-radius: 6px; font-family: var(--font-mono); background: var(--color-bg-body); color: var(--color-fg-0); cursor: pointer;"
            onchange={(e: Event) => {
              const conn = connections.find(c => c.id === editingPool);
              if (conn) poolEditText = (e.target as HTMLSelectElement).value;
            }}
          >
            <option value="">pick existing…</option>
            {#each pools as p}
              <option value={p.pool_id}>{p.pool_id}</option>
            {/each}
          </select>
        {/if}
        <button
          class="btn-primary"
          style="padding: 4px 10px; font-size: 11px;"
          onclick={() => {
            const conn = connections.find(c => c.id === editingPool);
            if (conn) setPoolId(conn.id, poolEditText?.trim());
          }}
        >
          <Check size={12} /> Save
        </button>
        <button class="btn-secondary" style="padding: 4px 10px; font-size: 11px;" onclick={() => editingPool = null}>
          Cancel
        </button>
      </div>
    {/if}
  {/if}

  <!-- Models Viewer Modal -->
  {#if viewingModelsOf}
    <div style="position: fixed; inset: 0; background: rgba(0,0,0,0.55); display: flex; align-items: center; justify-content: center; z-index: 1100; backdrop-filter: blur(2px);" onclick={closeModelsViewer}>
      <div class="card" style="width: 92%; max-width: 720px; max-height: 85vh; padding: 0; display: flex; flex-direction: column; box-shadow: 0 25px 50px -12px rgba(0,0,0,0.5);" onclick={(e) => e.stopPropagation()}>
        <!-- Header -->
        <div style="padding: 16px 20px; border-bottom: 1px solid var(--color-border); display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap;">
          <div class="flex items-center gap-3" style="min-width: 0; flex: 1;">
            <div style="width: 36px; height: 36px; border-radius: 8px; background: var(--color-primary-light); display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
              <Cpu size={18} style="color: var(--color-primary);" />
            </div>
            <div style="min-width: 0; flex: 1;">
              <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{viewingModelsOf.name}</div>
              <div style="font-size: 11px; color: var(--color-fg-3); font-family: var(--font-mono); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{viewingModelsOf.base_url}</div>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button
              class="btn-secondary flex items-center gap-1"
              style="padding: 6px 12px; font-size: 12px;"
              onclick={() => syncModelsInViewer(viewingModelsOf.id)}
              disabled={modelSyncing || !viewingModelsOf.is_active}
              title={!viewingModelsOf.is_active ? 'Connection must be active to sync' : 'Fetch latest models from upstream'}
            >
              <RefreshCw size={14} class={modelSyncing ? 'animate-spin' : ''} />
              {modelSyncing ? 'Syncing...' : 'Sync Now'}
            </button>
            <button onclick={closeModelsViewer} style="all: unset; cursor: pointer; color: var(--color-fg-3); padding: 4px;" title="Close">
              <X size={18} />
            </button>
          </div>
        </div>

        <!-- Stats strip -->
        {#if modelsList.length > 0}
          <div style="display: grid; grid-template-columns: repeat(4, 1fr); gap: 1px; background: var(--color-border); border-bottom: 1px solid var(--color-border);">
            <div class="text-center" style="padding: 10px; background: var(--color-bg-card);">
              <div style="font-size: 10px; font-weight: 500; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 2px;">Total</div>
              <div style="font-size: 16px; font-weight: 700; color: var(--color-fg-0); font-family: var(--font-mono);">{modelsStats.total}</div>
            </div>
            <div class="text-center" style="padding: 10px; background: var(--color-bg-card);">
              <div style="font-size: 10px; font-weight: 500; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 2px;">Active</div>
              <div style="font-size: 16px; font-weight: 700; color: var(--color-success); font-family: var(--font-mono);">{modelsStats.active}</div>
            </div>
            <div class="text-center" style="padding: 10px; background: var(--color-bg-card);">
              <div style="font-size: 10px; font-weight: 500; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 2px;">Inactive</div>
              <div style="font-size: 16px; font-weight: 700; color: var(--color-fg-3); font-family: var(--font-mono);">{modelsStats.inactive}</div>
            </div>
            <div class="text-center" style="padding: 10px; background: var(--color-bg-card);">
              <div style="font-size: 10px; font-weight: 500; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 2px;">Providers</div>
              <div style="font-size: 16px; font-weight: 700; color: var(--color-info); font-family: var(--font-mono);">{modelsStats.owners}</div>
            </div>
          </div>
        {/if}

        <!-- Search bar -->
        <div style="padding: 12px 20px; border-bottom: 1px solid var(--color-border);">
          <div class="relative" style="position: relative;">
            <Search size={14} style="position: absolute; left: 10px; top: 50%; transform: translateY(-50%); color: var(--color-fg-3); pointer-events: none;" />
            <input
              class="input-field"
              placeholder="Search models (id, name, owner)..."
              bind:value={modelsSearch}
              style="padding-left: 32px; font-size: 12px; padding-top: 7px; padding-bottom: 7px; width: 100%;"
            />
          </div>
        </div>

        <!-- Models list -->
        <div style="overflow-y: auto; flex: 1; padding: 4px 0;">
          {#if modelsLoading}
            <div class="flex justify-center" style="padding: 60px;">
              <Spinner />
            </div>
          {:else if modelsList.length === 0}
            <div style="padding: 50px 20px; text-align: center;">
              <Box size={32} style="color: var(--color-fg-3); opacity: 0.4; margin: 0 auto 12px;" />
              <div style="font-size: 14px; color: var(--color-fg-1); font-weight: 500;">No models discovered yet</div>
              <div style="font-size: 12px; color: var(--color-fg-3); margin-top: 6px; max-width: 320px; margin-left: auto; margin-right: auto;">
                {#if !viewingModelsOf.is_active}
                  Activate the connection first, then click <strong>Sync Now</strong> to discover models from the upstream API.
                {:else}
                  Click <strong>Sync Now</strong> to fetch models from <code style="background: var(--color-bg-3); padding: 1px 5px; border-radius: 3px; font-family: var(--font-mono);">{viewingModelsOf.base_url}/v1/models</code>
                {/if}
              </div>
              {#if viewingModelsOf.is_active}
                <button class="btn-primary" style="margin-top: 14px;" onclick={() => syncModelsInViewer(viewingModelsOf.id)} disabled={modelSyncing}>
                  <RefreshCw size={14} class={modelSyncing ? 'animate-spin' : ''} />
                  {modelSyncing ? 'Syncing...' : 'Sync Now'}
                </button>
              {/if}
            </div>
          {:else if filteredModels.length === 0}
            <div style="padding: 30px 20px; text-align: center;">
              <Search size={24} style="color: var(--color-fg-3); opacity: 0.5; margin: 0 auto 8px;" />
              <div style="font-size: 13px; color: var(--color-fg-2);">No models match "<strong>{modelsSearch}</strong>"</div>
              <button class="btn-secondary" style="margin-top: 10px; font-size: 11px; padding: 4px 10px;" onclick={() => modelsSearch = ''}>Clear search</button>
            </div>
          {:else}
            <table style="width: 100%; border-collapse: collapse; font-size: 12px;">
              <thead style="position: sticky; top: 0; background: var(--color-bg-card); z-index: 1;">
                <tr style="border-bottom: 1px solid var(--color-border);">
                  <th style="text-align: left; padding: 8px 12px; font-size: 10px; font-weight: 600; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">Model ID</th>
                  <th style="text-align: left; padding: 8px 12px; font-size: 10px; font-weight: 600; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">Owner</th>
                  <th style="text-align: left; padding: 8px 12px; font-size: 10px; font-weight: 600; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">Discovered</th>
                  <th style="text-align: center; padding: 8px 12px; font-size: 10px; font-weight: 600; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">Status</th>
                  <th style="text-align: right; padding: 8px 12px; font-size: 10px; font-weight: 600; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">Actions</th>
                </tr>
              </thead>
              <tbody>
                {#each filteredModels as model (model.id)}
                  <tr style="border-bottom: 1px solid var(--color-border-light); transition: background 0.1s;" onmouseenter={(e) => (e.currentTarget.style.background = 'var(--color-bg-3)')} onmouseleave={(e) => (e.currentTarget.style.background = 'transparent')}>
                    <td style="padding: 8px 12px; max-width: 0;">
                      <div style="display: flex; align-items: center; gap: 6px; min-width: 0;">
                        <code style="font-family: var(--font-mono); font-size: 11px; color: var(--color-fg-0); background: var(--color-bg-3); padding: 2px 6px; border-radius: 4px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 240px;" title={model.model_id}>{model.model_id}</code>
                        <button onclick={(e) => { e.stopPropagation(); copyModelId(model.model_id); }} style="all: unset; display: flex; align-items: center; justify-content: center; width: 20px; height: 20px; border-radius: 4px; cursor: pointer; color: var(--color-fg-3); flex-shrink: 0;" title="Copy model ID">
                          <Copy size={11} />
                        </button>
                      </div>
                    </td>
                    <td style="padding: 8px 12px;">
                      <span style="font-size: 10px; color: var(--color-fg-2); background: var(--color-bg-3); padding: 2px 6px; border-radius: 4px; text-transform: lowercase;">{model.owned_by || '—'}</span>
                    </td>
                    <td style="padding: 8px 12px; color: var(--color-fg-3); font-size: 11px; white-space: nowrap;">{model.discovered_at || '—'}</td>
                    <td style="padding: 8px 12px; text-align: center;">
                      {#if model.is_active === 1}
                        <span style="display: inline-flex; align-items: center; gap: 4px; font-size: 10px; font-weight: 600; color: var(--color-success); background: rgba(16,185,129,0.1); padding: 2px 8px; border-radius: 10px;">
                          <span style="width: 6px; height: 6px; border-radius: 50%; background: var(--color-success);"></span>
                          active
                        </span>
                      {:else}
                        <span style="display: inline-flex; align-items: center; gap: 4px; font-size: 10px; font-weight: 600; color: var(--color-fg-3); background: var(--color-bg-3); padding: 2px 8px; border-radius: 10px;">
                          <span style="width: 6px; height: 6px; border-radius: 50%; background: var(--color-fg-3);"></span>
                          inactive
                        </span>
                      {/if}
                    </td>
                    <td style="padding: 8px 12px; text-align: right;">
                      <button
                        onclick={() => toggleModelActive(viewingModelsOf.id, model.model_id, model.is_active)}
                        disabled={togglingModelId === model.model_id}
                        style="all: unset; display: inline-flex; align-items: center; gap: 4px; padding: 4px 8px; border-radius: 5px; cursor: pointer; font-size: 11px; color: {model.is_active === 1 ? 'var(--color-fg-2)' : 'var(--color-success)'}; background: {model.is_active === 1 ? 'var(--color-bg-3)' : 'rgba(16,185,129,0.1)'}; opacity: {togglingModelId === model.model_id ? 0.5 : 1};"
                        title={model.is_active === 1 ? 'Deactivate this model' : 'Activate this model'}
                      >
                        {#if model.is_active === 1}
                          <EyeOff size={11} /> Off
                        {:else}
                          <Eye size={11} /> On
                        {/if}
                      </button>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>

        <!-- Footer -->
        <div style="padding: 12px 20px; border-top: 1px solid var(--color-border); display: flex; justify-content: space-between; align-items: center; font-size: 11px; color: var(--color-fg-3);">
          <div>
            {#if !modelsLoading && modelsList.length > 0}
              Showing {filteredModels.length} of {modelsList.length} model(s){modelsSearch ? ` · filtered by "${modelsSearch}"` : ''}
            {:else if !modelsLoading}
              No models
            {:else}
              Loading...
            {/if}
          </div>
          <button class="btn-secondary" style="padding: 5px 12px; font-size: 11px;" onclick={closeModelsViewer}>Close</button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .oauth-lab-card {
    padding: 16px 20px;
    border-color: rgba(139, 92, 246, 0.28);
  }
  .oauth-preset-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 10px;
  }
  .oauth-preset-tile {
    border: 1px solid var(--color-border);
    border-radius: 10px;
    padding: 10px 12px;
  }
  .oauth-tile-actions {
    display: flex;
    gap: 6px;
    margin-top: 10px;
  }

  .preset-card {
    display: flex; align-items: center; gap: 10px;
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: 10px;
    transition: border-color 0.15s, transform 0.1s, box-shadow 0.15s;
    position: relative;
    min-height: 50px;
  }
  .preset-card:hover {
    border-color: var(--color-primary);
    box-shadow: 0 2px 8px rgba(59,130,246,0.12);
  }

  /* Bounce highlight on Test button when clicked */
  .bounce-highlight {
    animation: bounceFlash 0.6s ease-out;
  }
  @keyframes bounceFlash {
    0%   { transform: scale(1);    box-shadow: 0 0 0 0 rgba(59,130,246,0.6); }
    30%  { transform: scale(1.08); box-shadow: 0 0 0 10px rgba(59,130,246,0); }
    60%  { transform: scale(0.97); box-shadow: 0 0 0 6px rgba(59,130,246,0); }
    100% { transform: scale(1);    box-shadow: 0 0 0 0 rgba(59,130,246,0); }
  }
  /* Connections toolbar */
  .conn-toolbar {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 18px;
    margin-bottom: 20px;
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: 12px;
    flex-wrap: wrap;
  }
  .conn-toolbar-left {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  }
  .conn-toolbar-title {
    font-size: 17px;
    font-weight: 700;
    color: var(--color-fg-0);
    margin: 0;
    white-space: nowrap;
  }
  .conn-toolbar-count {
    font-size: 11px;
    font-weight: 600;
    color: var(--color-primary);
    background: var(--color-primary-light);
    padding: 2px 7px;
    border-radius: 10px;
    font-family: var(--font-mono);
  }
  .conn-toolbar-center {
    flex: 1;
    min-width: 180px;
    max-width: 360px;
  }
  .conn-search-wrap {
    position: relative;
    display: flex;
    align-items: center;
  }
  .conn-search-wrap :global(.conn-search-icon) {
    position: absolute;
    left: 10px;
    color: var(--color-fg-3);
    pointer-events: none;
  }
  .conn-search-input {
    padding-left: 32px !important;
    padding-right: 28px !important;
    font-size: 13px !important;
    width: 100%;
  }
  .conn-search-clear {
    all: unset;
    position: absolute;
    right: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    cursor: pointer;
    color: var(--color-fg-3);
    transition: background 0.1s;
  }
  .conn-search-clear:hover {
    background: var(--color-bg-sidebar-hover);
    color: var(--color-fg-0);
  }
  .conn-toolbar-right {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }
  .conn-toolbar-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 7px 12px;
    font-size: 13px;
    white-space: nowrap;
  }
  .conn-toolbar-btn-label {
    display: inline;
  }

  /* Connection cards grid + groups */
  .conn-groups {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .conn-grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 12px;
  }

  /* Provider group */
  .conn-group {
    border: 1px solid var(--color-border);
    border-radius: 12px;
    overflow: hidden;
    background: var(--color-bg-card);
  }
  .conn-group-header {
    all: unset;
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    box-sizing: border-box;
    padding: 12px 16px;
    cursor: pointer;
    background: var(--color-bg-body);
    border-bottom: 1px solid var(--color-border);
    transition: background 0.1s;
    gap: 12px;
    flex-wrap: wrap;
  }
  .conn-group-header:hover {
    background: var(--color-bg-sidebar-hover);
  }
  .conn-group-header-left {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .conn-group-header-right {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-shrink: 0;
  }
  .conn-group-label {
    font-size: 14px;
    font-weight: 700;
    color: var(--color-fg-0);
  }
  .conn-group-count {
    font-size: 11px;
    color: var(--color-fg-3);
    background: var(--color-bg-card);
    padding: 2px 8px;
    border-radius: 10px;
    border: 1px solid var(--color-border);
  }
  .conn-group-stat {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    color: var(--color-fg-2);
  }
  .conn-group-stat-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--color-fg-3);
  }
  .conn-group-stat-dot.active {
    background: var(--color-success);
  }
  :global(.conn-group-chevron) {
    color: var(--color-fg-3);
    transition: transform 0.2s;
    flex-shrink: 0;
  }
  :global(.conn-group-chevron.rotated) {
    transform: rotate(90deg);
  }
  .conn-group-body {
    display: flex;
    flex-direction: column;
    gap: 1px;
    background: var(--color-border-light);
  }
  .conn-card-nested {
    border-radius: 0 !important;
    border: none !important;
    border-bottom: 1px solid var(--color-border-light) !important;
    box-shadow: none !important;
  }
  .conn-card-nested:hover {
    box-shadow: none !important;
    border-color: var(--color-border-light) !important;
  }
  .conn-card-nested:last-child {
    border-bottom: none !important;
  }
  .conn-card {
    padding: 16px !important;
    transition: border-color 0.15s, box-shadow 0.15s;
  }
  .conn-card:hover {
    border-color: var(--color-primary);
    box-shadow: 0 2px 12px rgba(59,130,246,0.08);
  }
  .conn-inactive {
    opacity: 0.65;
  }
  .conn-inactive:hover {
    opacity: 1;
  }

  /* Card header */
  .conn-card-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }
  .conn-status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--color-fg-3);
    flex-shrink: 0;
    transition: background 0.2s;
  }
  .conn-status-dot.active {
    background: var(--color-success);
    box-shadow: 0 0 6px rgba(16,185,129,0.4);
  }
  .conn-card-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--color-fg-0);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
    min-width: 0;
  }
  .conn-format-badge {
    font-size: 10px;
    padding: 2px 7px;
    background: var(--color-purple-light);
    color: var(--color-purple);
    flex-shrink: 0;
  }
  .conn-card-priority {
    font-size: 11px;
    font-weight: 600;
    color: var(--color-fg-3);
    font-family: var(--font-mono);
    flex-shrink: 0;
  }

  /* Card meta */
  .conn-card-meta {
    margin-bottom: 12px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .conn-card-key {
    font-size: 11px;
    color: var(--color-fg-3);
    font-family: var(--font-mono);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .conn-card-oauth {
    font-size: 11px;
    color: var(--color-purple);
  }
  .conn-card-meta-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .conn-card-url {
    font-size: 11px;
    color: var(--color-fg-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 260px;
  }
  .conn-pool-badge {
    font-size: 10px;
    padding: 1px 6px;
    background: var(--color-primary-light);
    color: var(--color-primary);
    font-family: var(--font-mono);
    flex-shrink: 0;
  }
  .conn-card-models {
    font-size: 11px;
    color: var(--color-fg-3);
    margin-left: auto;
    flex-shrink: 0;
  }

  /* Card actions row */
  .conn-card-actions {
    display: flex;
    align-items: center;
    gap: 6px;
    padding-top: 12px;
    border-top: 1px solid var(--color-border-light);
  }
  .conn-action-btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 5px 10px;
    font-size: 12px;
    white-space: nowrap;
  }
  .conn-kebab-btn {
    font-size: 14px;
    letter-spacing: 1px;
    padding: 5px 8px;
  }
  .conn-spinner {
    display: inline-block;
    width: 12px;
    height: 12px;
    border: 2px solid var(--color-border);
    border-top-color: var(--color-primary);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  /* Dropdown */
  .conn-dropdown {
    position: absolute;
    right: 0;
    top: calc(100% + 4px);
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: 8px;
    box-shadow: 0 8px 24px rgba(0,0,0,0.15);
    min-width: 160px;
    z-index: 100;
    padding: 4px;
    animation: fadeInScale 0.15s ease-out;
  }
  .conn-dropdown-item {
    all: unset;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 10px;
    width: 100%;
    box-sizing: border-box;
    cursor: pointer;
    font-size: 12px;
    color: var(--color-fg-1);
    border-radius: 4px;
    transition: background 0.1s;
  }
  .conn-dropdown-item:hover {
    background: var(--color-bg-sidebar-hover);
  }
  .conn-dropdown-danger {
    color: var(--color-error);
  }
  .conn-dropdown-danger:hover {
    background: rgba(239,68,68,0.08);
  }
  .conn-dropdown-divider {
    height: 1px;
    background: var(--color-border);
    margin: 4px 0;
  }

  /* Responsive */
  @media (max-width: 640px) {
    .conn-toolbar {
      flex-direction: column;
      align-items: stretch;
      gap: 8px;
      padding: 12px 14px;
    }
    .conn-toolbar-left {
      justify-content: space-between;
    }
    .conn-toolbar-center {
      max-width: none;
    }
    .conn-toolbar-right {
      justify-content: flex-end;
    }
    .conn-toolbar-btn-label {
      display: none;
    }
    .conn-card-actions {
      flex-wrap: wrap;
    }
    .conn-action-btn span:not(.conn-spinner) {
      display: none;
    }
    .conn-card-url {
      max-width: 160px;
    }
  }
</style>
