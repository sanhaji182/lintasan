import test from 'node:test';
import assert from 'node:assert/strict';
import {
  comboProviderOptions,
  comboModelsForProvider,
  buildComboEntries,
} from '../src/lib/combo-entry-selector.ts';

const connections = [
  { id: 'qoder-a', name: 'Qoder Alice', format: 'qoder', base_url: 'https://api.qoder.com', chat_path: '/chat/completions', is_active: 1 },
  { id: 'qoder-b', name: 'Qoder Bob', format: 'qoder', base_url: 'https://api.qoder.com/', chat_path: '/chat/completions', is_active: true },
  { id: 'other-a', name: 'Other', format: 'openai', base_url: 'https://other.example/v1', chat_path: '/v1/chat/completions', is_active: 1 },
  { id: 'off', name: 'Disabled', format: 'qoder', base_url: 'https://api.qoder.com', chat_path: '/chat/completions', is_active: 0 },
];

const models = [
  { model_id: 'shared', connection_id: 'qoder-a', is_active: 1 },
  { model_id: 'only-a', connection_id: 'qoder-a', is_active: true },
  { model_id: 'only-b', connection_id: 'qoder-b', is_active: 1 },
  { model_id: 'shared', connection_id: 'other-a', is_active: 1 },
  { model_id: 'disabled', connection_id: 'qoder-b', is_active: 0 },
];

test('provider options group two active accounts into one canonical provider', () => {
  const options = comboProviderOptions(connections);
  assert.equal(options.length, 2);
  assert.deepEqual(options[0], {
    id: 'provider:qoder:api.qoder.com:/chat/completions',
    name: 'Qoder',
    provider: 'api.qoder.com',
    connectionIds: ['qoder-a', 'qoder-b'],
    accounts: [
      { id: 'qoder-a', name: 'Qoder Alice' },
      { id: 'qoder-b', name: 'Qoder Bob' },
    ],
    searchText: 'api.qoder.com Qoder qoder-a Qoder Alice qoder qoder-b Qoder Bob',
    canSync: true,
  });
});

test('provider model list is the active union for that provider only', () => {
  const qoder = comboProviderOptions(connections)[0];
  assert.deepEqual(comboModelsForProvider(models, qoder), ['only-a', 'only-b', 'shared']);
  const other = comboProviderOptions(connections)[1];
  assert.deepEqual(comboModelsForProvider(models, other), ['shared']);
});

test('provider entries persist canonical pool identity while advanced pin persists exact account', () => {
  assert.deepEqual(buildComboEntries([
    { model: 'shared', providerId: 'provider:qoder:api.qoder.com:/chat/completions' },
    { model: 'shared', providerId: 'provider:qoder:api.qoder.com:/chat/completions', connectionId: 'qoder-b' },
  ]), [
    { model: 'shared', provider_id: 'provider:qoder:api.qoder.com:/chat/completions' },
    { model: 'shared', connection_id: 'qoder-b' },
  ]);
});
