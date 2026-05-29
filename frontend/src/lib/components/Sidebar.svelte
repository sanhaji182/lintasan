<script lang="ts">
  import { page } from '$app/state';
  import { theme } from '$lib/stores/theme';
  import {
    LayoutDashboard, Link2, GitBranch, ShieldAlert, ScrollText,
    BarChart3, TrendingUp, Key, Users, UserCircle, Webhook,
    Database, Settings, Puzzle, MessageSquare, BookOpen,
    Brain, Globe, Sun, Moon, Menu, X
  } from 'lucide-svelte';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  const menuItems = [
    { label: 'Overview', path: '/dashboard', icon: LayoutDashboard },
    { label: 'Accounts', path: '/dashboard/connections', icon: Link2 },
    { label: 'Discover', path: '/dashboard/discover', icon: Globe },
    { label: 'Routing', path: '/dashboard/routing', icon: GitBranch },
    { label: 'Fallback', path: '/dashboard/fallback', icon: ShieldAlert },
    { label: 'Logs', path: '/dashboard/logs', icon: ScrollText },
    { label: 'Usage', path: '/dashboard/usage', icon: BarChart3 },
    { label: 'Analytics', path: '/dashboard/analytics', icon: TrendingUp },
    { label: 'Memory', path: '/dashboard/memory', icon: Brain },
  ];

  const manageItems = [
    { label: 'API Keys', path: '/dashboard/keys', icon: Key },
    { label: 'Teams', path: '/dashboard/teams', icon: Users },
    { label: 'Users', path: '/dashboard/users', icon: UserCircle },
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
</script>

<!-- Mobile backdrop -->
{#if open}
  <button
    class="fixed inset-0 bg-black/40 z-40 md:hidden"
    onclick={() => open = false}
    aria-label="Close sidebar"
  ></button>
{/if}

<aside
  class="fixed top-0 left-0 h-full z-50 flex flex-col border-r transition-transform duration-300 md:translate-x-0"
  style="
    width: var(--sidebar-w);
    background: var(--color-bg-sidebar);
    border-color: var(--color-sidebar-border);
    box-shadow: var(--shadow-sm);
    transform: {open ? 'translateX(0)' : ''};
  "
  class:-translate-x-full={!open}
>
  <!-- Logo -->
  <div class="flex items-center gap-3 p-5 border-b" style="border-color: var(--color-sidebar-border);">
    <div
      class="flex items-center justify-center text-white font-bold text-base"
      style="
        width: 36px; height: 36px; border-radius: 10px;
        background: linear-gradient(135deg, var(--color-primary) 0%, #6366f1 100%);
        box-shadow: 0 4px 12px var(--color-primary-glow);
      "
    >L</div>
    <div>
      <div class="font-bold text-sm" style="color: var(--color-fg-0); letter-spacing: -0.2px;">Lintasan</div>
      <div class="text-xs font-mono" style="color: var(--color-fg-3);">v2.0.0</div>
    </div>
  </div>

  <!-- Navigation -->
  <nav class="flex-1 overflow-y-auto p-3" style="padding: 12px 10px;">
    {#each [
      { label: 'MENU', items: menuItems },
      { label: 'MANAGE', items: manageItems },
      { label: 'TOOLS', items: toolItems }
    ] as group}
      <div class="mb-5">
        <div
          class="text-xs font-semibold uppercase mb-2 px-3"
          style="color: var(--color-fg-3); letter-spacing: 0.8px; font-size: 10px;"
        >{group.label}</div>
        {#each group.items as item}
          {@const active = isActive(item.path)}
          <a
            href={item.path}
            class="flex items-center gap-2.5 mb-0.5 rounded-lg transition-all duration-200 relative"
            style="
              padding: 10px 12px;
              font-size: 13px;
              font-weight: {active ? 600 : 500};
              color: {active ? 'var(--color-primary)' : 'var(--color-fg-1)'};
              background: {active ? 'var(--color-primary-light)' : 'transparent'};
              border-left: 3px solid {active ? 'var(--color-primary)' : 'transparent'};
            "
            onclick={() => open = false}
          >
            <item.icon size={18} stroke-width={1.8} />
            <span>{item.label}</span>
            {#if active}
              <span
                class="absolute right-3 w-1.5 h-1.5 rounded-full"
                style="background: var(--color-primary); animation: dotPulse 2s infinite;"
              ></span>
            {/if}
          </a>
        {/each}
      </div>
    {/each}
  </nav>

  <!-- Theme toggle -->
  <div class="p-4 border-t" style="border-color: var(--color-sidebar-border);">
    <button
      class="flex items-center gap-2.5 w-full rounded-lg transition-all duration-200"
      style="
        padding: 10px 12px;
        background: var(--color-bg-sidebar-hover);
        border: 1px solid var(--color-sidebar-border);
        color: var(--color-fg-1);
        font-size: 13px; font-weight: 500;
        min-height: 44px;
      "
      onclick={() => theme.toggle()}
    >
      {#if $theme === 'light'}
        <Moon size={18} stroke-width={1.8} />
        <span>Dark Mode</span>
      {:else}
        <Sun size={18} stroke-width={1.8} />
        <span>Light Mode</span>
      {/if}
    </button>
  </div>
</aside>
