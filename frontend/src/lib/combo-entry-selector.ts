export interface ComboConnection {
  id: string;
  name?: string;
  format?: string;
  base_url?: string;
  chat_path?: string;
  pool_id?: string;
  is_active?: number | boolean;
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

function displayName(connection: ComboConnection, provider: string): string {
  const format = (connection.format || '').trim();
  if (format) return format.charAt(0).toUpperCase() + format.slice(1);
  return provider || connection.name || connection.id;
}

export function comboProviderOptions(connections: ComboConnection[]): ComboProviderOption[] {
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
    const name = displayName(connection, identity.provider);
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
