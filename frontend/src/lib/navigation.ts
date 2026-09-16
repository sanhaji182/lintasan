export type NavigationItem = {
  label: string;
  path: string;
  icon: string;
  description: string;
  keywords?: string[];
};

export type NavigationGroup = {
  label: 'Operate' | 'Build' | 'Observe' | 'Configure';
  collapsible: boolean;
  items: NavigationItem[];
};

export const navigationGroups: NavigationGroup[] = [
  { label: 'Operate', collapsible: false, items: [
    { label: 'Overview', path: '/dashboard', icon: 'overview', description: 'Gateway health and recent activity', keywords: ['status', 'health'] },
    { label: 'Connections', path: '/dashboard/connections', icon: 'connections', description: 'Provider accounts and their models', keywords: ['provider', 'account', 'discover', 'oauth'] },
    { label: 'Routing', path: '/dashboard/routing', icon: 'routing', description: 'Traffic policies, combos and fallback', keywords: ['policy', 'alias', 'fallback'] },
  ]},
  { label: 'Build', collapsible: false, items: [
    { label: 'Quickstart', path: '/dashboard/quickstart', icon: 'quickstart', description: 'Connect an app to the gateway', keywords: ['setup', 'guide'] },
    { label: 'Playground', path: '/dashboard/playground', icon: 'playground', description: 'Try a callable model', keywords: ['chat', 'test'] },
    { label: 'Models', path: '/dashboard/models', icon: 'models', description: 'Inspect callable model routes', keywords: ['catalog', 'test'] },
    { label: 'Provider Catalog', path: '/dashboard/providers', icon: 'providers', description: 'Browse supported provider presets', keywords: ['provider', 'preset'] },
  ]},
  { label: 'Observe', collapsible: false, items: [
    { label: 'Analytics', path: '/dashboard/analytics', icon: 'observability', description: 'Requests, usage, savings and logs', keywords: ['observe', 'requests', 'usage', 'logs', 'cost'] },
  ]},
  { label: 'Configure', collapsible: true, items: [
    { label: 'API Keys', path: '/dashboard/keys', icon: 'keys', description: 'Gateway credentials', keywords: ['token', 'credential'] },
    { label: 'Teams', path: '/dashboard/teams', icon: 'teams', description: 'Team access and membership' },
    { label: 'Users', path: '/dashboard/users', icon: 'users', description: 'Dashboard users and roles' },
    { label: 'Webhooks', path: '/dashboard/webhooks', icon: 'webhooks', description: 'Event delivery endpoints' },
    { label: 'Memory', path: '/dashboard/memory', icon: 'memory', description: 'Gateway memory records' },
    { label: 'MCP Server', path: '/dashboard/mcp', icon: 'mcp', description: 'Model Context Protocol tools' },
    { label: 'Translator', path: '/dashboard/translator', icon: 'translator', description: 'Request format translator' },
    { label: 'Plugins', path: '/dashboard/plugins', icon: 'plugins', description: 'Gateway extensions' },
    { label: 'Backup', path: '/dashboard/backup', icon: 'backup', description: 'Export and restore configuration' },
    { label: 'Migrate', path: '/dashboard/migrate', icon: 'migrate', description: 'Import another gateway setup' },
    { label: 'Labs', path: '/dashboard/experimental', icon: 'labs', description: 'Experimental provider integrations' },
    { label: 'Settings', path: '/dashboard/settings', icon: 'settings', description: 'Gateway defaults and behavior' },
    { label: 'Docs', path: '/dashboard/docs', icon: 'docs', description: 'API and product documentation' },
  ]},
];

export const dashboardRoutes = navigationGroups.flatMap(group => group.items.map(item => item.path));
export const navigationItems = navigationGroups.flatMap(group =>
  group.items.map(item => ({ ...item, group: group.label }))
);

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

export function searchNavigation(query: string): Array<NavigationItem & { group: NavigationGroup['label'] }> {
  const terms = query.toLowerCase().trim().split(/\s+/).filter(Boolean);
  if (!terms.length) return navigationItems;
  return navigationItems.filter(item => {
    const haystack = [item.label, item.description, item.group, ...(item.keywords || [])].join(' ').toLowerCase();
    return terms.every(term => haystack.includes(term));
  });
}
