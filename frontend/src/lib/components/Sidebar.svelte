<script lang="ts">
  import { page } from '$app/state';
  import {
    LayoutDashboard, Link2, GitBranch, ShieldAlert, ScrollText,
    BarChart3, TrendingUp, Key, Users, UserCircle, Webhook,
    Database, Settings, Puzzle, MessageSquare, BookOpen,
    Brain, Globe, Server, Activity, Sun, Moon
  } from 'lucide-svelte';
  import { theme } from '$lib/stores/theme';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  const menuItems = [
    { label: 'Overview', path: '/dashboard', icon: LayoutDashboard },
    { label: 'Accounts', path: '/dashboard/connections', icon: Link2 },
    { label: 'Providers', path: '/dashboard/providers', icon: Server },
    { label: 'Discover', path: '/dashboard/discover', icon: Globe },
    { label: 'Routing', path: '/dashboard/routing', icon: GitBranch },
    { label: 'Fallback', path: '/dashboard/fallback', icon: ShieldAlert },
    { label: 'Logs', path: '/dashboard/logs', icon: ScrollText },
    { label: 'Usage', path: '/dashboard/usage', icon: BarChart3 },
    { label: 'Analytics', path: '/dashboard/analytics', icon: TrendingUp },
    { label: 'Observability', path: '/dashboard/observability', icon: Activity },
    { label: 'Memory', path: '/dashboard/memory', icon: Brain },
  ];

  const manageItems = [
    { label: 'API Keys', path: '/dashboard/keys', icon: Key },
    { label: 'Teams', path: '/dashboard/teams', icon: Users },
    { label: 'User Management', path: '/dashboard/users', icon: UserCircle },
    { label: 'Webhooks', path: '/dashboard/webhooks', icon: Webhook },
    { label: 'Backup', path: '/dashboard/backup', icon: Database },
    { label: 'Settings', path: '/dashboard/settings', icon: Settings },
  ];

  const toolItems = [
    { label: 'MCP Server', path: '/dashboard/mcp', icon: Puzzle },
    { label: 'Savings', path: '/dashboard/savings', icon: TrendingUp },
    { label: 'Translator', path: '/dashboard/translator', icon: Globe },
    { label: 'Plugins', path: '/dashboard/plugins', icon: Puzzle },
    { label: 'Playground', path: '/dashboard/playground', icon: MessageSquare },
    { label: 'Docs', path: '/dashboard/docs', icon: BookOpen },
  ];

  function isActive(path: string) {
    if (path === '/dashboard') return page.url.pathname === '/dashboard';
    return page.url.pathname.startsWith(path);
  }

  // Version is fetched from /health (single source of truth in the Go binary)
  // rather than hardcoded, so the sidebar never drifts from the actual build.
  let version = $state('');
  $effect(() => {
    fetch('/health')
      .then((r) => r.ok ? r.json() : null)
      .then((d) => { if (d?.version) version = d.version; })
      .catch(() => {});
  });

</script>

{#if open}
  <button class="overlay" onclick={() => open = false} aria-label="Close sidebar"></button>
{/if}

<aside class="sidebar" class:open>
  <div class="sidebar-brand">
    <span class="sb-logo">L</span>
    <div>
      <div class="sb-name">Lintasan</div>
      <div class="sb-version">{version}</div>
    </div>
  </div>

  <nav class="sidebar-nav">
    {#each [{ label: 'MENU', items: menuItems }, { label: 'MANAGE', items: manageItems }, { label: 'TOOLS', items: toolItems }] as group}
      <div class="nav-group">
        <div class="nav-group-label">{group.label}</div>
        {#each group.items as item}
          {@const active = isActive(item.path)}
          <a
            href={item.path}
            class="nav-item"
            class:active
            onclick={() => open = false}
          >
            <item.icon size={18} stroke-width={1.6} />
            <span>{item.label}</span>
          </a>
        {/each}
      </div>
    {/each}
  </nav>

  <div class="sidebar-footer">
    <button class="theme-btn" onclick={() => theme.toggle()}>
      {#if $theme === 'light'}
        <Moon size={16} /> Dark mode
      {:else}
        <Sun size={16} /> Light mode
      {/if}
    </button>
  </div>
</aside>

<style>
  .overlay {
    display: none;
    position: fixed;
    inset: 0;
    z-index: 45;
    background: rgba(15, 23, 42, 0.3);
    backdrop-filter: blur(4px);
  }

  .sidebar {
    position: fixed;
    top: 0;
    left: 0;
    height: 100%;
    z-index: 50;
    width: var(--sidebar-w);
    background: #ffffff;
    border-right: 1px solid #e2e8f0;
    display: flex;
    flex-direction: column;
    transition: transform 0.25s ease;
  }

  .sidebar-brand {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 20px 20px 16px;
    border-bottom: 1px solid #f1f5f9;
  }

  .sb-logo {
    width: 34px; height: 34px;
    border-radius: 9px;
    background: #4f46e5;
    color: #fff;
    display: grid;
    place-items: center;
    font-weight: 700;
    font-size: 14px;
  }

  .sb-name {
    font-size: 15px;
    font-weight: 700;
    color: #1e293b;
    letter-spacing: -0.2px;
  }

  .sb-version {
    font-size: 11px;
    color: #94a3b8;
    font-family: 'JetBrains Mono', monospace;
  }

  .sidebar-nav {
    flex: 1;
    overflow-y: auto;
    padding: 12px 10px;
  }

  .nav-group { margin-bottom: 20px; }

  .nav-group-label {
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.06em;
    color: #94a3b8;
    text-transform: uppercase;
    padding: 6px 12px 8px;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    border-radius: 10px;
    font-size: 13px;
    font-weight: 500;
    color: #475569;
    text-decoration: none;
    margin-bottom: 2px;
    transition: background 0.15s, color 0.15s;
  }

  .nav-item:hover {
    background: #f8fafc;
    color: #1e293b;
  }

  .nav-item.active {
    background: #eef2ff;
    color: #4f46e5;
    font-weight: 600;
  }

  .sidebar-footer {
    padding: 16px 14px;
    border-top: 1px solid #f1f5f9;
  }

  .theme-btn {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 10px 12px;
    background: none;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    font-size: 13px;
    font-weight: 500;
    color: #475569;
    cursor: pointer;
  }
  .theme-btn:hover { background: #f8fafc; }

  @media (max-width: 768px) {
    .sidebar {
      transform: translateX(-100%);
    }
    .sidebar.open {
      transform: translateX(0);
    }
    .overlay {
      display: block;
    }
  }
</style>
