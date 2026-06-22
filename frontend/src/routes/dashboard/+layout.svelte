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
    '/dashboard/providers': 'Providers',
    '/dashboard/experimental': 'Experimental',
    '/dashboard/discover': 'Discover',
    '/dashboard/routing': 'Routing',
    '/dashboard/fallback': 'Fallback',
    '/dashboard/logs': 'Logs',
    '/dashboard/usage': 'Usage',
    '/dashboard/analytics': 'Analytics',
    '/dashboard/observability': 'Observability',
    '/dashboard/memory': 'Memory',
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
    '/dashboard/oauth-ide': 'OAuth IDE',
  };

  const title = $derived(pageTitles[page.url.pathname] || 'Dashboard');
</script>

<Sidebar bind:open={sidebarOpen} />

<div class="dashboard-shell">
  <Header {title} bind:open={sidebarOpen} />

  <main class="dashboard-main">
   {@render children()}
  </main>
</div>

<Toast />

<style>
  .dashboard-shell {
    min-height: 100vh;
    transition: margin-left 0.25s ease;
  }
  .dashboard-shell:not(.sidebar-hidden) {
    margin-left: var(--sidebar-w);
  }

  .dashboard-main {
    padding: 24px;
    animation: fadeInUp 0.4s ease-out;
  }

  @media (max-width: 768px) {
    .dashboard-shell {
      margin-left: 0 !important;
    }
    .dashboard-main {
      padding: 16px 12px !important;
    }
  }

  @keyframes fadeInUp {
    from { opacity: 0; transform: translateY(14px); }
    to { opacity: 1; transform: translateY(0); }
  }
</style>
