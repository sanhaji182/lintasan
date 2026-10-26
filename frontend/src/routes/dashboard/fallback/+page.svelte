<script lang="ts">
  import TabNav from '$lib/components/TabNav.svelte';
  const __tabs = [
    { label: 'Routing', path: '/dashboard/routing' },
    { label: 'Fallback', path: '/dashboard/fallback' }
  ];
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { confirmAction } from '$lib/confirm';
  import { showToast } from '$lib/toast';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import ChainBuilder from './ChainBuilder.svelte';
  import {
    ShieldAlert, Plus, Trash2, ArrowDown, Link2,
    Brain, Server, Save, AlertTriangle
  } from 'lucide-svelte/icons';

  interface FallbackChain {
    id: string;
    name: string;
    type: 'model' | 'connection';
    chain: string[];
    usage_count: number;
  }

  let modelChains = $state<FallbackChain[]>([]);
  let connectionChains = $state<FallbackChain[]>([]);
  let loading = $state(true);
  let error = $state('');
  let saving = $state(false);

  // New chain forms
  let showModelForm = $state(false);
  let showConnectionForm = $state(false);
  let newModelName = $state('');
  let newModelChainRows = $state<string[]>(['', '']);
  let newConnectionName = $state('');
  let newConnectionChainRows = $state<string[]>(['', '']);
  let modelFormError = $state('');
  let connectionFormError = $state('');

  // Catalog used for suggestions/pickers
  let connections = $state<any[]>([]);
  let discoveredModels = $state<any[]>([]);
  let aliases = $state<Record<string, unknown>>({});
  let combos = $state<any[]>([]);
  let catalogLoading = $state(true);

  const connectionOptions = $derived(connections.map(connection => ({
    value: String(connection.id),
    label: `${connection.name || connection.id} — ${connection.id}${connection.is_active === 0 || connection.is_active === false ? ' (inactive)' : ''}`,
  })));

  const modelOptions = $derived.by(() => {
    const ids = new Set<string>();
    for (const model of discoveredModels) {
      const id = String(model?.model_id || '').trim();
      if (id) ids.add(id);
    }
    for (const alias of Object.keys(aliases)) ids.add(alias);
    for (const combo of combos) {
      const id = String(combo?.name || combo?.provider || '').trim();
      if (id) ids.add(id);
    }
    return [...ids].sort((a, b) => a.localeCompare(b)).map(value => ({ value }));
  });

  function normalizeChain(raw: any, type: 'model' | 'connection'): FallbackChain {
    const chain = Array.isArray(raw?.chain) ? raw.chain.map((item: any) => String(item)) : [];
    return {
      id: String(raw?.id || raw?.name || ''),
      name: String(raw?.name || 'Unnamed'),
      type,
      chain,
      usage_count: Number(raw?.usage_count) || 0,
    };
  }

  async function loadChains() {
    try {
      const res = await api.get<{ data: { model_chains?: any[]; connection_chains?: any[] } }>('/api/fallback');
      const data = res.data || {};
      modelChains = (data.model_chains || []).map(chain => normalizeChain(
        Array.isArray(chain.chain) ? chain : { ...chain, chain: chain.models },
        'model',
      ));
      connectionChains = (data.connection_chains || []).map(chain => normalizeChain(
        Array.isArray(chain.chain) ? chain : { ...chain, chain: chain.connections },
        'connection',
      ));
    } catch (e: any) {
      error = e.message || 'Failed to load fallback chains';
    }
  }

  async function loadCatalog() {
    catalogLoading = true;
    const [connectionsRes, modelsRes, aliasesRes, combosRes] = await Promise.all([
      api.get<any>('/api/connections').catch(() => ({ data: [] })),
      api.get<any>('/api/models/discovered').catch(() => ({ data: [] })),
      api.get<any>('/api/aliases').catch(() => ({ data: {} })),
      api.get<any>('/api/combos').catch(() => ({ data: [] })),
    ]);
    connections = Array.isArray(connectionsRes?.data) ? connectionsRes.data : [];
    discoveredModels = Array.isArray(modelsRes?.data) ? modelsRes.data : [];
    aliases = aliasesRes?.data && typeof aliasesRes.data === 'object' ? aliasesRes.data : {};
    const rawCombos = combosRes?.data || combosRes?.combos || [];
    combos = Array.isArray(rawCombos) ? rawCombos : [];
    catalogLoading = false;
  }

  onMount(async () => {
    loading = true;
    await loadChains();
    loading = false;
    void loadCatalog();
  });

  function openModelForm() {
    showModelForm = !showModelForm;
    if (showModelForm) {
      showConnectionForm = false;
      newModelName = '';
      newModelChainRows = ['', ''];
      modelFormError = '';
    }
  }

  function openConnectionForm() {
    showConnectionForm = !showConnectionForm;
    if (showConnectionForm) {
      showModelForm = false;
      newConnectionName = '';
      newConnectionChainRows = ['', ''];
      connectionFormError = '';
    }
  }

  function resetModelForm() {
    showModelForm = false;
    newModelName = '';
    newModelChainRows = ['', ''];
    modelFormError = '';
  }

  function resetConnectionForm() {
    showConnectionForm = false;
    newConnectionName = '';
    newConnectionChainRows = ['', ''];
    connectionFormError = '';
  }

  function chainItems(rows: string[]) {
    return rows.map(row => row.trim()).filter(Boolean);
  }

  function validateChain(name: string, rows: string[]) {
    if (!name.trim()) return 'Chain name is required.';
    const items = chainItems(rows);
    if (items.length < 2) return 'Add at least two steps: a primary and one fallback.';
    if (new Set(items).size !== items.length) return 'Each step must be unique.';
    return '';
  }

  async function createModelChain() {
    const problem = validateChain(newModelName, newModelChainRows);
    if (problem) {
      modelFormError = problem;
      return;
    }
    modelFormError = '';
    saving = true;
    try {
      const name = newModelName.trim();
      const data = await api.post<{ chain: any }>('/api/fallback/model-chains', {
        name,
        chain: chainItems(newModelChainRows),
      });
      modelChains = [...modelChains, normalizeChain(data.chain, 'model')];
      showToast(`Model fallback chain “${name}” created`, 'success');
      resetModelForm();
    } catch (e: any) {
      modelFormError = e.message || 'Failed to create model chain';
    } finally {
      saving = false;
    }
  }

  async function createConnectionChain() {
    const problem = validateChain(newConnectionName, newConnectionChainRows);
    if (problem) {
      connectionFormError = problem;
      return;
    }
    connectionFormError = '';
    saving = true;
    try {
      const name = newConnectionName.trim();
      const data = await api.post<{ chain: any }>('/api/fallback/connection-chains', {
        name,
        chain: chainItems(newConnectionChainRows),
      });
      connectionChains = [...connectionChains, normalizeChain(data.chain, 'connection')];
      showToast(`Connection fallback chain “${name}” created`, 'success');
      resetConnectionForm();
    } catch (e: any) {
      connectionFormError = e.message || 'Failed to create connection chain';
    } finally {
      saving = false;
    }
  }

  async function deleteModelChain(chain: FallbackChain) {
    if (!(await confirmAction({
      title: `Delete model chain “${chain.name}”?`,
      detail: 'Requests using this chain will lose its model fallbacks.',
      confirmLabel: 'Delete chain',
      tone: 'danger',
    }))) return;
    try {
      await api.delete(`/api/fallback/model-chains/${encodeURIComponent(chain.id)}`);
      modelChains = modelChains.filter(item => item.id !== chain.id);
      showToast(`Model fallback chain “${chain.name}” deleted`, 'success');
    } catch (e: any) {
      error = e.message || 'Failed to delete chain';
    }
  }

  async function deleteConnectionChain(chain: FallbackChain) {
    if (!(await confirmAction({
      title: `Delete connection chain “${chain.name}”?`,
      detail: 'Requests using this chain will lose its connection fallbacks.',
      confirmLabel: 'Delete chain',
      tone: 'danger',
    }))) return;
    try {
      await api.delete(`/api/fallback/connection-chains/${encodeURIComponent(chain.id)}`);
      connectionChains = connectionChains.filter(item => item.id !== chain.id);
      showToast(`Connection fallback chain “${chain.name}” deleted`, 'success');
    } catch (e: any) {
      error = e.message || 'Failed to delete chain';
    }
  }

  function connectionLabel(id: string) {
    const name = connections.find(connection => String(connection.id) === id)?.name;
    return name ? `${name} · ${id}` : id;
  }
</script>

<TabNav tabs={__tabs} />


<div style="display: flex; flex-direction: column; gap: 24px;">
  <!-- Model Fallback Chains -->
  <div class="card">
    <div class="flex items-center justify-between" style="margin-bottom: 20px;">
      <div class="flex items-center gap-2.5">
        <div
          class="flex items-center justify-center rounded-xl"
          style="width: 40px; height: 40px; background: var(--color-info-light);"
        >
          <Brain size={20} style="color: var(--color-info);" stroke-width={1.8} />
        </div>
        <div>
          <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">Model Fallback Chains</div>
          <div style="font-size: 12px; color: var(--color-fg-3);">When a model fails, try the next in the chain.</div>
        </div>
      </div>
      <button
        class="btn-secondary flex items-center gap-1.5"
        onclick={openModelForm}
      >
        <Plus size={14} stroke-width={2} />
        New Chain
      </button>
    </div>

    {#if showModelForm}
      <div class="chain-form">
        <div class="form-heading">Create Model Fallback Chain</div>
        <div class="form-grid">
          <label class="form-field">
            <span>Chain name</span>
            <input class="input-field" placeholder="e.g. GPT Fallback" bind:value={newModelName} />
          </label>
          <div class="form-field">
            <span>Ordered steps</span>
            {#if catalogLoading}
              <div class="catalog-hint" role="status"><Spinner /> Loading model catalog…</div>
            {/if}
            <ChainBuilder
              bind:items={newModelChainRows}
              options={modelOptions}
              placeholder="Model ID (e.g. gpt-4o)"
              listId="fallback-model-options"
              addLabel="Add model"
            />
            <small>First step is the primary; the rest are tried in order. Suggestions come from discovered models, aliases, and combos.</small>
          </div>
          {#if modelFormError}
            <div class="form-error" role="alert"><AlertTriangle size={16} /> <span>{modelFormError}</span></div>
          {/if}
          <div class="form-actions">
            <button class="btn-primary flex items-center gap-1.5" onclick={createModelChain} disabled={saving}>
              <Save size={14} stroke-width={2} />
              {saving ? 'Creating...' : 'Create Chain'}
            </button>
            <button class="btn-secondary" onclick={resetModelForm}>Cancel</button>
          </div>
        </div>
      </div>
    {/if}

    {#if loading}
      <Spinner />
    {:else if modelChains.length === 0 && !showModelForm}
      <EmptyState
        icon={Brain}
        title="No model fallback chains"
        description="Create chains to automatically fall back to alternative models when one fails."
        action={openModelForm}
        actionLabel="Create chain"
      />
    {:else}
      <div style="display: flex; flex-direction: column; gap: 12px;">
        {#each modelChains as chain (chain.id)}
          <div class="chain-card">
            <div class="flex items-center justify-between" style="margin-bottom: 12px;">
              <div class="flex items-center gap-2">
                <div
                  class="flex items-center justify-center rounded-lg"
                  style="width: 28px; height: 28px; background: var(--color-info-light);"
                >
                  <Brain size={14} style="color: var(--color-info);" />
                </div>
                <span style="font-size: 13px; font-weight: 600; color: var(--color-fg-0);">{chain.name}</span>
                <span style="font-size: 11px; color: var(--color-fg-3); font-family: var(--font-mono);">({chain.usage_count} uses)</span>
              </div>
              <button
                class="btn-icon"
                style="color: var(--color-error);"
                onclick={() => deleteModelChain(chain)}
                title="Delete chain"
                aria-label={`Delete model chain ${chain.name}`}
              >
                <Trash2 size={14} />
              </button>
            </div>
            {#if chain.chain.length > 0}
            <div class="chain-flow">
              {#each chain.chain as model, i}
                {#if i > 0}
                  <div class="chain-arrow">
                    <ArrowDown size={14} style="color: var(--color-fg-3);" />
                  </div>
                {/if}
                <div class="chain-step" class:primary={i === 0}>
                  <span class="font-mono" style="font-size: 12px; font-weight: {i === 0 ? 600 : 400};">
                    {model}
                  </span>
                  {#if i === 0}
                    <span class="badge badge-info" style="font-size: 9px; padding: 2px 6px;">PRIMARY</span>
                  {:else}
                    <span class="badge" style="font-size: 9px; padding: 2px 6px; background: var(--color-border-light); color: var(--color-fg-3);">FALLBACK {i}</span>
                  {/if}
                </div>
              {/each}
            </div>
            {:else}
              <div style="font-size: 12px; color: var(--color-fg-3); padding: 8px;">No chain items configured</div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Connection Fallback Chains -->
  <div class="card">
    <div class="flex items-center justify-between" style="margin-bottom: 20px;">
      <div class="flex items-center gap-2.5">
        <div
          class="flex items-center justify-center rounded-xl"
          style="width: 40px; height: 40px; background: var(--color-warning-light);"
        >
          <Link2 size={20} style="color: var(--color-warning);" stroke-width={1.8} />
        </div>
        <div>
          <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">Connection Fallback Chains</div>
          <div style="font-size: 12px; color: var(--color-fg-3);">When a connection fails, route to the next available.</div>
        </div>
      </div>
      <button
        class="btn-secondary flex items-center gap-1.5"
        onclick={openConnectionForm}
      >
        <Plus size={14} stroke-width={2} />
        New Chain
      </button>
    </div>

    {#if showConnectionForm}
      <div class="chain-form">
        <div class="form-heading">Create Connection Fallback Chain</div>
        <div class="form-grid">
          <label class="form-field">
            <span>Chain name</span>
            <input class="input-field" placeholder="e.g. Primary Connections" bind:value={newConnectionName} />
          </label>
          <div class="form-field">
            <span>Ordered connections</span>
            {#if catalogLoading}
              <div class="catalog-hint" role="status"><Spinner /> Loading connections…</div>
            {:else if connections.length === 0}
              <div class="catalog-hint warning" role="status">
                <AlertTriangle size={15} />
                <span>No connections configured. <a href="/dashboard/connections">Open Connections</a></span>
              </div>
            {/if}
            <ChainBuilder
              bind:items={newConnectionChainRows}
              mode="select"
              options={connectionOptions}
              addLabel="Add connection"
            />
            <small>First connection is primary. The runtime uses connection IDs, so pick them from the list.</small>
          </div>
          {#if connectionFormError}
            <div class="form-error" role="alert"><AlertTriangle size={16} /> <span>{connectionFormError}</span></div>
          {/if}
          <div class="form-actions">
            <button class="btn-primary flex items-center gap-1.5" onclick={createConnectionChain} disabled={saving}>
              <Save size={14} stroke-width={2} />
              {saving ? 'Creating...' : 'Create Chain'}
            </button>
            <button class="btn-secondary" onclick={resetConnectionForm}>Cancel</button>
          </div>
        </div>
      </div>
    {/if}

    {#if loading}
      <Spinner />
    {:else if connectionChains.length === 0 && !showConnectionForm}
      <EmptyState
        icon={Link2}
        title="No connection fallback chains"
        description="Create chains to automatically route to backup connections when one is unavailable."
        action={openConnectionForm}
        actionLabel="Create chain"
      />
    {:else}
      <div style="display: flex; flex-direction: column; gap: 12px;">
        {#each connectionChains as chain (chain.id)}
          <div class="chain-card">
            <div class="flex items-center justify-between" style="margin-bottom: 12px;">
              <div class="flex items-center gap-2">
                <div
                  class="flex items-center justify-center rounded-lg"
                  style="width: 28px; height: 28px; background: var(--color-warning-light);"
                >
                  <Server size={14} style="color: var(--color-warning);" />
                </div>
                <span style="font-size: 13px; font-weight: 600; color: var(--color-fg-0);">{chain.name}</span>
                <span style="font-size: 11px; color: var(--color-fg-3); font-family: var(--font-mono);">({chain.usage_count} uses)</span>
              </div>
              <button
                class="btn-icon"
                style="color: var(--color-error);"
                onclick={() => deleteConnectionChain(chain)}
                title="Delete chain"
                aria-label={`Delete connection chain ${chain.name}`}
              >
                <Trash2 size={14} />
              </button>
            </div>
            {#if chain.chain.length > 0}
            <div class="chain-flow">
              {#each chain.chain as conn, i}
                {#if i > 0}
                  <div class="chain-arrow">
                    <ArrowDown size={14} style="color: var(--color-fg-3);" />
                  </div>
                {/if}
                <div class="chain-step" class:primary={i === 0}>
                  <span class="font-mono" style="font-size: 12px; font-weight: {i === 0 ? 600 : 400};">
                    {connectionLabel(conn)}
                  </span>
                  {#if i === 0}
                    <span class="badge badge-warning" style="font-size: 9px; padding: 2px 6px;">PRIMARY</span>
                  {:else}
                    <span class="badge" style="font-size: 9px; padding: 2px 6px; background: var(--color-border-light); color: var(--color-fg-3);">FALLBACK {i}</span>
                  {/if}
                </div>
              {/each}
            </div>
            {:else}
              <div style="font-size: 12px; color: var(--color-fg-3); padding: 8px;">No chain items configured</div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>

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
      <ShieldAlert size={16} />
      {error}
      <button style="margin-left: auto; cursor: pointer; color: var(--color-error); background: none; border: none;" onclick={() => error = ''} aria-label="Dismiss error">&times;</button>
    </div>
  {/if}
</div>

<style>
  .chain-form {
    padding: 16px;
    background: var(--color-bg-body);
    border: 1px dashed var(--color-border);
    border-radius: var(--radius-sm);
    margin-bottom: 16px;
  }
  .form-heading { font-size: 13px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 12px; }
  .form-grid { display: flex; flex-direction: column; gap: 12px; }
  .form-field { display: flex; flex-direction: column; gap: 6px; }
  .form-field > span { font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.4px; color: var(--color-fg-3); }
  .form-field small { font-size: 11px; color: var(--color-fg-3); line-height: 1.4; }
  .form-actions { display: flex; align-items: center; gap: 8px; }
  .form-error {
    display: flex; align-items: center; gap: 8px;
    padding: 9px 12px; border: 1px solid color-mix(in srgb, var(--color-error) 35%, transparent);
    border-radius: 8px; background: var(--color-error-light); color: var(--color-error);
    font-size: 12px; font-weight: 600;
  }
  .catalog-hint {
    display: flex; align-items: center; gap: 8px;
    padding: 8px 10px; border: 1px dashed var(--color-border);
    border-radius: 8px; color: var(--color-fg-3); font-size: 11px;
  }
  .catalog-hint.warning { color: var(--color-fg-2); border-color: color-mix(in srgb, var(--color-warning) 50%, var(--color-border)); }
  .catalog-hint a { color: var(--color-primary); }
  .chain-card {
    padding: 16px;
    background: var(--color-bg-body);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    transition: var(--transition);
  }
  .chain-card:hover {
    box-shadow: var(--shadow-sm);
  }
  .chain-flow {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 0;
  }
  .chain-step {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 8px 12px;
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    color: var(--color-fg-1);
  }
  .chain-step.primary {
    border-color: var(--color-primary);
    background: var(--color-primary-light);
  }
  .chain-arrow {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2px 0;
    color: var(--color-fg-3);
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
</style>
