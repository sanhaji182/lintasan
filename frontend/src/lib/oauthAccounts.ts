export interface OAuthAccount {
  id: string;
  provider: string;
  status: string;
  health: 'healthy' | 'expiring' | 'restricted' | 'expired' | 'revoked' | string;
  created_at?: string;
  expires_at?: string;
  masked_token?: string;
  account_label?: string;
  requests?: number;
  failed_requests?: number;
  input_tokens?: number;
  output_tokens?: number;
  last_used_at?: string;
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

/** Plain-language name for an OAuth flow, avoiding jargon in the UI. */
export function flowLabel(flow: string): string {
  switch (flow) {
    case 'authorization_code_pkce':
      return 'Browser login';
    case 'authorization_code':
    case 'browser_redirect':
      return 'Browser login';
    case 'device_code':
      return 'Device code';
    case 'import_token':
    case 'import_state_vscdb':
    case 'token_import':
      return 'Token import';
    case 'custom_device':
      return 'Device approval';
    case 'local_app_callback':
      return 'App callback';
    case 'cli_poll':
      return 'CLI approval';
    case 'rsa_key_exchange':
      return 'Key-pair login';
    default:
      return 'OAuth';
  }
}

/** One sentence telling the operator what they will actually do. */
export function flowDescription(flow: string): string {
  switch (flow) {
    case 'authorization_code_pkce':
    case 'authorization_code':
    case 'browser_redirect':
      return 'A login window opens. Sign in with the provider account you already use — no token copying.';
    case 'device_code':
      return 'You enter a short code on the provider page and this page waits for the login to finish.';
    case 'import_token':
    case 'import_state_vscdb':
    case 'token_import':
      return 'Paste a token that already exists on your machine. There is no browser login for this provider.';
    case 'custom_device':
      return 'Approve the login in the provider app or page; this page polls until it completes.';
    case 'local_app_callback':
      return 'The provider app hands the session back to this page after you approve it.';
    case 'cli_poll':
      return 'Approve the login in the vendor CLI; this page polls until the account is ready.';
    case 'rsa_key_exchange':
      return 'Uses a key-pair callback instead of OAuth2.';
    default:
      return 'Starts the provider authorization flow.';
  }
}

/** What a health state means for requests, in operator terms. */
export function healthDescription(health: string): string {
  switch (health) {
    case 'healthy':
      return 'Usable now. Requests can be routed through this account.';
    case 'expiring':
      return 'Usable, but the token is close to expiry. Refresh or reconnect soon.';
    case 'restricted':
      return 'The provider has gated this account. Requests may be refused.';
    case 'expired':
      return 'The token has expired. Reconnect to use it again.';
    case 'revoked':
      return 'The session was revoked locally and is no longer used. Remove it to clear the row.';
    default:
      return 'Health reported by the backend.';
  }
}

function parseBackendDate(value?: string): Date | null {
  if (!value) return null;
  const raw = value.trim();
  if (!raw) return null;
  // SQLite stores "YYYY-MM-DD HH:MM:SS" (localtime, no zone); Safari rejects
  // that form, so normalize it to ISO before parsing.
  const normalized = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(raw) ? raw.replace(' ', 'T') : raw;
  const date = new Date(normalized);
  return Number.isNaN(date.getTime()) ? null : date;
}

function shortDuration(ms: number): string {
  const minutes = Math.round(ms / 60000);
  if (minutes < 1) return 'just now';
  if (minutes < 60) return `${minutes}m`;
  if (minutes < 1440) return `${Math.round(minutes / 60)}h`;
  return `${Math.round(minutes / 1440)}d`;
}

/** "in 2h" / "3h ago" for expiry timestamps. Empty when unavailable. */
export function formatUntil(value?: string, now: number = Date.now()): string {
  const date = parseBackendDate(value);
  if (!date) return '';
  const diff = date.getTime() - now;
  if (Math.abs(diff) < 60000) return 'now';
  return diff > 0 ? `in ${shortDuration(diff)}` : `${shortDuration(Math.abs(diff))} ago`;
}

/** "added 2d ago" fragment for created timestamps. Empty when unavailable. */
export function formatAge(value?: string, now: number = Date.now()): string {
  const date = parseBackendDate(value);
  if (!date) return '';
  const diff = now - date.getTime();
  if (diff < 60000) return 'just now';
  return `${shortDuration(diff)} ago`;
}

/** "Expires in 2h" / "Expired 3h ago" for an expiry timestamp. */
export function expiryLabel(value?: string, now: number = Date.now()): string {
  const relative = formatUntil(value, now);
  if (!relative) return '';
  return relative.endsWith('ago') ? `Expired ${relative}` : `Expires ${relative}`;
}
