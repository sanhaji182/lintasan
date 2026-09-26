import test from 'node:test';
import assert from 'node:assert/strict';
import {
  comboProviderOptions,
  comboModelsForProvider,
  comboProviderForEntry,
  buildComboEntries,
} from '../src/lib/combo-entry-selector.ts';

const connections = [
  { id: 'qoder-a', name: 'Qoder Alice', format: 'qoder', base_url: 'https://api.qoder.com', chat_path: '/v1/chat/completions', is_active: 1 },
  { id: 'qoder-b', name: 'Qoder Bob', format: 'qoder', base_url: 'https://api.qoder.com/v1', chat_path: '/v1/chat/completions', is_active: true },
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

const providerCatalog = [
  { name: 'Qoder', domain: 'qoder.com', base_url: 'https://api.qoder.com/v1' },
  { name: 'OpenRouter', domain: 'openrouter.ai', base_url: 'https://openrouter.ai/api/v1' },
];

test('provider options group two active accounts into one canonical provider', () => {
  const options = comboProviderOptions(connections, providerCatalog);
  assert.equal(options.length, 2);
  assert.deepEqual(options[0], {
    id: 'provider:qoder:https://api.qoder.com/v1/chat/completions',
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

test('canonical provider metadata wins over generic format and opaque account names', () => {
  const [option] = comboProviderOptions([
    { id: 'account-7', name: 'personal', format: 'openai', base_url: 'https://openrouter.ai/api/v1', chat_path: '/chat/completions', is_active: 1 },
  ], providerCatalog);
  assert.equal(option.name, 'OpenRouter');
  assert.equal(option.accounts[0].name, 'personal');
});

test('multiple connections retain canonical provider as primary and account names as secondary', () => {
  const [option] = comboProviderOptions(connections.slice(0, 2), providerCatalog);
  assert.equal(option.name, 'Qoder');
  assert.deepEqual(option.accounts.map(account => account.name), ['Qoder Alice', 'Qoder Bob']);
});

test('unknown custom providers fall back truthfully to configured connection name', () => {
  const [option] = comboProviderOptions([
    { id: 'custom-1', name: 'Acme Internal', format: 'openai', base_url: 'https://llm.internal.example/v1', chat_path: '/chat/completions', is_active: 1 },
  ], providerCatalog);
  assert.equal(option.name, 'Acme Internal');
  assert.equal(option.provider, 'llm.internal.example');
});

test('shared-host providers match exact normalized endpoints independent of catalogue order', () => {
  const sharedHostCatalog = [
    { name: 'GLM China', domain: 'open.bigmodel.cn', base_url: 'https://open.bigmodel.cn/api/coding/paas/v4' },
    { name: 'Zhipu', domain: 'open.bigmodel.cn', base_url: 'https://open.bigmodel.cn/api/paas/v4/' },
  ];
  const inputs = [
    { id: 'zhipu', name: 'Zhipu account', format: 'openai', base_url: 'https://OPEN.bigmodel.cn/api/paas/v4', is_active: 1 },
    { id: 'glm-cn', name: 'GLM account', format: 'openai', base_url: 'https://open.bigmodel.cn/api/coding/paas/v4/', is_active: 1 },
  ];

  for (const catalog of [sharedHostCatalog, [...sharedHostCatalog].reverse()]) {
    const options = comboProviderOptions(inputs, catalog);
    assert.equal(options.find(option => option.connectionIds.includes('zhipu'))?.name, 'Zhipu');
    assert.equal(options.find(option => option.connectionIds.includes('glm-cn'))?.name, 'GLM China');
  }
});

test('unknown path on a shared preset host falls back to the configured connection name', () => {
  const [option] = comboProviderOptions([
    { id: 'custom-bigmodel', name: 'Internal BigModel Proxy', format: 'openai', base_url: 'https://open.bigmodel.cn/custom/v1', is_active: 1 },
  ], [
    { name: 'Zhipu', domain: 'open.bigmodel.cn', base_url: 'https://open.bigmodel.cn/api/paas/v4' },
    { name: 'GLM China', domain: 'open.bigmodel.cn', base_url: 'https://open.bigmodel.cn/api/coding/paas/v4' },
  ]);
  assert.equal(option.name, 'Internal BigModel Proxy');
});

test('unique provider domain may identify a root endpoint unambiguously', () => {
  const [option] = comboProviderOptions([
    { id: 'qoder-root', name: 'Personal', format: 'qoder', base_url: 'https://api.qoder.com', is_active: 1 },
  ], providerCatalog);
  assert.equal(option.name, 'Qoder');
});

test('custom path on a known unique provider host keeps its configured name', () => {
  const [option] = comboProviderOptions([
    { id: 'qoder-custom', name: 'Qoder Compatible Proxy', format: 'openai', base_url: 'https://api.qoder.com/custom/v1', is_active: 1 },
  ], providerCatalog);
  assert.equal(option.name, 'Qoder Compatible Proxy');
});

test('provider model list is the active union for that provider only', () => {
  const qoder = comboProviderOptions(connections, providerCatalog)[0];
  assert.deepEqual(comboModelsForProvider(models, qoder), ['only-a', 'only-b', 'shared']);
  const other = comboProviderOptions(connections, providerCatalog)[1];
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

test('existing combo entries resolve their canonical provider without changing persisted ids', () => {
  const providers = comboProviderOptions(connections, providerCatalog);
  assert.equal(comboProviderForEntry({ connection_id: 'qoder-b' }, providers)?.name, 'Qoder');
  assert.equal(comboProviderForEntry({ provider_id: providers[0].id }, providers)?.name, 'Qoder');
});

test('provider identity follows effective endpoint and retains scheme and explicit port', () => {
  const options = comboProviderOptions([
    { id: 'equiv-a', format: 'openai', base_url: 'https://api.example.com', chat_path: '/v1/chat/completions', is_active: 1 },
    { id: 'equiv-b', format: 'openai', base_url: 'https://api.example.com/v1', chat_path: '/v1/chat/completions', is_active: 1 },
    { id: 'base-path', format: 'openai', base_url: 'https://api.example.com', chat_path: '/chat/completions', is_active: 1 },
    { id: 'scheme', format: 'openai', base_url: 'http://api.example.com', chat_path: '/v1/chat/completions', is_active: 1 },
    { id: 'port', format: 'openai', base_url: 'https://api.example.com:8443', chat_path: '/v1/chat/completions', is_active: 1 },
  ]);
  assert.equal(options.length, 4);
  assert.deepEqual(options[0].connectionIds, ['equiv-a', 'equiv-b']);
  assert.equal(new Set(options.map(option => option.id)).size, 4);
});
