<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { Menu, UserCircle2, LogOut, LogIn, Sun, Moon } from 'lucide-svelte';
  import { theme } from '$lib/stores/theme';
  import HelpTooltip from '$lib/components/HelpTooltip.svelte';
  import { helpContent } from '$lib/helpContent';

  let { title = 'Overview', open = $bindable(false) }: { title?: string; open?: boolean } = $props();

  const currentHelp = $derived(helpContent[page.url.pathname] || '');

  let isAuthenticated = $state(false);
  let username = $state('');

  function refreshAuthState() {
    if (typeof window === 'undefined') return;
    const token = localStorage.getItem('lintasan_token');
    const userStr = localStorage.getItem('lintasan_user');
    isAuthenticated = !!token;
    if (userStr) {
      try { username = JSON.parse(userStr)?.username || ''; } catch { username = ''; }
    } else { username = ''; }
  }

  async function handleLogout() {
    try {
      await fetch('/api/auth/logout', {
        method: 'POST',
        headers: { Authorization: `Bearer ${localStorage.getItem('lintasan_token') || ''}` }
      });
    } catch {}
    localStorage.removeItem('lintasan_token');
    localStorage.removeItem('lintasan_user');
    refreshAuthState();
    await goto('/login');
  }

  onMount(() => {
    refreshAuthState();
    const handler = () => refreshAuthState();
    window.addEventListener('storage', handler);
    window.addEventListener('focus', handler);
    return () => {
      window.removeEventListener('storage', handler);
      window.removeEventListener('focus', handler);
    };
  });
</script>

<header class="header">
  <div class="header-left">
    <button class="menu-btn" onclick={() => open = !open} aria-label="Toggle sidebar">
      <Menu size={20} />
    </button>
    <h1 class="header-title">{title}</h1>
    {#if currentHelp}
      <HelpTooltip content={currentHelp} />
    {/if}
  </div>

  <div class="header-right">
    <button class="theme-toggle" onclick={() => theme.toggle()} title={$theme === 'light' ? 'Dark mode' : 'Light mode'}>
      {#if $theme === 'light'}<Moon size={16} />{:else}<Sun size={16} />{/if}
    </button>

    {#if isAuthenticated}
      <a href="/dashboard/users" class="user-pill" title="User Management">
        <UserCircle2 size={15} />
        <span class="user-pill-label">{username || 'Account'}</span>
      </a>
      <button class="logout-btn" onclick={handleLogout} title="Sign out">
        <LogOut size={15} />
        <span class="logout-label">Logout</span>
      </button>
    {:else}
      <a href="/login" class="login-link">
        <LogIn size={15} />
        Sign In
      </a>
    {/if}
  </div>
</header>

<style>
  .header {
    position: sticky;
    top: 0;
    z-index: 40;
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: var(--header-h);
    padding: 0 24px;
    background: rgba(255,255,255,0.85);
    backdrop-filter: blur(12px);
    border-bottom: 1px solid #e2e8f0;
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: 0;
  }

  .menu-btn {
    display: none;
    width: 40px; height: 40px;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    border-radius: 10px;
    color: #64748b;
    cursor: pointer;
    flex-shrink: 0;
  }
  .menu-btn:hover { background: #f1f5f9; color: #1e293b; }

  .header-title {
    font-size: 16px;
    font-weight: 600;
    color: #1e293b;
    margin: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .header-right {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  }

  .theme-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px; height: 36px;
    background: none;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    color: #64748b;
    cursor: pointer;
    flex-shrink: 0;
  }
  .theme-toggle:hover { background: #f8fafc; color: #1e293b; }

  .user-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 7px 14px;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    font-size: 13px;
    font-weight: 500;
    color: #334155;
    text-decoration: none;
    max-width: 140px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    transition: background 0.15s;
  }
  .user-pill:hover { background: #f8fafc; }

  .logout-btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 7px 12px;
    background: none;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    font-size: 13px;
    font-weight: 500;
    color: #dc2626;
    cursor: pointer;
    transition: background 0.15s;
  }
  .logout-btn:hover { background: #fef2f2; }

  .login-link {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 8px 16px;
    background: #4f46e5;
    color: #fff;
    border-radius: 10px;
    font-size: 13px;
    font-weight: 600;
    text-decoration: none;
  }
  .login-link:hover { background: #4338ca; }

  @media (max-width: 768px) {
    .menu-btn { display: flex; }
    .header { padding: 0 12px; }
    .header-title { font-size: 14px; }
    .user-pill-label { display: none; }
    .user-pill {
      padding: 7px 10px;
      max-width: none;
    }
    .logout-label { display: none; }
    .logout-btn { padding: 7px 10px; }
  }
</style>
