export interface ComboConnection {
  id: string;
  name?: string;
  format?: string;
  base_url?: string;
  is_active?: number | boolean;
}

export interface DiscoveredComboModel {
  model_id: string;
  connection_id?: string;
  is_active?: number | boolean;
}

export interface DraftComboEntry {
  model: string;
  connectionId: string;
}

export function comboProviderOptions(connections: ComboConnection[]) {
  return connections
    .filter(connection => connection.is_active === 1 || connection.is_active === true)
    .map(connection => {
      let provider = connection.format || 'custom';
      try { provider = new URL(connection.base_url || '').hostname || provider; } catch { /* use format */ }
      const name = connection.name || connection.id;
      return {
        id: connection.id,
        name,
        provider,
        searchText: `${provider} ${name} ${connection.id} ${connection.format || ''}`.trim(),
        canSync: Boolean(connection.base_url),
      };
    });
}

export function comboModelsForConnection(models: DiscoveredComboModel[], connectionId: string): string[] {
  return [...new Set(models
    .filter(model => model.connection_id === connectionId && (model.is_active === 1 || model.is_active === true))
    .map(model => model.model_id.trim())
    .filter(Boolean))]
    .sort((a, b) => a.localeCompare(b));
}

export function buildPinnedComboEntries(entries: DraftComboEntry[]) {
  return entries
    .map(entry => ({ model: entry.model.trim(), connection_id: entry.connectionId.trim() }))
    .filter(entry => entry.model && entry.connection_id);
}
