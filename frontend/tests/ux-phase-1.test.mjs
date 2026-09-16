import test from 'node:test';
import assert from 'node:assert/strict';

import {
  deriveQuickstart,
  maskGatewayKey,
  recommendCallableModel,
  buildQuickstartSnippets,
} from '../src/lib/quickstart.ts';
import { navigationGroups, dashboardRoutes, groupIsInitiallyOpen, routeIsActive, searchNavigation } from '../src/lib/navigation.ts';
import { deriveLandingMetrics } from '../src/lib/landing-metrics.ts';

test('Quickstart derives an incomplete but actionable empty state', () => {
  const state = deriveQuickstart({ connections: [], keys: [], models: [], baseUrl: 'https://lintasan.example/v1' });
  assert.equal(state.completedSteps, 1);
  assert.equal(state.connection.ready, false);
  assert.equal(state.key.ready, false);
  assert.equal(state.model.ready, false);
  assert.equal(state.endpoint.ready, true);
  assert.equal(state.playground.ready, false);
});

test('Quickstart accepts only active healthy connections and callable models', () => {
  const state = deriveQuickstart({
    connections: [
      { id: 'down', name: 'Down', is_active: 1, health_status: 'unhealthy' },
      { id: 'ready', name: 'Ready', is_active: 1, health_status: 'healthy' },
    ],
    keys: [{ id: 'key-1', name: 'App', key: 'sk-lintasan-supersecret', is_active: true }],
    models: [{ id: '' }, { id: 'hoplite-agent/project', provider_kind: 'cloud_agent', connection_id: 'other' }, { id: 'gpt-4o-mini', owned_by: 'Ready', connection_id: 'ready', source: 'discovered' }],
    baseUrl: 'https://lintasan.example/v1',
  });
  assert.equal(state.completedSteps, 5);
  assert.equal(state.connection.label, 'Ready');
  assert.equal(state.key.label, 'App · sk-l…cret');
  assert.equal(state.model.label, 'gpt-4o-mini');
});

test('model recommendation prefers a standard callable model over a long-running cloud agent', () => {
  assert.equal(recommendCallableModel([
    { id: 'hoplite-agent/project', provider_kind: 'cloud_agent', connection_id: 'hoplite' },
    { id: 'deepseek-chat', owned_by: 'DeepSeek', connection_id: 'deepseek', source: 'discovered' },
  ], [{ id: 'deepseek', name: 'DeepSeek', is_active: 1 }])?.id, 'deepseek-chat');
  assert.equal(recommendCallableModel([]), null);
});

test('Quickstart does not recommend catalog fallback models unrelated to active connections', () => {
  const state = deriveQuickstart({
    connections: [{ id: 'ready', name: 'My OpenAI', is_active: 1 }],
    keys: [{ id: 'key-1', name: 'App', prefix: 'sk-l…cdef' }],
    models: [{ id: 'catalog-only', owned_by: 'My OpenAI', source: 'catalog' }],
    baseUrl: 'https://lintasan.example/v1',
  });
  assert.equal(state.model.ready, false);
  assert.equal(state.recommendedModel, null);
});

test('Quickstart correlates cloud-agent recommendations to the active account', () => {
  const state = deriveQuickstart({
    connections: [{ id: 'hoplite-a', name: 'Hoplite A', is_active: 1 }],
    keys: [],
    models: [{ id: 'hoplite-agent/other/project', provider_kind: 'cloud_agent', connection_id: 'hoplite-b', source: 'dynamic' }],
    baseUrl: 'https://lintasan.example/v1',
  });
  assert.equal(state.recommendedModel, null);
});

test('Quickstart finds a callable model on any healthy active connection', () => {
  const state = deriveQuickstart({
    connections: [
      { id: 'first', name: 'First', is_active: 1 },
      { id: 'second', name: 'Second', is_active: 1 },
    ],
    keys: [],
    models: [{ id: 'callable', connection_id: 'second', source: 'discovered' }],
    baseUrl: 'https://lintasan.example/v1',
  });
  assert.equal(state.recommendedModel?.id, 'callable');
  assert.equal(state.connection.label, 'Second');
});

test('Quickstart fails closed when models exist but no active connection exists', () => {
  const state = deriveQuickstart({
    connections: [],
    keys: [],
    models: [{ id: 'orphan', connection_id: 'missing', source: 'discovered' }],
    baseUrl: 'https://lintasan.example/v1',
  });
  assert.equal(state.recommendedModel, null);
  assert.equal(state.model.ready, false);
});

test('gateway key masking never returns the full secret', () => {
  const secret = 'sk-lintasan-1234567890abcdef';
  const masked = maskGatewayKey(secret);
  assert.equal(masked, 'sk-l…cdef');
  assert.equal(masked.includes(secret), false);
});

test('snippets use real endpoint/model and a safe placeholder when key is not newly issued', () => {
  const snippets = buildQuickstartSnippets({
    baseUrl: 'https://lintasan.example/v1', model: 'deepseek-chat', gatewayKey: null,
  });
  for (const source of Object.values(snippets)) {
    assert.match(source, /https:\/\/lintasan\.example\/v1/);
    assert.match(source, /deepseek-chat/);
    assert.match(source, /YOUR_LINTASAN_API_KEY/);
    assert.doesNotMatch(source, /undefined|null/);
  }
  assert.match(snippets.curl, /Bearer YOUR_LINTASAN_API_KEY/);
  assert.match(snippets.javascript, /apiKey: "YOUR_LINTASAN_API_KEY"/);
});

test('snippets use a newly issued gateway key in every copyable example', () => {
  const issued = 'sk-lintasan-newly-issued';
  const snippets = buildQuickstartSnippets({
    baseUrl: 'https://lintasan.example/v1', model: 'deepseek-chat', gatewayKey: issued,
  });
  for (const source of Object.values(snippets)) assert.match(source, new RegExp(issued));
});

test('sidebar intent groups preserve every dashboard route and progressive disclosure', () => {
  const expected = [
    '/dashboard', '/dashboard/quickstart', '/dashboard/playground', '/dashboard/connections',
    '/dashboard/models', '/dashboard/providers', '/dashboard/routing', '/dashboard/analytics', '/dashboard/keys',
    '/dashboard/teams', '/dashboard/users', '/dashboard/webhooks', '/dashboard/memory',
    '/dashboard/mcp', '/dashboard/translator', '/dashboard/plugins', '/dashboard/backup',
    '/dashboard/migrate', '/dashboard/experimental', '/dashboard/settings', '/dashboard/docs',
  ];
  assert.deepEqual(new Set(dashboardRoutes), new Set(expected));
  assert.deepEqual(navigationGroups.map(group => group.label), ['Operate', 'Build', 'Observe', 'Configure']);
  assert.equal(groupIsInitiallyOpen('Operate', '/dashboard'), true);
  assert.equal(groupIsInitiallyOpen('Build', '/dashboard'), true);
  assert.equal(groupIsInitiallyOpen('Observe', '/dashboard'), true);
  assert.equal(groupIsInitiallyOpen('Configure', '/dashboard'), false);
  assert.equal(groupIsInitiallyOpen('Configure', '/dashboard/memory'), true);
  assert.equal(routeIsActive('/dashboard/connections', '/dashboard/discover'), true);
  assert.equal(routeIsActive('/dashboard/analytics', '/dashboard/logs'), true);
});

test('command search returns only real destinations and matches intent metadata', () => {
  assert.deepEqual(searchNavigation('provider').map(item => item.path), [
    '/dashboard/connections', '/dashboard/providers', '/dashboard/experimental',
  ]);
  assert.equal(searchNavigation('observe requests')[0]?.path, '/dashboard/analytics');
  assert.equal(searchNavigation('not-a-real-command').length, 0);
  assert.ok(searchNavigation('').every(item => dashboardRoutes.includes(item.path)));
});

test('landing metrics expose skeleton, verified counts, or nonnumeric proof without 0+', () => {
  assert.equal(deriveLandingMetrics({ status: 'loading' }).kind, 'loading');

  const verified = deriveLandingMetrics({ status: 'ready', providers: 12, models: 84 });
  assert.equal(verified.kind, 'verified');
  assert.deepEqual(verified.items.map(item => item.value), ['12', '84']);

  const fallback = deriveLandingMetrics({ status: 'error' });
  assert.equal(fallback.kind, 'fallback');
  assert.ok(fallback.items.every(item => !/^0\+?$/.test(item.value)));
});
