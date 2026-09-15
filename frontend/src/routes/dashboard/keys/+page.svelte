<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { captureOneTimeSecret, clearOneTimeSecret, maskedKeyLabel, type OneTimeSecret } from '$lib/api-key-secret';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { showToast } from '$lib/toast';
  import { Key, Plus, Copy, Trash2, X, Check, AlertCircle, ShieldAlert, LockKeyhole } from 'lucide-svelte/icons';

  let keys = $state<any[]>([]);
  let loading = $state(true);
  let error = $state('');
  let showForm = $state(false);
  let creating = $state(false);
  let newKeyName = $state('');
  let oneTimeSecret = $state<OneTimeSecret | null>(null);
  let secretCopied = $state(false);
  let secretAcknowledged = $state(false);

  const summary = $derived({
    total: keys.length,
    active: keys.filter(k => k.is_active !== false).length
  });

  async function loadKeys() {
    const res = await api.get<any>('/api/keys');
    keys = res.data || [];
  }

  onMount(async () => {
    try {
      await loadKeys();
    } catch (e: any) {
      error = e.message || 'Failed to load API keys';
    } finally {
      loading = false;
    }
  });

  async function createKey() {
    if (!newKeyName.trim() || creating) return;
    creating = true;
    try {
      const created = await api.post<any>('/api/keys', { action: 'create', name: newKeyName.trim() });
      oneTimeSecret = captureOneTimeSecret(created);
      if (!oneTimeSecret) throw new Error('The server did not return the new key. Revoke it and create another key.');
      secretCopied = false;
      secretAcknowledged = false;
      showForm = false;
      newKeyName = '';
      await loadKeys();
      showToast('API key created. Copy the one-time secret now.', 'success');
    } catch (e: any) {
      showToast(e.message || 'Failed to create API key', 'error');
    } finally {
      creating = false;
    }
  }

  async function copySecret(secret: string) {
    try {
      await navigator.clipboard.writeText(secret);
      secretCopied = true;
      showToast('Full API key copied', 'success');
    } catch {
      showToast('Could not copy automatically. Select and copy the key manually.', 'error');
    }
  }

  async function copyCurrentSecret() {
    if (oneTimeSecret) await copySecret(oneTimeSecret.value);
  }

  function dismissSecret() {
    if (!secretAcknowledged) return;
    oneTimeSecret = clearOneTimeSecret();
    secretCopied = false;
    secretAcknowledged = false;
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

<svelte:head><title>API Keys — Lintasan</title></svelte:head>

<div class="keys-page">
  <section class="hero-card">
    <div class="hero-icon"><Key size={23} /></div>
    <div>
      <div class="eyebrow">Gateway credentials</div>
      <h1>API Keys</h1>
      <p>Create keys for OpenAI-compatible clients. For security, each full secret is shown exactly once.</p>
    </div>
    <button class="btn-primary create-button" onclick={() => showForm = !showForm}>
      {#if showForm}<X size={16} />{:else}<Plus size={16} />{/if}
      {showForm ? 'Cancel' : 'Create Key'}
    </button>
  </section>

  <div class="summary-grid">
    <div class="summary-card"><span>Total keys</span><strong>{summary.total}</strong></div>
    <div class="summary-card active"><span>Active</span><strong>{summary.active}</strong></div>
    <div class="summary-card security"><span>Stored display</span><strong>Masked</strong></div>
  </div>

  {#if showForm}
    <section class="create-card">
      <div><strong>Name this key</strong><span>Use a purpose such as “Production API” or “CI deploy”.</span></div>
      <div class="create-controls">
        <label class="sr-only" for="new-key-name">Key name</label>
        <input id="new-key-name" class="input-field" bind:value={newKeyName} placeholder="e.g. Production API" autocomplete="off" onkeydown={(e) => e.key === 'Enter' && createKey()} />
        <button class="btn-primary" onclick={createKey} disabled={creating || !newKeyName.trim()}>{creating ? 'Creating…' : 'Create key'}</button>
      </div>
    </section>
  {/if}

  {#if loading}
    <Spinner />
  {:else if error}
    <div class="card"><EmptyState icon={AlertCircle} title="Failed to load API keys" description={error} action={async () => { loading = true; error = ''; try { await loadKeys(); } catch (e: any) { error = e.message || 'Failed to load API keys'; } loading = false; }} actionLabel="Retry" /></div>
  {:else if keys.length === 0}
    <div class="card"><EmptyState icon={Key} title="No gateway API keys" description="Create a gateway API key for OpenAI-compatible client access. Provider credentials stay under Connections." /></div>
  {:else}
    <section class="keys-card">
      <div class="table-heading">
        <div><strong>Issued keys</strong><span>Saved secrets are masked and cannot be copied as credentials.</span></div>
        <div class="masked-badge"><LockKeyhole size={13} /> Masked list</div>
      </div>
      <div class="table-scroll">
        <table>
          <thead><tr><th>Name</th><th>Key fingerprint</th><th>Created</th><th><span class="sr-only">Actions</span></th></tr></thead>
          <tbody>
            {#each keys as k}
              <tr>
                <td><strong>{k.name || 'Unnamed'}</strong></td>
                <td>
                  <code class="masked-key">{maskedKeyLabel(k)}</code>
                  <span class="masked-note">Not a usable credential</span>
                </td>
                <td class="created-date">{k.created_at ? new Date(k.created_at).toLocaleDateString() : '-'}</td>
                <td class="actions">
                  <button class="delete-button" onclick={() => deleteKey(k.id)} title="Delete key" aria-label={`Delete ${k.name || 'unnamed'} key`}><Trash2 size={15} /></button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>
  {/if}
</div>

{#if oneTimeSecret}
  <div class="dialog-backdrop">
    <div class="secret-dialog" role="dialog" aria-modal="true" aria-labelledby="secret-title" aria-describedby="secret-warning">
      <div class="warning-icon"><ShieldAlert size={27} /></div>
      <div class="eyebrow warning">One-time secret</div>
      <h2 id="secret-title">Save “{oneTimeSecret.name}” now</h2>
      <p id="secret-warning">This full API key is shown once and cannot be retrieved again after you close this dialog.</p>
      <div class="secret-value">
        <code>{oneTimeSecret.value}</code>
        <button class="copy-button" onclick={copyCurrentSecret}>
          {#if secretCopied}<Check size={16} /> Copied{:else}<Copy size={16} /> Copy full key{/if}
        </button>
      </div>
      {#if secretCopied}<div class="copy-confirmation" role="status"><Check size={14} /> Full key copied to clipboard.</div>{/if}
      <label class="acknowledgement">
        <input type="checkbox" bind:checked={secretAcknowledged} />
        <span>I saved this key and understand it cannot be retrieved again.</span>
      </label>
      <button class="btn-primary close-secret" onclick={dismissSecret} disabled={!secretAcknowledged}>Close and continue</button>
    </div>
  </div>
{/if}

<style>
  .keys-page { max-width: 1080px; margin: 0 auto; animation: fadeInUp .4s ease-out; }
  .hero-card { display: flex; align-items: center; gap: 16px; padding: 24px; border-radius: 18px; margin-bottom: 14px; background: linear-gradient(135deg, var(--color-primary-light), var(--color-purple-light)); border: 1px solid color-mix(in srgb, var(--color-primary) 22%, var(--color-border)); }
  .hero-icon { display: grid; place-items: center; width: 46px; height: 46px; flex: none; border-radius: 14px; color: var(--color-primary); background: var(--color-bg-card); box-shadow: var(--shadow-sm); }
  .eyebrow { color: var(--color-primary); text-transform: uppercase; letter-spacing: .09em; font-size: 10px; font-weight: 800; }
  h1, h2 { color: var(--color-fg-0); letter-spacing: -.025em; }
  h1 { margin: 2px 0 3px; font-size: 25px; }
  h2 { margin: 5px 0 7px; font-size: 23px; }
  .hero-card p, .secret-dialog p { margin: 0; color: var(--color-fg-2); font-size: 13px; }
  .create-button { margin-left: auto; flex: none; display: inline-flex; align-items: center; gap: 7px; }
  .summary-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; margin-bottom: 16px; }
  .summary-card { padding: 14px 16px; border-radius: 12px; border: 1px solid var(--color-border); background: var(--color-bg-card); }
  .summary-card span { display: block; color: var(--color-fg-3); font-size: 10px; font-weight: 700; text-transform: uppercase; letter-spacing: .07em; }
  .summary-card strong { display: block; margin-top: 4px; color: var(--color-fg-0); font: 700 20px var(--font-mono); }
  .summary-card.active { background: var(--color-success-light); border-color: color-mix(in srgb, var(--color-success) 22%, var(--color-border)); }
  .summary-card.security { background: var(--color-primary-light); border-color: color-mix(in srgb, var(--color-primary) 20%, var(--color-border)); }
  .summary-card.security strong { font-size: 16px; }
  .create-card { display: grid; grid-template-columns: minmax(180px, .65fr) 1fr; align-items: center; gap: 18px; padding: 18px; margin-bottom: 16px; border-radius: 14px; background: var(--color-bg-card); border: 1px solid var(--color-border); box-shadow: var(--shadow-sm); animation: fadeInScale .25s ease-out; }
  .create-card strong, .create-card span { display: block; }
  .create-card span, .table-heading span { margin-top: 3px; color: var(--color-fg-3); font-size: 11px; }
  .create-controls { display: flex; gap: 9px; }
  .create-controls input { flex: 1; min-width: 0; }
  .keys-card { border-radius: 15px; overflow: hidden; border: 1px solid var(--color-border); background: var(--color-bg-card); box-shadow: var(--shadow-sm); }
  .table-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 15px 17px; border-bottom: 1px solid var(--color-border); }
  .table-heading strong, .table-heading span { display: block; }
  .masked-badge { display: inline-flex; align-items: center; gap: 5px; flex: none; padding: 6px 9px; color: var(--color-primary); background: var(--color-primary-light); border-radius: 8px; font-size: 11px; font-weight: 700; }
  .table-scroll { overflow-x: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th { text-align: left; padding: 11px 16px; font-size: 10px; font-weight: 700; text-transform: uppercase; letter-spacing: .06em; color: var(--color-fg-3); background: var(--color-bg-body); border-bottom: 1px solid var(--color-border); }
  td { padding: 13px 16px; border-bottom: 1px solid var(--color-border-light); color: var(--color-fg-1); }
  tbody tr:last-child td { border-bottom: 0; }
  .masked-key { display: inline-block; padding: 4px 8px; border-radius: 6px; background: var(--color-bg-body); color: var(--color-fg-2); font-size: 11px; }
  .masked-note { display: block; margin-top: 4px; color: var(--color-fg-3); font-size: 10px; }
  .created-date { color: var(--color-fg-3); font-size: 12px; }
  .actions { text-align: right; }
  .delete-button { display: inline-grid; place-items: center; padding: 7px; border: 1px solid var(--color-border); border-radius: 8px; background: var(--color-bg-card); color: var(--color-error); cursor: pointer; }
  .delete-button:hover { background: var(--color-error-light); }
  .dialog-backdrop { position: fixed; inset: 0; z-index: 1000; display: grid; place-items: center; padding: 18px; background: rgba(8, 15, 30, .68); backdrop-filter: blur(5px); animation: fadeIn .2s ease-out; }
  .secret-dialog { width: min(610px, 100%); padding: 27px; border-radius: 20px; border: 1px solid color-mix(in srgb, var(--color-warning) 36%, var(--color-border)); background: var(--color-bg-card); box-shadow: 0 24px 80px rgba(0,0,0,.35); animation: fadeInScale .25s ease-out; }
  .warning-icon { display: grid; place-items: center; width: 52px; height: 52px; margin-bottom: 13px; border-radius: 15px; color: var(--color-warning); background: var(--color-warning-light); }
  .eyebrow.warning { color: var(--color-warning); }
  .secret-value { margin-top: 18px; padding: 13px; border: 1px solid color-mix(in srgb, var(--color-warning) 32%, var(--color-border)); border-radius: 12px; background: var(--color-warning-light); }
  .secret-value code { display: block; padding: 10px; overflow-wrap: anywhere; color: var(--color-fg-0); background: var(--color-bg-card); border-radius: 8px; font-size: 12px; user-select: all; }
  .copy-button { width: 100%; display: inline-flex; align-items: center; justify-content: center; gap: 7px; margin-top: 9px; padding: 10px; border: 0; border-radius: 8px; color: white; background: var(--color-primary); font-weight: 700; cursor: pointer; }
  .copy-confirmation { display: flex; align-items: center; gap: 6px; margin-top: 9px; color: var(--color-success); font-size: 12px; font-weight: 650; }
  .acknowledgement { display: flex; align-items: flex-start; gap: 9px; margin: 17px 0 13px; padding: 12px; border-radius: 10px; color: var(--color-fg-1); background: var(--color-bg-body); font-size: 12px; cursor: pointer; }
  .acknowledgement input { margin-top: 2px; accent-color: var(--color-primary); }
  .close-secret { width: 100%; }
  @media (max-width: 640px) {
    .hero-card { align-items: flex-start; flex-wrap: wrap; padding: 19px; }
    .create-button { width: 100%; margin-left: 0; }
    .summary-grid { grid-template-columns: 1fr 1fr; }
    .summary-card.security { grid-column: 1 / -1; }
    .create-card { grid-template-columns: 1fr; }
    .create-controls { flex-direction: column; }
    .table-heading { align-items: flex-start; }
    .secret-dialog { padding: 21px; }
  }
</style>
