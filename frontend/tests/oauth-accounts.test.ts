import { describe, expect, it } from 'vitest';
import {
  accountsForProvider,
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
});
