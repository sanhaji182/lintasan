export type NavigationItem = { label: string; path: string; icon: string };
export type NavigationGroup = { label: 'Primary' | 'Gateway' | 'Observability' | 'Manage' | 'Advanced'; collapsible: boolean; items: NavigationItem[] };

export const navigationGroups: NavigationGroup[] = [
  { label: 'Primary', collapsible: false, items: [
    { label: 'Overview', path: '/dashboard', icon: 'overview' },
    { label: 'Quickstart', path: '/dashboard/quickstart', icon: 'quickstart' },
    { label: 'Playground', path: '/dashboard/playground', icon: 'playground' },
  ]},
  { label: 'Gateway', collapsible: false, items: [
    { label: 'Connections', path: '/dashboard/connections', icon: 'connections' },
    { label: 'Models', path: '/dashboard/models', icon: 'models' },
    { label: 'Provider Catalog', path: '/dashboard/providers', icon: 'providers' },
    { label: 'Routing', path: '/dashboard/routing', icon: 'routing' },
  ]},
  { label: 'Observability', collapsible: false, items: [
    { label: 'Observability', path: '/dashboard/analytics', icon: 'observability' },
  ]},
  { label: 'Manage', collapsible: true, items: [
    { label: 'API Keys', path: '/dashboard/keys', icon: 'keys' },
    { label: 'Teams', path: '/dashboard/teams', icon: 'teams' },
    { label: 'Users', path: '/dashboard/users', icon: 'users' },
    { label: 'Webhooks', path: '/dashboard/webhooks', icon: 'webhooks' },
  ]},
  { label: 'Advanced', collapsible: true, items: [
    { label: 'Memory', path: '/dashboard/memory', icon: 'memory' },
    { label: 'MCP Server', path: '/dashboard/mcp', icon: 'mcp' },
    { label: 'Translator', path: '/dashboard/translator', icon: 'translator' },
    { label: 'Plugins', path: '/dashboard/plugins', icon: 'plugins' },
    { label: 'Backup', path: '/dashboard/backup', icon: 'backup' },
    { label: 'Migrate', path: '/dashboard/migrate', icon: 'migrate' },
    { label: 'Labs', path: '/dashboard/experimental', icon: 'labs' },
    { label: 'Settings', path: '/dashboard/settings', icon: 'settings' },
    { label: 'Docs', path: '/dashboard/docs', icon: 'docs' },
  ]},
];

export const dashboardRoutes = navigationGroups.flatMap(group => group.items.map(item => item.path));

const relatedRoutes: Record<string, string[]> = {
  '/dashboard/connections': ['/dashboard/discover', '/dashboard/oauth-ide'],
  '/dashboard/routing': ['/dashboard/fallback'],
  '/dashboard/analytics': ['/dashboard/usage', '/dashboard/savings', '/dashboard/logs', '/dashboard/observability'],
};

export function routeIsActive(itemPath: string, pathname: string): boolean {
  if (itemPath === '/dashboard') return pathname === itemPath;
  return pathname.startsWith(itemPath) || (relatedRoutes[itemPath] || []).some(path => pathname.startsWith(path));
}

export function groupIsInitiallyOpen(label: NavigationGroup['label'], pathname: string): boolean {
  const group = navigationGroups.find(item => item.label === label);
  if (!group || !group.collapsible) return true;
  return group.items.some(item => routeIsActive(item.path, pathname));
}
