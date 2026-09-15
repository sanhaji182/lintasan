export interface CreatedGatewayKey {
  id?: string;
  name?: string;
  key?: string;
}

export interface OneTimeSecret {
  id: string;
  name: string;
  value: string;
}

/**
 * Converts only a freshly-created POST response into ephemeral UI state.
 * Callers must never pass data from the masked GET collection here.
 */
export function captureOneTimeSecret(created: CreatedGatewayKey): OneTimeSecret | null {
  const value = typeof created.key === 'string' ? created.key.trim() : '';
  if (!value) return null;
  return {
    id: created.id || '',
    name: created.name?.trim() || 'Unnamed key',
    value,
  };
}

export function clearOneTimeSecret(): null {
  return null;
}

export function maskedKeyLabel(key: { prefix?: string }): string {
  return `${key.prefix?.trim() || 'Configured'} (masked)`;
}
