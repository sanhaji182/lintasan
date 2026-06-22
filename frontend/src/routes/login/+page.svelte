<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import {
    ArrowRight, Eye, EyeOff, Loader2, Lock, LogIn, User,
    ShieldCheck, Terminal, ArrowUpRight, Info, KeyRound, WifiOff, AlertTriangle
  } from 'lucide-svelte';

  let username = $state('');
  let password = $state('');
  let error = $state('');
  let errorType = $state<'auth' | 'network' | 'validation' | 'unknown'>('unknown');
  let loading = $state(false);
  let checkingSession = $state(true);
  let showPassword = $state(false);
  let mounted = $state(false);

  // First-run setup state
  let setupState = $state<'loading' | 'bootstrap' | 'active'>('loading');
  let hasAdmin = $state(false);
  let hasMasterKey = $state(false);

  onMount(async () => {
    mounted = true;

    // Fetch setup status for first-run detection
    try {
      const st = await api.get<{
        state: string; has_admin: boolean; has_master_key: boolean; setup_required: boolean;
      }>('/api/setup/status');
      setupState = st.setup_required ? 'bootstrap' : 'active';
      hasAdmin = st.has_admin;
      hasMasterKey = st.has_master_key;
    } catch {
      setupState = 'active';
    }

    // Session check
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
      errorType = 'validation';
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
      if (e.message?.toLowerCase().includes('network') ||
          e.message?.toLowerCase().includes('fetch') ||
          e.message?.toLowerCase().includes('failed to fetch')) {
        error = 'Cannot reach server. Make sure Lintasan is running on port 20180.';
        errorType = 'network';
      } else if (e.status === 401 || e.status === 403) {
        error = 'Wrong username or password. Please try again.';
        errorType = 'auth';
      } else {
        error = e.message || 'Invalid credentials';
        errorType = 'unknown';
      }
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

      <!-- First-run card in left panel -->
      {#if setupState === 'bootstrap'}
        <div class="bootstrap-card">
          <div class="bootstrap-head">
            <KeyRound size={16} />
            <span class="bootstrap-title">First-Run Setup</span>
          </div>
          <p class="bootstrap-text">
            This is your <strong>first time</strong> running Lintasan. The admin password was
            generated randomly and printed to the terminal console (stderr).
          </p>
          <div class="bootstrap-tip">
            <Terminal size={14} />
            <span>Look for: <code>generated admin password: …</code> in server output</span>
          </div>
          <p class="bootstrap-text" style="margin-top: 8px;">
            After logging in, you'll be prompted to set a new password and configure a master key.
          </p>
        </div>
      {/if}
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
          <div class="error-msg" role="alert" class:error-auth={errorType === 'auth'} class:error-network={errorType === 'network'}>
            <span class="error-icon">
              {#if errorType === 'auth'}
                <AlertTriangle size={14} />
              {:else if errorType === 'network'}
                <WifiOff size={14} />
              {:else}
                <AlertTriangle size={14} />
              {/if}
            </span>
            <span>{error}</span>
          </div>
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

      <!-- Password recovery card -->
      <div class="forgot-block">
        <div class="forgot-inner">
          <div class="forgot-icon"><Info size={14} /></div>
          <div class="forgot-body">
            <span class="forgot-title">Lupa password?</span>
            <span class="forgot-hint">
              Reset via terminal: <code>lintasan reset-password &lt;username&gt;</code>
            </span>
          </div>
          <a
            href="https://github.com/sanhaji182/lintasan#password-recovery"
            class="forgot-link"
            target="_blank"
            rel="noopener noreferrer"
            tabindex="0"
            title="View password recovery guide"
          >
            <ArrowUpRight size={13} />
          </a>
        </div>
      </div>

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

  /* First-run card in brand area */
  .bootstrap-card {
    margin-top: 28px;
    padding: 16px;
    background: #fffbeb;
    border: 1px solid #fde68a;
    border-radius: 14px;
  }
  .bootstrap-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
    color: #d97706;
  }
  .bootstrap-title {
    font-size: 13px;
    font-weight: 700;
    color: #92400e;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .bootstrap-text {
    margin: 0 0 6px;
    font-size: 13px;
    line-height: 1.5;
    color: #78350f;
  }
  .bootstrap-tip {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    background: #fef3c7;
    border-radius: 8px;
    font-size: 12px;
    color: #92400e;
    font-weight: 500;
  }
  .bootstrap-tip code {
    font-size: 12px;
    background: rgba(0,0,0,0.06);
    padding: 1px 5px;
    border-radius: 4px;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
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
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 14px;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 10px;
    font-size: 13px;
    color: #dc2626;
    font-weight: 500;
    animation: shakeX 0.4s ease-out;
  }
  .error-msg.error-network {
    background: #eff6ff;
    border-color: #bfdbfe;
    color: #2563eb;
  }
  .error-msg .error-icon { flex-shrink: 0; }

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

  /* Forgot password block */
  .forgot-block {
    margin-top: 20px;
  }
  .forgot-inner {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 12px 14px;
    background: #f8fafc;
    border: 1px solid #e2e8f0;
    border-radius: 12px;
    transition: all 0.15s;
  }
  .forgot-inner:hover {
    background: #eef2ff;
    border-color: #c7d2fe;
  }
  .forgot-icon {
    flex-shrink: 0;
    width: 24px; height: 24px;
    border-radius: 8px;
    background: #eef2ff;
    color: #4f46e5;
    display: grid;
    place-items: center;
  }
  .forgot-body { flex: 1; min-width: 0; }
  .forgot-title {
    display: block;
    font-size: 13px;
    font-weight: 600;
    color: #334155;
    margin-bottom: 2px;
  }
  .forgot-hint {
    display: block;
    font-size: 11px;
    color: #94a3b8;
    line-height: 1.4;
  }
  .forgot-hint code {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 10px;
    background: #e2e8f0;
    padding: 1px 4px;
    border-radius: 4px;
  }
  .forgot-link {
    flex-shrink: 0;
    display: grid;
    place-items: center;
    width: 28px; height: 28px;
    border-radius: 8px;
    background: #e2e8f0;
    color: #64748b;
    text-decoration: none;
    transition: all 0.15s;
    margin-top: 2px;
  }
  .forgot-link:hover {
    background: #c7d2fe;
    color: #4f46e5;
  }

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
  @keyframes shakeX {
    0%, 100% { transform: translateX(0); }
    10%, 50%, 90% { transform: translateX(-3px); }
    30%, 70% { transform: translateX(3px); }
  }

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
