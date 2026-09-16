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
