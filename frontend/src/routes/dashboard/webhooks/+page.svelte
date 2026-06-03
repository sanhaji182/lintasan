<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import StatusBadge from '$lib/components/StatusBadge.svelte';
  import {
    Webhook, Plus, Trash2, X, Save, Send, ToggleLeft, ToggleRight,
    Globe, Bell, AlertCircle, CheckCircle2, Clock, Check
  } from 'lucide-svelte/icons';

  interface WebhookRecord {
    id: string;
    name: string;
    url: string;
    events: string[];
    active: boolean;
    action: string;
  }

  let webhooks = $state<WebhookRecord[]>([]);
  let loading = $state(true);
  let error = $state('');
  let saving = $state(false);
  let showCreateForm = $state(false);
  let testingId = $state<string | null>(null);

  // Create/edit form
  let formUrl = $state('');
  let formEvents = $state<string[]>(['request.completed']);
  let formSecret = $state('');

  const availableEvents = [
    { value: 'request.completed', label: 'Request Completed' },
    { value: 'request.failed', label: 'Request Failed' },
    { value: 'key.expired', label: 'Key Expired' },
    { value: 'provider.down', label: 'Provider Down' },
    { value: 'provider.recovered', label: 'Provider Recovered' },
    { value: 'usage.threshold', label: 'Usage Threshold' },
    { value: 'cache.cleared', label: 'Cache Cleared' },
  ];

  async function loadWebhooks() {
    try {
      const res = await api.get<any>('/api/webhooks');
      const raw = res?.data?.webhooks || res?.webhooks || res?.data || [];
      webhooks = Array.isArray(raw) ? raw.map((w: any) => ({
        id: w.id || '',
        name: w.name || '',
        url: w.url || '',
        events: Array.isArray(w.events) ? w.events : [],
        active: !!w.active,
        action: w.action || ''
      })) : [];
    } catch (e: any) {
      console.error('loadWebhooks error:', e);
      error = e.message || 'Failed to load webhooks';
    }
  }

  onMount(async () => {
    loading = true;
    await loadWebhooks();
    loading = false;
  });

  function resetForm() {
    formUrl = '';
    formEvents = ['request.completed'];
    formSecret = '';
    showCreateForm = false;
  }

  async function createWebhook() {
    if (!formUrl.trim()) return;
    saving = true;
    try {
      const res = await api.post<{ data: WebhookRecord }>('/api/webhooks', {
        url: formUrl.trim(),
        events: formEvents,
        secret: formSecret.trim() || undefined
      });
      const webhook = res.data || res as any;
      webhooks = [...webhooks, webhook];
      resetForm();
    } catch (e: any) {
      error = e.message || 'Failed to create webhook';
    }
    saving = false;
  }

  async function toggleWebhookStatus(webhook: WebhookRecord) {
    const newActive = !webhook.active;
    try {
      await api.patch(`/api/webhooks/${webhook.id}`, { active: newActive });
      webhooks = webhooks.map(w => w.id === webhook.id ? { ...w, active: newActive } : w);
    } catch (e: any) {
      error = e.message || 'Failed to update webhook';
    }
  }

  async function testWebhook(webhookId: string) {
    testingId = webhookId;
    try {
      await api.post(`/api/webhooks/${webhookId}/test`);
    } catch (e: any) {
      error = e.message || 'Failed to test webhook';
    }
    testingId = null;
  }

  async function deleteWebhook(webhookId: string) {
    if (!confirm('Are you sure you want to delete this webhook?')) return;
    try {
      await api.delete(`/api/webhooks/${webhookId}`);
      webhooks = webhooks.filter(w => w.id !== webhookId);
    } catch (e: any) {
      error = e.message || 'Failed to delete webhook';
    }
  }

  function toggleEvent(event: string) {
    if (formEvents.includes(event)) {
      formEvents = formEvents.filter(e => e !== event);
    } else {
      formEvents = [...formEvents, event];
    }
  }

  function formatTime(ts: string): string {
    try {
      const diff = Date.now() - new Date(ts).getTime();
      const mins = Math.floor(diff / 60000);
      if (mins < 1) return 'just now';
      if (mins < 60) return `${mins}m ago`;
      const hrs = Math.floor(mins / 60);
      if (hrs < 24) return `${hrs}h ago`;
      return `${Math.floor(hrs / 24)}d ago`;
    } catch {
      return ts;
    }
  }

  function truncateUrl(url: string): string {
    try {
      const u = new URL(url);
      return u.hostname + u.pathname.slice(0, 20);
    } catch {
      return url.length > 40 ? url.slice(0, 40) + '...' : url;
    }
  }
</script>

<svelte:head>
  <title>Webhooks — Lintasan</title>
</svelte:head>

<div style="display: flex; flex-direction: column; gap: 24px;">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-2.5">
      <div
        class="flex items-center justify-center rounded-xl"
        style="width: 40px; height: 40px; background: var(--color-warning-light);"
      >
        <Webhook size={20} style="color: var(--color-warning);" stroke-width={1.8} />
      </div>
      <div>
        <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">Webhooks</div>
        <div style="font-size: 12px; color: var(--color-fg-3);">Receive HTTP callbacks for gateway events</div>
      </div>
    </div>
    <button
      class="btn-primary flex items-center gap-1.5"
      onclick={() => { resetForm(); showCreateForm = !showCreateForm; }}
    >
      <Plus size={14} stroke-width={2} />
      Add Webhook
    </button>
  </div>

  <!-- Create Form -->
  {#if showCreateForm}
    <div class="card" style="animation: fadeInUp 0.3s ease-out;">
      <div class="flex items-center justify-between" style="margin-bottom: 16px;">
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">New Webhook</div>
        <button class="btn-icon" onclick={resetForm}>
          <X size={16} style="color: var(--color-fg-3);" />
        </button>
      </div>
      <div class="flex flex-col gap-4">
        <div>
          <!-- svelte-ignore a11y_label_has_associated_control -->
          <label style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 6px;">
            Endpoint URL
          </label>
          <input
            class="input-field"
            placeholder="https://example.com/webhook"
            bind:value={formUrl}
          />
        </div>

        <div>
          <!-- svelte-ignore a11y_label_has_associated_control -->
          <label style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 6px;">
            Secret (optional)
          </label>
          <input
            class="input-field"
            placeholder="Signing secret for payload verification"
            bind:value={formSecret}
          />
        </div>

        <div>
          <!-- svelte-ignore a11y_label_has_associated_control -->
          <label style="font-size: 12px; font-weight: 500; color: var(--color-fg-2); display: block; margin-bottom: 8px;">
            Events
          </label>
          <div class="flex flex-wrap gap-2">
            {#each availableEvents as evt}
              <button
                class="event-chip"
                class:selected={formEvents.includes(evt.value)}
                onclick={() => toggleEvent(evt.value)}
              >
                {#if formEvents.includes(evt.value)}
                  <CheckCircle2 size={12} />
                {:else}
                  <Bell size={12} />
                {/if}
                {evt.label}
              </button>
            {/each}
          </div>
        </div>

        <div class="flex items-center gap-2">
          <button
            class="btn-primary flex items-center gap-1.5"
            onclick={createWebhook}
            disabled={saving || !formUrl.trim() || formEvents.length === 0}
          >
            <Save size={14} />
            {saving ? 'Creating...' : 'Create Webhook'}
          </button>
          <button class="btn-secondary" onclick={resetForm}>Cancel</button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Webhooks List -->
  {#if loading}
    <Spinner />
  {:else if webhooks.length === 0}
    <div class="card">
      <EmptyState
        icon={Webhook}
        title="No webhooks configured"
        description="Set up webhooks to receive HTTP callbacks when gateway events occur."
      />
    </div>
  {:else}
    <div style="display: flex; flex-direction: column; gap: 12px;">
      {#each webhooks as webhook, i (webhook.id)}
        <div class="card webhook-card" style="animation: fadeInUp {0.3 + i * 0.05}s ease-out;">
          <div class="flex items-start justify-between">
            <!-- Left: Webhook Info -->
            <div class="flex items-start gap-3" style="flex: 1; min-width: 0;">
              <div
                class="flex items-center justify-center rounded-lg"
                style="width: 36px; height: 36px; background: {webhook.active ? 'var(--color-success-light)' : 'var(--color-warning-light)'}; flex-shrink: 0;"
              >
                <Globe
                  size={18}
                  style="color: {webhook.active ? 'var(--color-success)' : 'var(--color-warning)'};"
                />
              </div>
              <div style="flex: 1; min-width: 0;">
                <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 2px;">
                  {webhook.name || 'Webhook'}
                </div>
                <div class="font-mono" style="font-size: 13px; font-weight: 500; color: var(--color-fg-2); word-break: break-all;">
                  {webhook.url}
                </div>
                <div class="flex items-center gap-3" style="margin-top: 6px; flex-wrap: wrap;">
                  <!-- Events -->
                  <div class="flex items-center gap-1 flex-wrap">
                    {#each webhook.events.slice(0, 3) as evt}
                      <span class="badge" style="font-size: 10px; padding: 2px 8px; background: var(--color-info-light); color: var(--color-info);">
                        {evt}
                      </span>
                    {/each}
                    {#if webhook.events.length > 3}
                      <span class="badge" style="font-size: 10px; padding: 2px 8px; background: var(--color-border-light); color: var(--color-fg-3);">
                        +{webhook.events.length - 3} more
                      </span>
                    {/if}
                  </div>
                </div>
                <div class="flex items-center gap-4" style="margin-top: 8px;">
                </div>
              </div>
            </div>

            <!-- Right: Actions -->
            <div class="flex items-center gap-1" style="flex-shrink: 0; margin-left: 12px;">
              <StatusBadge status={webhook.active ? 'active' : 'inactive'} />
              <button
                class="btn-icon"
                style="color: {webhook.active ? 'var(--color-warning)' : 'var(--color-success)'};"
                onclick={() => toggleWebhookStatus(webhook)}
                title={webhook.active ? 'Disable webhook' : 'Enable webhook'}
              >
                {#if webhook.active}
                  <ToggleRight size={20} />
                {:else}
                  <ToggleLeft size={20} />
                {/if}
              </button>
              <button
                class="btn-icon"
                style="color: var(--color-info);"
                onclick={() => testWebhook(webhook.id)}
                disabled={testingId === webhook.id}
                title="Send test event"
              >
                <Send size={14} class={testingId === webhook.id ? 'spin-icon' : ''} />
              </button>
              <button
                class="btn-icon"
                style="color: var(--color-error);"
                onclick={() => deleteWebhook(webhook.id)}
                title="Delete webhook"
              >
                <Trash2 size={14} />
              </button>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}

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
  .webhook-card {
    transition: var(--transition);
  }
  .webhook-card:hover {
    border-color: var(--color-primary);
    box-shadow: 0 0 0 3px var(--color-primary-glow);
  }
  .event-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 5px 12px;
    border-radius: 9999px;
    border: 1px solid var(--color-border);
    background: transparent;
    color: var(--color-fg-2);
    font-size: 12px;
    font-weight: 500;
    cursor: pointer;
    transition: var(--transition);
  }
  .event-chip:hover {
    border-color: var(--color-primary);
    background: var(--color-primary-light);
  }
  .event-chip.selected {
    border-color: var(--color-primary);
    background: var(--color-primary-light);
    color: var(--color-primary);
  }
  .btn-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: var(--radius-sm);
    border: none;
    background: transparent;
    cursor: pointer;
    transition: var(--transition);
  }
  .btn-icon:hover {
    background: var(--color-bg-sidebar-hover);
  }
  .btn-icon:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  :global(.spin-icon) {
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
