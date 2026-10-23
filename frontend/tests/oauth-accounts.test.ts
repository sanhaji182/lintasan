import { describe, expect, it } from 'vitest';
import {
  accountsForProvider,
  flowDescription,
  flowLabel,
  formatAge,
  formatUntil,
  expiryLabel,
  healthDescription,
  healthLabel,
  providerAccountState,
  quotaLabel,
  wireState,
  wireStateLabel,
  type OAuthAccount,
  type OAuthProviderAccounts
} from '../src/lib/oauthAccounts';

const providers: OAuthProviderAccounts[] = [
  {
    provider: 'codex',
    name: 'OpenAI Codex',
    active: 2,
    expiring: 0,
    restricted: 1,
    expired: 0,
    revoked: 0,
    total: 3,
    wired: true,
    connection_id: 'conn-1',
    connection_name: 'oauth-codex'
  },
  {
    provider: 'claude',
    active: 0,
    expiring: 0,
    restricted: 1,
    expired: 0,
    revoked: 0,
    total: 1,
    wired: true
  }
];

const accounts: OAuthAccount[] = [
  { id: '1', provider: 'codex', status: 'active', health: 'healthy', masked_token: 'abcd…wxyz' },
  { id: '2', provider: 'codex', status: 'restricted', health: 'restricted' },
  { id: '3', provider: 'claude', status: 'restricted', health: 'restricted' }
];

describe('OAuth account dashboard state', () => {
  it('groups accounts without hiding restricted rows', () => {
    const codex = accountsForProvider(accounts, 'codex');
    expect(codex).toHaveLength(2);
    expect(codex.map((row) => row.health)).toContain('restricted');
  });

  it('derives honest wire states', () => {
    expect(wireState(providerAccountState(providers, 'codex'))).toBe('healthy');
    expect(wireStateLabel(providerAccountState(providers, 'codex'))).toBe('Wired & active');
    expect(wireState(providerAccountState(providers, 'claude'))).toBe('error');
    expect(wireStateLabel(providerAccountState(providers, 'claude'))).toBe('Wired · token error');
    expect(wireState(providerAccountState(providers, 'xai'))).toBe('unwired');
  });

  it('shows unavailable quota truthfully', () => {
    expect(quotaLabel()).toBe('–');
  });

  it('uses explicit human-readable health labels', () => {
    expect(healthLabel('restricted')).toBe('Restricted');
    expect(healthLabel('expired')).toBe('Expired');
  });

  it('explains flow and health states without jargon', () => {
    expect(flowLabel('authorization_code_pkce')).toBe('Browser login');
    expect(flowLabel('token_import')).toBe('Token import');
    expect(flowDescription('device_code')).toMatch(/short code/i);
    expect(healthDescription('healthy')).toMatch(/usable/i);
    expect(healthDescription('restricted')).toMatch(/gated/i);
  });

  it('formats expiry/age from backend timestamps', () => {
    const now = Date.parse('2026-10-23T12:00:00Z');
    expect(formatUntil('2026-10-23T14:00:00Z', now)).toBe('in 2h');
    expect(formatUntil('2026-10-23T09:00:00Z', now)).toBe('3h ago');
    expect(formatUntil('not-a-date', now)).toBe('');
    expect(formatAge('2026-10-21T12:00:00Z', now)).toBe('2d ago');
    expect(formatAge('', now)).toBe('');
    // SQLite's "YYYY-MM-DD HH:MM:SS" form must parse on every browser.
    expect(formatUntil('2026-10-23 14:00:00', now)).not.toBe('');
    expect(expiryLabel('2026-10-23T14:00:00Z', now)).toBe('Expires in 2h');
    expect(expiryLabel('2026-10-23T09:00:00Z', now)).toBe('Expired 3h ago');
    expect(expiryLabel('', now)).toBe('');
  });
});
