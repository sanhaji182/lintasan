import test from 'node:test';
import assert from 'node:assert/strict';
import { deriveOverview } from '../src/lib/overview-state.ts';

test('overview derives only metrics backed by successful sources', () => {
  const state = deriveOverview({
    health: { status: 'fulfilled', value: { status: 'ok', version: 'v1.2.3' } },
    stats: { status: 'fulfilled', value: { total_requests: 42, cache_hit_rate: 25, avg_latency: 175, uptime: '1h' } },
    logs: { status: 'fulfilled', value: [{ input_tokens: 10, output_tokens: 20 }] },
    connections: { status: 'fulfilled', value: [{ id: 'a', name: 'A', is_active: 1 }, { id: 'b', name: 'B', is_active: 0 }] },
  });
  assert.equal(state.gateway.label, 'Operational');
  assert.equal(state.requests.value, '42');
  assert.equal(state.tokens.value, '30');
  assert.equal(state.providers.value, '1 / 2');
  assert.deepEqual(state.failedSources, []);
});

test('overview never converts failed sources into zero metrics', () => {
  const failed = { status: 'rejected', reason: new Error('offline') };
  const state = deriveOverview({ health: failed, stats: failed, logs: failed, connections: failed });
  assert.equal(state.gateway.label, 'Status unavailable');
  assert.equal(state.requests.value, 'Unavailable');
  assert.equal(state.tokens.value, 'Unavailable');
  assert.equal(state.providers.value, 'Unavailable');
  assert.deepEqual(state.failedSources, ['Gateway health', 'Traffic stats', 'Recent requests', 'Connections']);
});

test('overview treats fulfilled partial envelopes as partial instead of fabricating values', () => {
  const state = deriveOverview({
    health: { status: 'fulfilled', value: { status: 'ok' } },
    stats: { status: 'fulfilled', value: {} },
    logs: { status: 'fulfilled', value: [{}] },
    connections: { status: 'fulfilled', value: [] },
  });
  assert.equal(state.requests.value, 'Unavailable');
  assert.equal(state.latency.value, 'Unavailable');
  assert.equal(state.tokens.value, 'Unavailable');
  assert.equal(state.providers.value, '0 / 0');
  assert.deepEqual(state.failedSources, ['Traffic stats (partial)', 'Recent requests (partial)']);
});

test('overview validates fields independently while preserving legitimate zero', () => {
  const cases = [
    { value: {}, expected: ['Unavailable', 'Unavailable'] },
    { value: { total_requests: null, avg_latency: null }, expected: ['Unavailable', 'Unavailable'] },
    { value: { total_requests: '0', avg_latency: '0' }, expected: ['Unavailable', 'Unavailable'] },
    { value: { total_requests: Number.NaN, avg_latency: Number.NaN }, expected: ['Unavailable', 'Unavailable'] },
    { value: { total_requests: -1, avg_latency: -1 }, expected: ['Unavailable', 'Unavailable'] },
    { value: { total_requests: 0, avg_latency: 0 }, expected: ['0', '0ms'] },
  ];
  for (const { value, expected } of cases) {
    const state = deriveOverview({
      health: { status: 'fulfilled', value: { status: 'ok' } },
      stats: { status: 'fulfilled', value },
      logs: { status: 'fulfilled', value: [] },
      connections: { status: 'fulfilled', value: [] },
    });
    assert.deepEqual([state.requests.value, state.latency.value], expected);
  }
});

test('overview rejects malformed health, logs and connections independently', () => {
  const state = deriveOverview({
    health: { status: 'fulfilled', value: { status: 42 } },
    stats: { status: 'fulfilled', value: { total_requests: 0, avg_latency: 0 } },
    logs: { status: 'fulfilled', value: [{ input_tokens: 0, output_tokens: -1 }] },
    connections: { status: 'fulfilled', value: [{ id: 'bad', name: 'Bad', is_active: 'yes' }] },
  });
  assert.equal(state.gateway.label, 'Status unavailable');
  assert.equal(state.tokens.value, 'Unavailable');
  assert.equal(state.providers.value, 'Unavailable');
  assert.deepEqual(state.failedSources, ['Gateway health (partial)', 'Recent requests (partial)', 'Connections (partial)']);
});
