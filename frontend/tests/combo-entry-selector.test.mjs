import test from 'node:test';
import assert from 'node:assert/strict';
import {
  comboProviderOptions,
  comboModelsForConnection,
  buildPinnedComboEntries,
} from '../src/lib/combo-entry-selector.ts';

test('provider options expose searchable provider identity and connection name', () => {
  const options = comboProviderOptions([
    { id: 'sumo-1', name: 'Primary Account', format: 'openai', base_url: 'https://ai.sumopod.com/v1', is_active: 1 },
    { id: 'off', name: 'Disabled', format: 'anthropic', base_url: 'https://example.test', is_active: 0 },
  ]);
  assert.deepEqual(options, [{
    id: 'sumo-1',
    name: 'Primary Account',
    provider: 'ai.sumopod.com',
    searchText: 'ai.sumopod.com Primary Account sumo-1 openai',
    canSync: true,
  }]);
});

test('models are filtered to the selected connection and active discovery rows', () => {
  const rows = [
    { model_id: 'shared-model', connection_id: 'a', is_active: 1 },
    { model_id: 'only-a', connection_id: 'a', is_active: true },
    { model_id: 'disabled-a', connection_id: 'a', is_active: 0 },
    { model_id: 'shared-model', connection_id: 'b', is_active: 1 },
  ];
  assert.deepEqual(comboModelsForConnection(rows, 'a'), ['only-a', 'shared-model']);
  assert.deepEqual(comboModelsForConnection(rows, 'b'), ['shared-model']);
});

test('duplicate model names on different connections remain distinct pinned entries', () => {
  assert.deepEqual(buildPinnedComboEntries([
    { model: 'shared-model', connectionId: 'a' },
    { model: 'shared-model', connectionId: 'b' },
  ]), [
    { model: 'shared-model', connection_id: 'a' },
    { model: 'shared-model', connection_id: 'b' },
  ]);
});
