<script lang="ts">
 import Sidebar from '$lib/components/Sidebar.svelte';
 import Header from '$lib/components/Header.svelte';
 import Toast from '$lib/components/Toast.svelte';
 import { page } from '$app/state';

  let { children } = $props();
  let sidebarOpen = $state(false);

  const pageTitles: Record<string, string> = {
    '/dashboard': 'Overview',
    '/dashboard/connections': 'Accounts',
    '/dashboard/routing': 'Routing',
    '/dashboard/fallback': 'Fallback',
    '/dashboard/logs': 'Logs',
    '/dashboard/usage': 'Usage',
    '/dashboard/analytics': 'Analytics',
    '/dashboard/keys': 'API Keys',
    '/dashboard/teams': 'Teams',
    '/dashboard/users': 'Users',
    '/dashboard/webhooks': 'Webhooks',
    '/dashboard/backup': 'Backup',
    '/dashboard/settings': 'Settings',
    '/dashboard/plugins': 'Plugins',
    '/dashboard/playground': 'Playground',
    '/dashboard/docs': 'Docs',
    '/dashboard/mcp': 'MCP Server',
    '/dashboard/savings': 'Cost Savings',
    '/dashboard/translator': 'Format Translator',
  };

  const title = $derived(pageTitles[page.url.pathname] || 'Dashboard');
</script>

<Sidebar bind:open={sidebarOpen} />

<div class="min-h-screen transition-all duration-300" style="margin-left: var(--sidebar-w);">
  <Header {title} bind:open={sidebarOpen} />

  <main class="fade-in" style="padding: 24px;">
   {@render children()}
 </main>
</div>

<Toast />

<style>
  @media (max-width: 768px) {
    div { margin-left: 0 !important; }
    main { padding: 16px 12px !important; }
  }
  .fade-in { animation: fadeInUp 0.4s ease-out; }
</style>
