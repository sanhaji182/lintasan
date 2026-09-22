<script lang="ts">
import Sidebar from '$lib/components/Sidebar.svelte';
import Header from '$lib/components/Header.svelte';
import Toast from '$lib/components/Toast.svelte';
import { page } from '$app/state';

  let { children } = $props();
  let sidebarOpen = $state(false);

  const pageTitles: Record<string, string> = {
    '/dashboard': 'Overview',
    '/dashboard/quickstart': 'Quickstart',
    '/dashboard/connections': 'Connections',
    '/dashboard/models': 'Models',
    '/dashboard/providers': 'Provider Catalog',
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
    '/dashboard/migrate': 'Migrate',
    '/dashboard/settings': 'Settings',
    '/dashboard/plugins': 'Plugins',
    '/dashboard/playground': 'Playground',
    '/dashboard/docs': 'Docs',
    '/dashboard/mcp': 'MCP Server',
    '/dashboard/savings': 'Cost Savings',
    '/dashboard/translator': 'Format Translator',
    '/dashboard/oauth-ide': 'OAuth IDE',
    '/dashboard/qoder-connections': 'Qoder Connections',
  };

  const title = $derived(pageTitles[page.url.pathname] || 'Dashboard');
</script>

<Sidebar bind:open={sidebarOpen} />

<div class="dashboard-shell">
  <a class="skip-link" href="#dashboard-content">Skip to content</a>
  <Header {title} bind:open={sidebarOpen} />

  <main class="dashboard-main" id="dashboard-content" tabindex="-1">
   {@render children()}
  </main>
</div>

<Toast />

<style>
  .dashboard-shell {
    min-height: 100vh;
    min-width: 0;
    transition: margin-left 0.25s ease;
  }
  .dashboard-shell:not(.sidebar-hidden) {
    margin-left: var(--sidebar-w);
  }

  .dashboard-main {
    min-width: 0;
    padding: 28px clamp(20px, 3vw, 42px) 48px;
    animation: fadeInUp 0.4s ease;
  }

  .skip-link {
    position: fixed;
    top: 8px;
    left: calc(var(--sidebar-w) + 12px);
    z-index: 100;
    transform: translateY(-150%);
    padding: 9px 14px;
    border-radius: 9px;
    background: var(--color-primary);
    color: white;
    font-size: 13px;
    font-weight: 700;
    text-decoration: none;
    transition: transform .15s ease;
  }
  .skip-link:focus { transform: translateY(0); }

  @media (max-width: 768px) {
    .skip-link { left: 12px; }
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
