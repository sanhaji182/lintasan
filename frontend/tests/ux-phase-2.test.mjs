import test from 'node:test';
import assert from 'node:assert/strict';
import {
  buildCallableCatalog, filterCallableModels, groupCallableModels,
  rememberRecentModel, routingDirtyState, analyticsScope,
  buildPolicyPayload, buildQuotaPayload, comboOrderFingerprint, shouldUseStreaming,
  sourceFreshness, selectCatalogModel,
} from '../src/lib/workflow-consolidation.ts';

test('catalog combines aliases, combos and provider models without duplicate callable IDs', () => {
  const rows = buildCallableCatalog({
    aliases: { fast: { model: 'gpt-mini' } },
    combos: [{ id: 'combo-1', name: 'core', strategy: 'priority', models: ['gpt-mini'] }],
    connections: [
      { id: 'openai-1', name: 'OpenAI Prod', is_active: 1, provider_kind: 'llm', health_status: 'healthy' },
    ],
    models: [
      { id: 'gpt-mini', connection_id: 'openai-1', owned_by: 'openai', context_window_tokens: 128000 },
      { id: 'gpt-mini', connection_id: 'openai-1', owned_by: 'openai' },
    ],
  });
  assert.deepEqual(rows.map(row => row.id), ['fast', 'core', 'gpt-mini']);
  assert.equal(rows.find(row => row.id === 'gpt-mini')?.account, 'OpenAI Prod');
  assert.equal(rows.find(row => row.id === 'gpt-mini')?.contextWindow, 128000);
  assert.equal(rows.find(row => row.id === 'fast')?.route, 'gpt-mini');
});

test('catalog preserves authoritative context metadata and legacy compatibility', () => {
  const rows = buildCallableCatalog({
    models: [
      { id: 'authoritative', context_window_tokens: 200000, context_window: 128000 },
      { id: 'legacy', context_window: 64000 },
    ],
    aliases: {}, combos: [], connections: [],
  });
  assert.equal(rows.find(row => row.id === 'authoritative')?.contextWindow, 200000);
  assert.equal(rows.find(row => row.id === 'legacy')?.contextWindow, 64000);
});

test('catalog does not invent missing health, context, capabilities, or price', () => {
  const [row] = buildCallableCatalog({ models: [{ id: 'plain', owned_by: 'custom' }], aliases: {}, combos: [], connections: [] });
  assert.equal(row.health, 'unknown');
  assert.equal(row.contextWindow, null);
  assert.equal(row.price, null);
  assert.deepEqual(row.capabilities, []);
});

test('playground selects only models present in the loaded callable catalog', () => {
  const rows = buildCallableCatalog({
    models: [{ id: 'catalog-first' }, { id: 'remembered-model' }],
    aliases: {}, combos: [], connections: [],
  });
  assert.equal(selectCatalogModel(rows, 'catalog-first', 'remembered-model'), 'catalog-first');
  assert.equal(selectCatalogModel(rows, 'stale-query-model', 'remembered-model'), 'remembered-model');
  assert.equal(selectCatalogModel(rows, 'stale-query-model', 'stale-remembered-model'), 'catalog-first');
  assert.equal(selectCatalogModel([], 'stale-query-model', 'stale-remembered-model'), '');
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

test('picker groups recommended/recent, routes and provider models', () => {
  const rows = buildCallableCatalog({
    aliases: { fast: 'gpt-mini' }, combos: [], connections: [],
    models: [{ id: 'gpt-mini' }],
  });
  const groups = groupCallableModels(rows, ['gpt-mini'], 'gpt-mini');
  assert.deepEqual(groups.map(g => g.label), ['Recommended & Recent', 'Aliases & Combos']);
  assert.equal(groups[0].items[0].id, 'gpt-mini');
  assert.equal(groups.flatMap(group => group.items).filter(item => item.id === 'gpt-mini').length, 1);
  assert.equal(rememberRecentModel(['a', 'b', 'a'], 'c', 3).join(','), 'c,a,b');
});

test('streaming is disabled for any capability that declares it unsupported', () => {
  assert.equal(shouldUseStreaming({ id: 'non-streaming-combo', kind: 'route', supportsStreaming: false }), false);
  assert.equal(shouldUseStreaming({ id: 'gpt-mini', kind: 'provider', supportsStreaming: null }), true);
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

test('analytics scope preserves signed discrepancies for either query completion order', () => {
  const globalFinishedLater = analyticsScope(28, 20);
  assert.deepEqual(globalFinishedLater, {
    globalLabel: 'All recorded requests', snapshotLabel: '20 retained request rows',
    note: 'The all-recorded counter exceeds the retained rows by 8. These independently collected sources do not reconcile.',
    discrepancy: 8, reconciled: false, state: 'mismatch',
  });

  const snapshotFinishedLater = analyticsScope(10, 11);
  assert.deepEqual(snapshotFinishedLater, {
    globalLabel: 'All recorded requests', snapshotLabel: '11 retained request rows',
    note: 'The retained rows exceed the all-recorded counter by 1. These independently collected sources do not reconcile.',
    discrepancy: -1, reconciled: false, state: 'mismatch',
  });

  assert.notEqual(globalFinishedLater.discrepancy, snapshotFinishedLater.discrepancy);
  assert.equal(snapshotFinishedLater.reconciled, false, 'a reverse race mismatch must not be presented as reconciled');
  assert.match(snapshotFinishedLater.note, /skew|mismatch|do not reconcile|unknown/i);
});

test('analytics scope reconciles exactly when both comparable counts match', () => {
  assert.deepEqual(analyticsScope(20, 20), {
    globalLabel: 'All recorded requests', snapshotLabel: '20 retained request rows',
    note: 'The independently collected counts match.', discrepancy: 0, reconciled: true, state: 'reconciled',
  });
});

test('analytics scope is explicitly unknown for missing or incomparable counts', () => {
  const expected = {
    globalLabel: 'All-recorded request count unavailable',
    snapshotLabel: 'Retained request row count unavailable',
    note: 'Reconciliation is unknown because comparable counts are unavailable.',
    discrepancy: null,
    reconciled: false,
    state: 'unknown',
  };
  assert.deepEqual(analyticsScope(null, undefined), expected);
  assert.deepEqual(analyticsScope(Number.NaN, Number.POSITIVE_INFINITY), expected);
  assert.deepEqual(analyticsScope(Number.NEGATIVE_INFINITY, 20), {
    ...expected,
    snapshotLabel: '20 retained request rows',
  });
});

test('analytics scope does not invent filters or mismatch causes absent from its inputs', () => {
  for (const result of [analyticsScope(28, 20), analyticsScope(10, 11), analyticsScope(null, 11)]) {
    assert.doesNotMatch(result.note, /filter|sampling|retention policy|delayed aggregation|excluded categor/i);
  }
});

test('source freshness distinguishes fresh, stale, failed and unknown collection state', () => {
  const now = Date.parse('2026-09-15T18:00:45Z');
  assert.deepEqual(sourceFreshness(null, now, false), { state: 'unknown', ageMs: null });
  assert.deepEqual(sourceFreshness(null, now, true), { state: 'unknown', ageMs: null });
  assert.deepEqual(sourceFreshness(now - 5_000, now, false), { state: 'fresh', ageMs: 5_000 });
  assert.deepEqual(sourceFreshness(now - 31_000, now, false), { state: 'stale', ageMs: 31_000 });
  assert.deepEqual(sourceFreshness(now - 1_000, now, true), { state: 'stale', ageMs: 1_000 });
});
