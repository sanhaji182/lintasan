import test from 'node:test';
import assert from 'node:assert/strict';
import {
  buildCallableCatalog, filterCallableModels, groupCallableModels,
  rememberRecentModel, routingDirtyState, analyticsScope,
  buildPolicyPayload, buildQuotaPayload, comboOrderFingerprint,
} from '../src/lib/workflow-consolidation.ts';

test('catalog combines aliases, combos, provider models and cloud agents without duplicate callable IDs', () => {
  const rows = buildCallableCatalog({
    aliases: { fast: { model: 'gpt-mini' } },
    combos: [{ id: 'combo-1', name: 'core', strategy: 'priority', models: ['gpt-mini'] }],
    connections: [
      { id: 'openai-1', name: 'OpenAI Prod', is_active: 1, provider_kind: 'llm', health_status: 'healthy' },
      { id: 'hoplite-account/a', name: 'Hoplite Work', is_active: 1, provider_kind: 'cloud_agent' },
    ],
    models: [
      { id: 'gpt-mini', connection_id: 'openai-1', owned_by: 'openai', context_window: 128000 },
      { id: 'gpt-mini', connection_id: 'openai-1', owned_by: 'openai' },
      { id: 'hoplite-agent/a/project', connection_id: 'hoplite-account/a', provider_kind: 'cloud_agent', supports_streaming: false },
    ],
  });
  assert.deepEqual(rows.map(row => row.id), ['fast', 'core', 'hoplite-agent/a/project', 'gpt-mini']);
  assert.equal(rows.find(row => row.id === 'gpt-mini')?.account, 'OpenAI Prod');
  assert.equal(rows.find(row => row.id === 'gpt-mini')?.contextWindow, 128000);
  assert.equal(rows.find(row => row.id === 'fast')?.route, 'gpt-mini');
  assert.equal(rows.find(row => row.id === 'hoplite-agent/a/project')?.supportsStreaming, false);
});

test('catalog does not invent missing health, context, capabilities, or price', () => {
  const [row] = buildCallableCatalog({ models: [{ id: 'plain', owned_by: 'custom' }], aliases: {}, combos: [], connections: [] });
  assert.equal(row.health, 'unknown');
  assert.equal(row.contextWindow, null);
  assert.equal(row.price, null);
  assert.deepEqual(row.capabilities, []);
});

test('fuzzy model search tolerates separators and matches route/account metadata', () => {
  const rows = buildCallableCatalog({
    aliases: {}, combos: [],
    connections: [{ id: 'c1', name: 'Production Account', is_active: 1 }],
    models: [{ id: 'deepseek/deepseek-v4-pro', connection_id: 'c1', owned_by: 'command-code' }],
  });
  assert.equal(filterCallableModels(rows, 'deep seek v4').length, 1);
  assert.equal(filterCallableModels(rows, 'production').length, 1);
  assert.equal(filterCallableModels(rows, 'missing').length, 0);
});

test('picker groups recommended/recent, routes, cloud agents and provider models', () => {
  const rows = buildCallableCatalog({
    aliases: { fast: 'gpt-mini' }, combos: [], connections: [],
    models: [{ id: 'hoplite-agent/project', provider_kind: 'cloud_agent' }, { id: 'gpt-mini' }],
  });
  const groups = groupCallableModels(rows, ['gpt-mini'], 'gpt-mini');
  assert.deepEqual(groups.map(g => g.label), ['Recommended & Recent', 'Aliases & Combos', 'Cloud Agents']);
  assert.equal(groups[0].items[0].id, 'gpt-mini');
  assert.equal(groups.flatMap(group => group.items).filter(item => item.id === 'gpt-mini').length, 1);
  assert.equal(rememberRecentModel(['a', 'b', 'a'], 'c', 3).join(','), 'c,a,b');
});

test('routing dirty state reports each explicit save scope', () => {
  assert.deepEqual(routingDirtyState({ policy: true, combos: false, quotas: true }), { count: 2, scopes: ['Policies', 'Quotas'] });
});

test('policy and quota payloads never cross save scopes', () => {
  const smart = {
    ml_router_enabled: true,
    ml_router_cheap_model: 'cheap',
    ml_router_expensive_model: 'expensive',
    ml_router_threshold: '0.7',
    cost_quality_floor: '0.4',
    cost_expensive_anchor: '0.03',
    quota_limits: { old: { max_tokens_per_day: 1 } },
  };
  assert.deepEqual(buildPolicyPayload(smart), {
    ml_router_enabled: true,
    ml_router_cheap_model: 'cheap',
    ml_router_expensive_model: 'expensive',
    ml_router_threshold: '0.7',
    cost_quality_floor: '0.4',
    cost_expensive_anchor: '0.03',
  });
  assert.deepEqual(buildQuotaPayload([{ connId: ' alpha ', maxPerDay: '1200' }, { connId: '', maxPerDay: '3' }, { connId: 'bad', maxPerDay: '0' }]), {
    quota_limits: { alpha: { max_tokens_per_day: 1200 } },
  });
});

test('combo order fingerprint changes when rows are reordered', () => {
  const rows = [{ id: 'a' }, { id: 'b' }];
  assert.notEqual(comboOrderFingerprint(rows), comboOrderFingerprint([...rows].reverse()));
});

test('analytics scope explains a retained snapshot that differs from global counter', () => {
  assert.deepEqual(analyticsScope(28, 20), {
    globalLabel: 'All recorded requests', snapshotLabel: '20 retained request rows',
    note: '8 requests are outside the retained log snapshot or its current filters.', reconciled: false,
  });
  assert.equal(analyticsScope(20, 20).reconciled, true);
});
