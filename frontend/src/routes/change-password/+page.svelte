<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import {
    ArrowRight, Eye, EyeOff, Loader2, Lock, ShieldCheck, KeyRound
  } from 'lucide-svelte';

  let currentPassword = $state('');
  let newPassword = $state('');
  let confirmPassword = $state('');
  let error = $state('');
  let success = $state(false);
  let loading = $state(false);
  let mounted = $state(false);
  let showCurrent = $state(false);
  let showNew = $state(false);

  let username = $state('');

  onMount(() => {
    mounted = true;
    // Surface who is rotating, if we have it cached.
    try {
      const raw = localStorage.getItem('lintasan_user');
      if (raw) username = (JSON.parse(raw)?.username) || '';
    } catch { /* ignore */ }
  });

  function validate(): string | null {
    if (!currentPassword.trim()) return 'Current password is required';
    if (newPassword.length < 8) return 'New password must be at least 8 characters';
    if (newPassword === currentPassword) return 'New password must differ from the current password';
    if (newPassword !== confirmPassword) return 'New password and confirmation do not match';
    return null;
  }

  async function handleSubmit() {
    const v = validate();
    if (v) { error = v; return; }
    loading = true;
    error = '';
    try {
      await api.post('/api/auth/change-password', {
        current_password: currentPassword,
        new_password: newPassword
      });
      success = true;
      // Refresh cached user so must_change_password flips to false everywhere.
      try {
        const me = await api.get<{ id: string; username: string; role: string; must_change_password?: boolean }>('/api/auth/me');
        localStorage.setItem('lintasan_user', JSON.stringify(me));
      } catch { /* non-fatal */ }
      setTimeout(() => goto('/dashboard'), 900);
    } catch (e: any) {
      error = e.message || 'Could not change password';
    } finally {
      loading = false;
    }
  }

  function onSubmit(e: SubmitEvent) {
    e.preventDefault();
    if (!loading && !success) handleSubmit();
  }
</script>

<svelte:head>
  <title>Change Password — Lintasan</title>
  <meta name="description" content="Rotate your Lintasan password to continue." />
</svelte:head>

<div class="shell" class:mounted>
  <div class="card">
    <div class="card-head">
      <span class="head-icon"><ShieldCheck size={20} /></span>
      <div>
        <h1>Update your password</h1>
        <p>
          {#if username}
            For security, <strong>{username}</strong> must set a new password before continuing.
          {:else}
            For security, you must set a new password before continuing.
          {/if}
        </p>
      </div>
    </div>

    {#if success}
      <div class="success-msg" role="status">
        <ShieldCheck size={16} />
        Password updated. Redirecting to your dashboard...
      </div>
    {:else}
      <form onsubmit={onSubmit} novalidate>
        <div class="field">
          <label for="current">Current password</label>
          <div class="input-wrap">
            <span class="input-icon"><Lock size={16} /></span>
            <input
              id="current"
              type={showCurrent ? 'text' : 'password'}
              autocomplete="current-password"
              placeholder="Enter your current password"
              bind:value={currentPassword}
              disabled={loading}
              required
            />
            <button type="button" class="toggle-vis" onclick={() => showCurrent = !showCurrent}
              disabled={loading} aria-label={showCurrent ? 'Hide password' : 'Show password'}>
              {#if showCurrent}<EyeOff size={16} />{:else}<Eye size={16} />{/if}
            </button>
          </div>
        </div>

        <div class="field">
          <label for="new">New password</label>
          <div class="input-wrap">
            <span class="input-icon"><KeyRound size={16} /></span>
            <input
              id="new"
              type={showNew ? 'text' : 'password'}
              autocomplete="new-password"
              placeholder="At least 8 characters"
              bind:value={newPassword}
              disabled={loading}
              required
            />
            <button type="button" class="toggle-vis" onclick={() => showNew = !showNew}
              disabled={loading} aria-label={showNew ? 'Hide password' : 'Show password'}>
              {#if showNew}<EyeOff size={16} />{:else}<Eye size={16} />{/if}
            </button>
          </div>
        </div>

        <div class="field">
          <label for="confirm">Confirm new password</label>
          <div class="input-wrap">
            <span class="input-icon"><KeyRound size={16} /></span>
            <input
              id="confirm"
              type={showNew ? 'text' : 'password'}
              autocomplete="new-password"
              placeholder="Re-enter your new password"
              bind:value={confirmPassword}
              disabled={loading}
              required
            />
          </div>
        </div>

        {#if error}
          <div class="error-msg" role="alert">{error}</div>
        {/if}

        <button type="submit" class="submit-btn" disabled={loading}>
          {#if loading}
            <Loader2 size={16} class="spin" />
            Updating...
          {:else}
            Update password
            <ArrowRight size={16} />
          {/if}
        </button>
      </form>
    {/if}

    <p class="foot"><Lock size={12} /> Your session stays active while you rotate.</p>
  </div>
</div>

<style>
  .shell {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: #f8fafc;
    font-family: 'Inter', system-ui, -apple-system, sans-serif;
    opacity: 0;
    transition: opacity 0.3s ease;
    padding: 24px;
  }
  .shell.mounted { opacity: 1; }

  .card {
    width: 100%;
    max-width: 440px;
    background: #ffffff;
    border: 1px solid #e2e8f0;
    border-radius: 16px;
    padding: 32px;
    box-shadow: 0 4px 24px rgba(15, 23, 42, 0.05);
  }

  .card-head {
    display: flex;
    gap: 14px;
    align-items: flex-start;
    margin-bottom: 24px;
  }
  .head-icon {
    flex-shrink: 0;
    width: 40px; height: 40px;
    border-radius: 10px;
    background: #eef2ff;
    color: #4f46e5;
    display: grid;
    place-items: center;
  }
  .card-head h1 {
    margin: 0 0 4px;
    font-size: 20px;
    font-weight: 700;
    letter-spacing: -0.02em;
    color: #0f172a;
  }
  .card-head p {
    margin: 0;
    font-size: 13px;
    color: #64748b;
    line-height: 1.5;
  }

  form { display: flex; flex-direction: column; gap: 16px; }

  .field label {
    display: block;
    margin-bottom: 6px;
    font-size: 13px;
    font-weight: 600;
    color: #334155;
  }
  .input-wrap { position: relative; display: flex; align-items: center; }
  .input-icon { position: absolute; left: 12px; color: #94a3b8; pointer-events: none; }
  .input-wrap input {
    width: 100%;
    padding: 11px 40px 11px 38px;
    background: #f8fafc;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    font-size: 14px;
    color: #1e293b;
    transition: border-color 0.2s, box-shadow 0.2s;
    outline: none;
  }
  .input-wrap input:focus { border-color: #4f46e5; box-shadow: 0 0 0 3px rgba(79, 70, 229, 0.12); }
  .input-wrap input::placeholder { color: #94a3b8; }
  .input-wrap input:disabled { opacity: 0.6; }

  .toggle-vis {
    position: absolute;
    right: 8px;
    background: none;
    border: none;
    cursor: pointer;
    padding: 6px;
    color: #94a3b8;
    border-radius: 6px;
  }
  .toggle-vis:hover { background: #f1f5f9; color: #64748b; }

  .error-msg {
    padding: 10px 14px;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 10px;
    font-size: 13px;
    color: #dc2626;
    font-weight: 500;
  }
  .success-msg {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 14px;
    background: #f0fdf4;
    border: 1px solid #bbf7d0;
    border-radius: 10px;
    font-size: 14px;
    color: #16a34a;
    font-weight: 500;
  }

  .submit-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    width: 100%;
    padding: 12px 20px;
    background: #4f46e5;
    color: #fff;
    border: none;
    border-radius: 10px;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s;
  }
  .submit-btn:hover:not(:disabled) { background: #4338ca; }
  .submit-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .foot {
    margin-top: 20px;
    text-align: center;
    font-size: 12px;
    color: #94a3b8;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
  }

  @keyframes spin { to { transform: rotate(360deg); } }
  :global(.spin) { animation: spin 1s linear infinite; }
</style>
