export interface ComboConnection {
  id: string;
  name?: string;
  format?: string;
  base_url?: string;
  chat_path?: string;
  pool_id?: string;
  is_active?: number | boolean;
  oauth_provider?: string;
}

export interface ProviderCatalogEntry {
  name: string;
  domain?: string;
  base_url?: string;
  oauth_provider?: string;
}

export interface DiscoveredComboModel {
  model_id: string;
  connection_id?: string;
  is_active?: number | boolean;
}

export interface ComboProviderOption {
  id: string;
  name: string;
  provider: string;
  connectionIds: string[];
  accounts: Array<{ id: string; name: string }>;
  searchText: string;
  canSync: boolean;
}

export interface DraftComboEntry {
  model: string;
  providerId: string;
  connectionId?: string;
}

function endpointIdentity(connection: ComboConnection): { id: string; provider: string } {
  const pool = connection.pool_id?.trim();
  if (pool) return { id: `pool:${pool}`, provider: pool };

  const format = (connection.format || 'custom').trim().toLowerCase();
  let provider = 'unknown';
  let endpoint = 'unknown';
  try {
    let base = (connection.base_url || '').trim().replace(/\/+$/, '');
    let path = (connection.chat_path || '').trim();
    if (path && !path.startsWith('/')) path = `/${path}`;
    const version = base.match(/\/(v\d[^/]*)$/i)?.[1]?.toLowerCase();
    if (version && path.toLowerCase().startsWith(`/${version}/`)) {
      path = path.slice(version.length + 1);
    }
    const parsed = new URL(`${base}${path}`);
    provider = parsed.hostname.toLowerCase().replace(/\.$/, '') || provider;
    const effectivePath = `/${parsed.pathname.toLowerCase().replace(/^\/+|\/+$/g, '')}`.replace(/^\/$/, '');
    endpoint = `${parsed.protocol.toLowerCase()}//${parsed.host.toLowerCase().replace(/\.$/, '')}${effectivePath}`;
  } catch { /* retain conservative identity */ }
  return { id: `provider:${format}:${endpoint}`, provider };
}

function normalizedEndpoint(value?: string): string {
  const raw = (value || '').trim();
  if (!raw) return '';
  try {
    const parsed = new URL(raw.includes('://') ? raw : `https://${raw}`);
    const path = parsed.pathname.replace(/\/+$/, '');
    return `${parsed.protocol.toLowerCase()}//${parsed.host.toLowerCase().replace(/\.$/, '')}${path}`;
  } catch {
    return raw.replace(/\/+$/, '');
  }
}

function normalizedHost(value?: string): string {
  const endpoint = normalizedEndpoint(value);
  if (!endpoint) return '';
  try { return new URL(endpoint).hostname.toLowerCase().replace(/^www\./, ''); }
  catch { return endpoint.toLowerCase().replace(/^www\./, '').split('/')[0]; }
}

function uniqueCatalogName(entries: ProviderCatalogEntry[]): string {
  const names = new Map<string, string>();
  for (const entry of entries) {
    const name = entry.name?.trim();
    if (name) names.set(name.toLowerCase(), name);
  }
  return names.size === 1 ? [...names.values()][0] : '';
}

/** Resolve the primary human-facing label from the shared provider catalogue. */
export function canonicalProviderName(
  connection: ComboConnection,
  catalog: ProviderCatalogEntry[] = [],
): string {
  const endpoint = normalizedEndpoint(connection.base_url);
  const exactName = uniqueCatalogName(catalog.filter(entry => {
    const catalogEndpoint = normalizedEndpoint(entry.base_url);
    return Boolean(endpoint && catalogEndpoint && endpoint === catalogEndpoint);
  }));
  if (exactName) return exactName;

  const oauthProvider = connection.oauth_provider?.trim().toLowerCase();
  const oauthName = uniqueCatalogName(catalog.filter(entry =>
    Boolean(oauthProvider && entry.oauth_provider?.trim().toLowerCase() === oauthProvider),
  ));
  if (oauthName) return oauthName;

  const host = normalizedHost(connection.base_url);
  let isRootEndpoint = false;
  try { isRootEndpoint = new URL(endpoint).pathname.replace(/\/+$/, '') === ''; }
  catch { /* malformed endpoints do not qualify for host inference */ }
  if (host && isRootEndpoint) {
    const hostName = uniqueCatalogName(catalog.filter(entry => {
      const catalogHost = normalizedHost(entry.base_url || entry.domain);
      return Boolean(catalogHost && (host === catalogHost || host.endsWith(`.${catalogHost}`) || catalogHost.endsWith(`.${host}`)));
    }));
    if (hostName) return hostName;
  }

  return connection.name?.trim() || host || connection.id;
}

export function comboProviderOptions(connections: ComboConnection[], catalog: ProviderCatalogEntry[] = []): ComboProviderOption[] {
  const groups = new Map<string, ComboProviderOption>();
  for (const connection of connections) {
    if (connection.is_active !== 1 && connection.is_active !== true) continue;
    const identity = endpointIdentity(connection);
    const account = { id: connection.id, name: connection.name || connection.id };
    const existing = groups.get(identity.id);
    if (existing) {
      existing.connectionIds.push(connection.id);
      existing.accounts.push(account);
      existing.canSync ||= Boolean(connection.base_url);
      existing.searchText += ` ${connection.id} ${account.name}`;
      continue;
    }
    const name = canonicalProviderName(connection, catalog);
    groups.set(identity.id, {
      id: identity.id,
      name,
      provider: identity.provider,
      connectionIds: [connection.id],
      accounts: [account],
      searchText: `${identity.provider} ${name} ${connection.id} ${account.name} ${connection.format || ''}`.trim(),
      canSync: Boolean(connection.base_url),
    });
  }
  return [...groups.values()];
}

export function comboModelsForProvider(models: DiscoveredComboModel[], provider?: ComboProviderOption): string[] {
  if (!provider) return [];
  const ids = new Set(provider.connectionIds);
  return [...new Set(models
    .filter(model => model.connection_id && ids.has(model.connection_id) && (model.is_active === 1 || model.is_active === true))
    .map(model => model.model_id.trim())
    .filter(Boolean))]
    .sort((a, b) => a.localeCompare(b));
}

export function comboProviderForEntry(
  entry: { connection_id?: string; connection_ids?: string[]; provider_id?: string },
  providers: ComboProviderOption[],
): ComboProviderOption | undefined {
  const ids = new Set([entry.connection_id, ...(entry.connection_ids || [])].filter((id): id is string => Boolean(id)));
  return providers.find(provider => provider.id === entry.provider_id
    || provider.connectionIds.some(id => ids.has(id)));
}

export function buildComboEntries(entries: DraftComboEntry[]) {
  return entries
    .map(entry => {
      const model = entry.model.trim();
      const connectionID = entry.connectionId?.trim();
      return connectionID
        ? { model, connection_id: connectionID }
        : { model, provider_id: entry.providerId.trim() };
    })
    .filter(entry => entry.model && ('connection_id' in entry ? entry.connection_id : entry.provider_id));
}
