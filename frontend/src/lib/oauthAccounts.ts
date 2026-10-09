export interface OAuthAccount {
  id: string;
  provider: string;
  status: string;
  health: 'healthy' | 'expiring' | 'restricted' | 'expired' | 'revoked' | string;
  created_at?: string;
  expires_at?: string;
  masked_token?: string;
}

export interface OAuthProviderAccounts {
  provider: string;
  name?: string;
  active: number;
  expiring: number;
  restricted: number;
  expired: number;
  revoked: number;
  total: number;
  wired: boolean;
  connection_id?: string;
  connection_name?: string;
}

export interface OAuthAccountsResponse {
  enabled: boolean;
  disclaimer?: string;
  accounts: OAuthAccount[];
  providers: OAuthProviderAccounts[];
}

export type WireState = 'unwired' | 'healthy' | 'error';

export function accountsForProvider(accounts: OAuthAccount[], provider: string): OAuthAccount[] {
  return accounts.filter((account) => account.provider === provider);
}

export function providerAccountState(
  providers: OAuthProviderAccounts[],
  provider: string
): OAuthProviderAccounts | undefined {
  return providers.find((entry) => entry.provider === provider);
}

export function wireState(provider?: OAuthProviderAccounts): WireState {
  if (!provider?.wired) return 'unwired';
  if (provider.active + provider.expiring > 0) return 'healthy';
  return 'error';
}

export function wireStateLabel(provider?: OAuthProviderAccounts): string {
  switch (wireState(provider)) {
    case 'healthy':
      return 'Wired & active';
    case 'error':
      return 'Wired · token error';
    default:
      return 'Not wired';
  }
}

export function healthLabel(health: string): string {
  switch (health) {
    case 'healthy':
      return 'Healthy';
    case 'expiring':
      return 'Expiring';
    case 'restricted':
      return 'Restricted';
    case 'expired':
      return 'Expired';
    case 'revoked':
      return 'Revoked';
    default:
      return health || 'Unknown';
  }
}

export function quotaLabel(): string {
  // The backend contract intentionally does not invent quota data. Keep the UI
  // truthful until a provider exposes a real quota endpoint.
  return '–';
}
