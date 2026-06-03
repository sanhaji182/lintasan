<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import {
    ShieldAlert, Plus, Trash2, ArrowDown, Link2,
    Brain, Server, ChevronRight, Save
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

  // New chain form
  let showModelForm = $state(false);
  let showConnectionForm = $state(false);
  let newModelName = $state('');
  let newModelChain = $state('');
  let newConnectionName = $state('');
  let newConnectionChain = $state('');

  async function loadChains() {
    try {
      const res = await api.get<{ data: { model_chains: any[]; connection_chains: any[] } }>('/api/fallback');
      const d = res.data || {};
      modelChains = (d.model_chains || []).map((c: any) => ({
        id: c.id || c.name,
        name: c.name || 'Unnamed',
        type: 'model' as const,
        chain: c.chain || c.models || [],
        usage_count: c.usage_count || 0
      }));
      connectionChains = (d.connection_chains || []).map((c: any) => ({
        id: c.id || c.name,
        name: c.name || 'Unnamed',
        type: 'connection' as const,
        chain: c.chain || c.connections || [],
        usage_count: c.usage_count || 0
      }));
    } catch (e: any) {
      error = e.message || 'Failed to load fallback chains';
    }
  }

  onMount(async () => {
    loading = true;
    await loadChains();
    loading = false;
  });

  async function createModelChain() {
    const items = newModelChain.split(',').map(s => s.trim()).filter(Boolean);
    if (!newModelName.trim() || items.length < 2) return;
    saving = true;
    try {
      const data = await api.post<{ chain: FallbackChain }>('/api/fallback/model-chains', {
        name: newModelName.trim(),
        chain: items
      });
      modelChains = [...modelChains, data.chain];
      newModelName = '';
      newModelChain = '';
      showModelForm = false;
    } catch (e: any) {
      error = e.message || 'Failed to create model chain';
    }
    saving = false;
  }

  async function createConnectionChain() {
    const items = newConnectionChain.split(',').map(s => s.trim()).filter(Boolean);
    if (!newConnectionName.trim() || items.length < 2) return;
    saving = true;
    try {
      const data = await api.post<{ chain: FallbackChain }>('/api/fallback/connection-chains', {
        name: newConnectionName.trim(),
        chain: items
      });
      connectionChains = [...connectionChains, data.chain];
      newConnectionName = '';
      newConnectionChain = '';
      showConnectionForm = false;
    } catch (e: any) {
      error = e.message || 'Failed to create connection chain';
    }
    saving = false;
  }

  async function deleteModelChain(id: string) {
    try {
      await api.delete(`/api/fallback/model-chains/${id}`);
      modelChains = modelChains.filter(c => c.id !== id);
    } catch (e: any) {
      error = e.message || 'Failed to delete chain';
    }
  }

  async function deleteConnectionChain(id: string) {
    try {
      await api.delete(`/api/fallback/connection-chains/${id}`);
      connectionChains = connectionChains.filter(c => c.id !== id);
    } catch (e: any) {
      error = e.message || 'Failed to delete chain';
    }
  }
</script>

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
        onclick={() => showModelForm = !showModelForm}
      >
        <Plus size={14} stroke-width={2} />
        New Chain
      </button>
    </div>

    {#if showModelForm}
      <div class="chain-form">
        <div style="font-size: 13px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 12px;">Create Model Fallback Chain</div>
        <div style="display: flex; flex-direction: column; gap: 10px;">
          <input
            class="input-field"
            placeholder="Chain name (e.g. GPT Fallback)"
            bind:value={newModelName}
          />
          <input
            class="input-field"
            placeholder="Models, comma-separated (e.g. gpt-4, gpt-3.5-turbo, claude-2)"
            bind:value={newModelChain}
          />
          <div style="font-size: 11px; color: var(--color-fg-3);">Enter model names separated by commas. First model is primary; others are fallbacks in order.</div>
          <div class="flex items-center gap-2">
            <button class="btn-primary" onclick={createModelChain} disabled={saving}>
              <Save size={14} stroke-width={2} />
              {saving ? 'Creating...' : 'Create Chain'}
            </button>
            <button class="btn-secondary" onclick={() => { showModelForm = false; newModelName = ''; newModelChain = ''; }}>
              Cancel
            </button>
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
                onclick={() => deleteModelChain(chain.id)}
                title="Delete chain"
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
            {/if}
            {#if chain.chain.length === 0}
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
        onclick={() => showConnectionForm = !showConnectionForm}
      >
        <Plus size={14} stroke-width={2} />
        New Chain
      </button>
    </div>

    {#if showConnectionForm}
      <div class="chain-form">
        <div style="font-size: 13px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 12px;">Create Connection Fallback Chain</div>
        <div style="display: flex; flex-direction: column; gap: 10px;">
          <input
            class="input-field"
            placeholder="Chain name (e.g. Primary Connections)"
            bind:value={newConnectionName}
          />
          <input
            class="input-field"
            placeholder="Connections, comma-separated (e.g. openai-main, openai-backup, azure-main)"
            bind:value={newConnectionChain}
          />
          <div style="font-size: 11px; color: var(--color-fg-3);">Enter connection names separated by commas. First is primary; others are fallbacks.</div>
          <div class="flex items-center gap-2">
            <button class="btn-primary" onclick={createConnectionChain} disabled={saving}>
              <Save size={14} stroke-width={2} />
              {saving ? 'Creating...' : 'Create Chain'}
            </button>
            <button class="btn-secondary" onclick={() => { showConnectionForm = false; newConnectionName = ''; newConnectionChain = ''; }}>
              Cancel
            </button>
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
                onclick={() => deleteConnectionChain(chain.id)}
                title="Delete chain"
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
                    {conn}
                  </span>
                  {#if i === 0}
                    <span class="badge badge-warning" style="font-size: 9px; padding: 2px 6px;">PRIMARY</span>
                  {:else}
                    <span class="badge" style="font-size: 9px; padding: 2px 6px; background: var(--color-border-light); color: var(--color-fg-3);">FALLBACK {i}</span>
                  {/if}
                </div>
              {/each}
            </div>
            {/if}
            {#if chain.chain.length === 0}
              <div style="font-size: 12px; color: var(--color-fg-3); padding: 8px;">No chain items configured</div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>

  {#if error}
    <div
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
  .chain-form {
    padding: 16px;
    background: var(--color-bg-body);
    border: 1px dashed var(--color-border);
    border-radius: var(--radius-sm);
    margin-bottom: 16px;
  }
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
