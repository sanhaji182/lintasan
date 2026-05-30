<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import {
    ArrowRight, Eye, EyeOff, Loader2, Lock, LogIn, User
  } from 'lucide-svelte';

  let username = $state('');
  let password = $state('');
  let error = $state('');
  let loading = $state(false);
  let checkingSession = $state(true);
  let showPassword = $state(false);
  let mounted = $state(false);

  onMount(async () => {
    mounted = true;
    const token = localStorage.getItem('lintasan_token');
    if (!token) { checkingSession = false; return; }
    try {
      const me = await api.get<{ must_change_password?: boolean }>('/api/auth/me');
      if (me.must_change_password) {
        await goto('/change-password');
      } else {
        await goto('/dashboard');
      }
    } catch {
      localStorage.removeItem('lintasan_token');
      localStorage.removeItem('lintasan_user');
      checkingSession = false;
    }
  });

  async function handleLogin() {
    if (!username.trim() || !password.trim()) {
      error = 'Username and password are required';
      return;
    }
    loading = true;
    error = '';
    try {
      const data = await api.post<{ token: string; user: { id: string; username: string; role: string; must_change_password?: boolean } }>(
        '/api/auth/login',
        { username: username.trim(), password: password.trim() }
      );
      if (data.token) {
        localStorage.setItem('lintasan_token', data.token);
        localStorage.setItem('lintasan_user', JSON.stringify(data.user));
      }
      if (data.user?.must_change_password) {
        await goto('/change-password');
      } else {
        await goto('/dashboard');
      }
    } catch (e: any) {
      error = e.message || 'Invalid credentials';
      password = '';
    } finally { loading = false; }
  }

  function onSubmit(e: SubmitEvent) {
    e.preventDefault();
    if (!loading) handleLogin();
  }

  const formDisabled = $derived(loading || checkingSession);
</script>

<svelte:head>
  <title>Sign In — Lintasan</title>
  <meta name="description" content="Sign in to Lintasan dashboard." />
</svelte:head>

<div class="login-shell" class:mounted>
  <div class="login-layout">
    <div class="brand-card">
      <a href="/" class="brand">
        <span class="brand-mark">L</span>
        <span>Lintasan</span>
      </a>
      <h1>Welcome back</h1>
      <p>Sign in to manage your AI gateway.</p>
    </div>

    <div class="form-card">
      {#if checkingSession}
        <div class="session-pill" role="status">
          <Loader2 size={14} class="spin" />
          Checking session...
        </div>
      {/if}

      <form onsubmit={onSubmit} novalidate>
        <div class="field">
          <label for="username">Username</label>
          <div class="input-wrap">
            <span class="input-icon"><User size={16} /></span>
            <input
              id="username"
              type="text"
              autocomplete="username"
              placeholder="Enter your username"
              bind:value={username}
              disabled={formDisabled}
              required
            />
          </div>
        </div>

        <div class="field">
          <label for="password">Password</label>
          <div class="input-wrap">
            <span class="input-icon"><Lock size={16} /></span>
            <input
              id="password"
              type={showPassword ? 'text' : 'password'}
              autocomplete="current-password"
              placeholder="Enter your password"
              bind:value={password}
              disabled={formDisabled}
              required
            />
            <button
              type="button"
              class="toggle-vis"
              onclick={() => showPassword = !showPassword}
              disabled={formDisabled}
              aria-label={showPassword ? 'Hide password' : 'Show password'}
            >
              {#if showPassword}<EyeOff size={16} />{:else}<Eye size={16} />{/if}
            </button>
          </div>
        </div>

        {#if error}
          <div class="error-msg" role="alert">{error}</div>
        {/if}

        <button
          type="submit"
          class="submit-btn"
          disabled={formDisabled || !username.trim() || !password.trim()}
        >
          {#if loading}
            <Loader2 size={16} class="spin" />
            Signing in...
          {:else}
            <LogIn size={16} />
            Sign In
            <ArrowRight size={16} />
          {/if}
        </button>
      </form>

      <p class="form-footer">
        <Lock size={12} />
        Secured with JWT authentication
      </p>
    </div>
  </div>
</div>

<style>
  .login-shell {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: #f8fafc;
    font-family: 'Inter', system-ui, -apple-system, sans-serif;
    opacity: 0;
    transition: opacity 0.3s ease;
  }
  .login-shell.mounted { opacity: 1; }

  .login-layout {
    display: flex;
    align-items: stretch;
    gap: 32px;
    max-width: 880px;
    width: 100%;
    padding: 32px;
  }

  .brand-card {
    flex: 1;
    display: flex;
    flex-direction: column;
    justify-content: center;
    padding: 40px 32px;
  }
  .brand {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    text-decoration: none;
    margin-bottom: 32px;
  }
  .brand-mark {
    width: 34px; height: 34px;
    border-radius: 9px;
    background: #4f46e5;
    color: #fff;
    display: grid;
    place-items: center;
    font-weight: 700;
    font-size: 14px;
  }
  .brand span {
    font-size: 17px;
    font-weight: 700;
    color: #1e293b;
  }
  .brand-card h1 {
    margin: 0 0 8px;
    font-size: 32px;
    font-weight: 700;
    letter-spacing: -0.03em;
    color: #0f172a;
  }
  .brand-card p {
    margin: 0;
    font-size: 15px;
    color: #64748b;
    line-height: 1.6;
  }

  .form-card {
    flex: 1;
    background: #ffffff;
    border: 1px solid #e2e8f0;
    border-radius: 16px;
    padding: 36px 32px;
    box-shadow: 0 4px 24px rgba(15, 23, 42, 0.05);
  }

  .session-pill {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 8px 14px;
    background: #eef2ff;
    color: #4f46e5;
    border-radius: 8px;
    font-size: 13px;
    font-weight: 500;
    margin-bottom: 20px;
  }
  form {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  .field label {
    display: block;
    margin-bottom: 6px;
    font-size: 13px;
    font-weight: 600;
    color: #334155;
  }

  .input-wrap {
    position: relative;
    display: flex;
    align-items: center;
  }
  .input-icon {
    position: absolute;
    left: 12px;
    color: #94a3b8;
    pointer-events: none;
  }
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
  .input-wrap input:focus {
    border-color: #4f46e5;
    box-shadow: 0 0 0 3px rgba(79, 70, 229, 0.12);
  }
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

  .form-footer {
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

  @media (max-width: 640px) {
    .login-layout {
      flex-direction: column;
      padding: 20px;
      gap: 24px;
    }
    .brand-card { padding: 20px 0; }
    .form-card { padding: 28px 20px; }
  }
</style>
