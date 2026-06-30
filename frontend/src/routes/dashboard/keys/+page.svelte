<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { showToast } from '$lib/toast';
  import { Key, Plus, Copy, Trash2, X, Check, AlertCircle } from 'lucide-svelte/icons';

  let keys = $state<any[]>([]);
  let loading = $state(true);
  let error = $state('');
  let showForm = $state(false);
  let creating = $state(false);
  let newKeyName = $state('');
  let copiedId = $state<string | null>(null);

  const summary = $derived({
    total: keys.length,
    active: keys.filter(k => k.is_active !== false).length
  });

  onMount(async () => {
    try {
      const res = await api.get<any>('/api/keys');
      keys = res.data || [];
    } catch (e: any) {
      error = e.message || 'Failed to load API keys';
    }
    finally { loading = false; }
  });

  async function createKey() {
    if (!newKeyName.trim()) return;
    creating = true;
    try {
      const res = await api.post<any>('/api/keys', { action: 'create', name: newKeyName });
      const updated = await api.get<any>('/api/keys');
      keys = updated.data || [];
      showForm = false;
      newKeyName = '';
      showToast('API key created successfully', 'success');
    } catch (e: any) {
      showToast(e.message || 'Failed to create API key', 'error');
    } finally {
      creating = false;
    }
  }

  async function copyKey(key: string) {
    await navigator.clipboard.writeText(key);
    copiedId = key;
    setTimeout(() => copiedId = null, 2000);
  }

  async function deleteKey(id: string) {
    if (!confirm('Delete this key?')) return;
    try {
      await api.delete('/api/keys/' + id);
      keys = keys.filter(k => k.id !== id);
      showToast('API key deleted', 'success');
    } catch (e: any) {
      showToast(e.message || 'Failed to delete API key', 'error');
    }
  }
</script>

<div style="animation: fadeInUp 0.4s ease-out;">
  <!-- Summary strip -->
  <div class="card mb-5" style="padding: 0; overflow: hidden;">
    <div class="grid grid-cols-2" style="gap: 1px; background: var(--color-border);">
      {#each [
        { label: 'TOTAL KEYS', value: summary.total },
        { label: 'ACTIVE', value: summary.active }
      ] as stat}
        <div class="text-center" style="padding: 16px; background: var(--color-bg-card);">
          <div style="font-size: 11px; font-weight: 500; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">{stat.label}</div>
          <div style="font-size: 20px; font-weight: 700; color: var(--color-fg-0); font-family: var(--font-mono);">{stat.value}</div>
        </div>
      {/each}
    </div>
  </div>

  <div class="flex items-center justify-between mb-5">
    <h2 style="font-size: 18px; font-weight: 600; color: var(--color-fg-0);">API Keys</h2>
    <button class="btn-primary flex items-center gap-2" onclick={() => showForm = !showForm}>
      {#if showForm}<X size={16} />{:else}<Plus size={16} />{/if}
      {showForm ? 'Cancel' : 'Create Key'}
    </button>
  </div>

  {#if showForm}
    <div class="card mb-5 flex items-center gap-3" style="animation: fadeInScale 0.3s ease-out;">
      <input class="input-field" bind:value={newKeyName} placeholder="Key name..." style="flex: 1;" onkeydown={(e) => e.key === 'Enter' && createKey()} />
      <button class="btn-primary" onclick={createKey} disabled={creating}>{creating ? 'Creating...' : 'Create'}</button>
    </div>
  {/if}

  {#if loading}
    <Spinner />
  {:else if error}
    <div class="card"><EmptyState icon={AlertCircle} title="Failed to load API keys" description={error} action={async () => { loading = true; error = ''; try { const res = await api.get<any>('/api/keys'); keys = res.data || []; } catch (e: any) { error = e.message || 'Failed to load API keys'; } loading = false; }} actionLabel="Retry" /></div>
  {:else if keys.length === 0}
    <div class="card"><EmptyState icon={Key} title="No API keys" description="Create your first API key." /></div>
  {:else}
    <div class="card" style="padding: 0; overflow: hidden;">
      <div style="overflow-x: auto;">
        <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
          <thead>
            <tr>
              {#each ['Name', 'Key', 'Created', 'Actions'] as h}
                <th style="text-align: left; padding: 12px 16px; font-size: 11px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.5px; color: var(--color-fg-3); border-bottom: 1px solid var(--color-border);">{h}</th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each keys as k}
              <tr style="border-bottom: 1px solid var(--color-border-light);">
                <td style="padding: 12px 16px; font-weight: 500; color: var(--color-fg-0);">{k.name || 'Unnamed'}</td>
                <td style="padding: 12px 16px;">
                  <span style="font-family: var(--font-mono); font-size: 12px; background: var(--color-bg-body); padding: 3px 8px; border-radius: 4px; color: var(--color-fg-2);">{k.key || k.prefix || '***'}</span>
                </td>
                <td style="padding: 12px 16px; font-size: 12px; color: var(--color-fg-3);">{k.created_at ? new Date(k.created_at).toLocaleDateString() : '-'}</td>
                <td style="padding: 12px 16px;">
                  <div class="flex items-center gap-2">
                    <button class="btn-secondary" style="padding: 6px 10px; font-size: 12px;" onclick={() => copyKey(k.key || k.prefix || '')} title="Copy">
                      {#if copiedId === (k.key || k.prefix)}<Check size={14} style="color: var(--color-success);" />{:else}<Copy size={14} />{/if}
                    </button>
                    <button class="btn-secondary" style="padding: 6px 10px; font-size: 12px; color: var(--color-error);" onclick={() => deleteKey(k.id)} title="Delete">
                      <Trash2 size={14} />
                    </button>
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
