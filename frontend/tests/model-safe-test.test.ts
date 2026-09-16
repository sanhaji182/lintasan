import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ModelsPage from '../src/routes/dashboard/models/+page.svelte';
import { formatModelTestError, formatModelTestResponse } from '../src/lib/model-test-result';
import { buildCallableCatalog } from '../src/lib/workflow-consolidation';

const mocks = vi.hoisted(() => ({
  models: [{ id: 'demo-model', connection_id: 'conn-1', owned_by: 'Demo' }],
  connections: [{ id: 'conn-1', name: 'Demo Account', format: 'openai', is_active: 1 }],
  get: vi.fn(async (path: string) => {
    if (path === '/v1/models') return { data: mocks.models };
    if (path === '/api/connections') return { data: mocks.connections };
    if (path === '/api/aliases') return { data: {} };
    return { data: [] };
  }),
  post: vi.fn(),
  showToast: vi.fn(),
}));

vi.mock('$lib/api', () => ({ api: { get: mocks.get, post: mocks.post } }));
vi.mock('$lib/toast', () => ({ showToast: mocks.showToast }));

describe('model Safe test result formatting', () => {
  it.each([
    {
      name: 'auth error',
      input: { success: false, status: 'auth_error', http_status: 401, latency_ms: 42, message: 'Provider rejected credentials', body: '{"error":"invalid api key"}', hint: 'Update this connection credential.' },
      expected: { code: 'auth_error', httpStatus: 401, latencyMs: 42, message: 'Provider rejected credentials', detail: '{"error":"invalid api key"}', hint: 'Update this connection credential.' },
    },
    {
      name: 'rate limit',
      input: { success: false, status: 'rate_limited', http_status: 429, latency_ms: 87, message: 'Too many requests', body: 'Retry later' },
      expected: { code: 'rate_limited', httpStatus: 429, latencyMs: 87, message: 'Too many requests', detail: 'Retry later' },
    },
    {
      name: 'model missing',
      input: { success: false, status: 'model_not_found', http_status: 404, message: 'Model is not available', hint: 'Sync models for this connection.' },
      expected: { code: 'model_not_found', httpStatus: 404, message: 'Model is not available', hint: 'Sync models for this connection.' },
    },
  ])('preserves actionable $name fields', ({ input, expected }) => {
    expect(formatModelTestResponse(input)).toMatchObject({ ok: false, ...expected });
  });

  it('normalizes a successful result', () => {
    expect(formatModelTestResponse({ success: true, status: 'ok', http_status: 200, latency_ms: 31, message: 'Available' }))
      .toEqual({ ok: true, code: 'ok', httpStatus: 200, latencyMs: 31, message: 'Available' });
  });

  it.each([999, 99, 600, 200.5, '429'])('omits invalid HTTP status %s', (http_status) => {
    expect(formatModelTestResponse({ success: false, http_status, message: 'bad status' })).not.toHaveProperty('httpStatus');
  });

  it.each([-1, Number.NaN, Number.POSITIVE_INFINITY])('omits invalid latency %s', (latency_ms) => {
    expect(formatModelTestResponse({ success: false, latency_ms, message: 'bad latency' })).not.toHaveProperty('latencyMs');
  });

  it('rounds finite nonnegative latency to integer milliseconds', () => {
    expect(formatModelTestResponse({ success: false, latency_ms: 12.6, message: 'slow' })).toMatchObject({ latencyMs: 13 });
  });

  it('reads a non-2xx ApiError envelope', () => {
    const error = Object.assign(new Error('upstream refused'), {
      status: 502,
      detail: { code: 'upstream_error', message: 'upstream refused', body: '<b>Bad gateway</b>', hint: 'Check provider status.' },
      envelope: { success: false, latency_ms: 123, error: { code: 'upstream_error', message: 'upstream refused', body: '<b>Bad gateway</b>' }, hint: 'Check provider status.' },
    });
    expect(formatModelTestError(error)).toEqual({
      ok: false, code: 'upstream_error', httpStatus: 502, latencyMs: 123,
      message: 'upstream refused', detail: 'Bad gateway', hint: 'Check provider status.',
    });
  });

  it('uses a valid thrown HTTP status when envelope diagnostics are invalid', () => {
    const error = Object.assign(new Error('bad gateway'), {
      status: 502,
      envelope: { success: false, http_status: 600, latency_ms: Number.POSITIVE_INFINITY },
    });
    expect(formatModelTestError(error)).toEqual({ ok: false, code: 'test_failed', httpStatus: 502, message: 'bad gateway' });
  });

  it('sanitizes credentials, raw HTML, control characters, and truncates unsafe network bodies', () => {
    const unsafe = `<script>alert(1)</script> {"Authorization":"Bearer secret-token","api_key":"sk-supersecret"} ${'x'.repeat(500)}`;
    const result = formatModelTestResponse({ success: false, status: 'network_error', message: 'Network failed', body: unsafe });
    expect(result.detail).not.toMatch(/<script>|secret-token|sk-supersecret|Authorization/i);
    expect(result.detail).not.toMatch(/[\u0000-\u001F]/);
    expect(result.detail!.length).toBeLessThanOrEqual(243);
    expect(result.detail).toMatch(/…$/);
  });

  it('classifies a transport TypeError as a network error', () => {
    expect(formatModelTestError(new TypeError('Failed to fetch'))).toMatchObject({
      ok: false, code: 'network_error', message: 'Failed to fetch',
    });
  });
});

describe('Models Safe test UI', () => {
  beforeEach(() => {
    mocks.models = [{ id: 'demo-model', connection_id: 'conn-1', owned_by: 'Demo' }];
    mocks.connections = [{ id: 'conn-1', name: 'Demo Account', format: 'openai', is_active: 1 }];
    mocks.get.mockClear(); mocks.post.mockReset(); mocks.showToast.mockClear();
  });
  afterEach(() => cleanup());

  async function renderAndTest() {
    render(ModelsPage);
    const button = await screen.findByRole('button', { name: 'Safe test' });
    await fireEvent.click(button);
  }

  it('renders and toasts actionable HTTP-200 success:false diagnostics', async () => {
    mocks.post.mockResolvedValue({ success: false, status: 'rate_limited', http_status: 429, latency_ms: 87.6, message: 'Too many requests', body: '<b>Retry later</b>', hint: 'Wait before retrying.' });
    await renderAndTest();
    expect(await screen.findByText('Too many requests')).not.toBeNull();
    expect(screen.getByText('rate_limited')).not.toBeNull();
    expect(screen.getByText('HTTP 429')).not.toBeNull();
    expect(screen.getByText('88 ms')).not.toBeNull();
    expect(screen.getByText('Retry later')).not.toBeNull();
    expect(screen.getByText('Wait before retrying.')).not.toBeNull();
    expect(mocks.showToast).toHaveBeenCalledWith('Safe test failed: Too many requests', 'error', 6000, expect.objectContaining({ code: 'rate_limited', httpStatus: 429, latencyMs: 88, hint: 'Wait before retrying.' }));
  });

  it('renders a non-2xx ApiError envelope without exposing raw HTML', async () => {
    mocks.post.mockRejectedValue(Object.assign(new Error('upstream refused'), {
      status: 502,
      detail: { code: 'upstream_error', message: 'upstream refused', body: '<script>steal()</script><b>Bad gateway</b>' },
      envelope: { success: false, latency_ms: 123.6, error: { code: 'upstream_error', message: 'upstream refused', body: '<script>steal()</script><b>Bad gateway</b>' }, hint: 'Check provider status.' },
    }));
    await renderAndTest();
    expect(await screen.findByText('upstream refused')).not.toBeNull();
    expect(screen.getByText('Bad gateway')).not.toBeNull();
    expect(screen.getByText('HTTP 502')).not.toBeNull();
    expect(screen.getByText('124 ms')).not.toBeNull();
    expect(document.body.textContent).not.toContain('<script>');
    expect(mocks.showToast).toHaveBeenCalledWith('Safe test failed: upstream refused', 'error', 6000, expect.objectContaining({ code: 'upstream_error', httpStatus: 502, latencyMs: 124 }));
  });

  it('omits invalid numeric diagnostics from both card and toast', async () => {
    mocks.post.mockResolvedValue({ success: false, status: 'test_failed', http_status: 999, latency_ms: -1.6, message: 'Invalid diagnostics' });
    await renderAndTest();
    expect(await screen.findByText('Invalid diagnostics')).not.toBeNull();
    expect(document.body.textContent).not.toContain('HTTP 999');
    expect(document.body.textContent).not.toContain('-1.6 ms');
    const detail = mocks.showToast.mock.calls[0][3];
    expect(detail.httpStatus).toBeUndefined();
    expect(detail.latencyMs).toBeUndefined();
  });

  it('renders success without an error toast', async () => {
    mocks.post.mockResolvedValue({ success: true, status: 'ok', http_status: 200, latency_ms: 31, message: 'Available' });
    await renderAndTest();
    await waitFor(() => expect(screen.getByText('Available')).not.toBeNull());
    expect(screen.getByText('HTTP 200')).not.toBeNull();
    expect(screen.getByText('31 ms')).not.toBeNull();
    expect(mocks.showToast).not.toHaveBeenCalled();
  });

  it('keeps duplicate model IDs isolated per account while deduping true connection duplicates', async () => {
    mocks.models = [
      { id: 'shared-model', connection_id: 'conn-1', owned_by: 'Demo' },
      { id: 'shared-model', connection_id: 'conn-1', owned_by: 'Demo' },
      { id: 'shared-model', connection_id: 'conn-2', owned_by: 'Demo' },
    ];
    mocks.connections = [
      { id: 'conn-1', name: 'Account One', format: 'openai', is_active: 1 },
      { id: 'conn-2', name: 'Account Two', format: 'openai', is_active: 1 },
    ];
    const pending: Array<(value: any) => void> = [];
    mocks.post.mockImplementation(() => new Promise(resolve => pending.push(resolve)));

    render(ModelsPage);
    expect(await screen.findByText('Account One')).not.toBeNull();
    expect(screen.getByText('Account Two')).not.toBeNull();
    expect(screen.getAllByText('shared-model')).toHaveLength(2);

    const buttons = screen.getAllByRole('button', { name: 'Safe test' });
    await fireEvent.click(buttons[0]);
    await fireEvent.click(buttons[1]);
    expect(mocks.post).toHaveBeenNthCalledWith(1, '/api/models/test', { model_id: 'shared-model', connection_id: 'conn-1' });
    expect(mocks.post).toHaveBeenNthCalledWith(2, '/api/models/test', { model_id: 'shared-model', connection_id: 'conn-2' });
    expect(screen.getAllByRole('button', { name: 'Testing…' })).toHaveLength(2);

    pending[0]({ success: true, status: 'ok', latency_ms: 11, message: 'Account one works' });
    await waitFor(() => expect(screen.getByText('Account one works')).not.toBeNull());
    expect(screen.getByRole('button', { name: 'Safe test' })).not.toBeNull();
    expect(screen.getByRole('button', { name: 'Testing…' })).not.toBeNull();

    pending[1]({ success: false, status: 'rate_limited', http_status: 429, latency_ms: 22, message: 'Account two limited' });
    await waitFor(() => expect(screen.getByText('Account two limited')).not.toBeNull());
    expect(screen.getByText('Account one works')).not.toBeNull();
  });
});

describe('callable catalog row identity', () => {
  it('keys by kind, account identity, and model while retaining cross-account rows', () => {
    const rows = buildCallableCatalog({
      connections: [{ id: 'a', name: 'One' }, { id: 'b', name: 'Two' }],
      models: [
        { id: 'same', connection_id: 'a' },
        { id: 'same', connection_id: 'a' },
        { id: 'same', connection_id: 'b' },
      ],
    });
    expect(rows).toHaveLength(2);
    expect(new Set(rows.map(row => row.rowKey)).size).toBe(2);
  });

  it('preserves authoritative cloud-account identity without connection_id', () => {
    const rows = buildCallableCatalog({
      models: [
        { id: 'hoplite-agent/shared', provider_kind: 'cloud_agent', hoplite_account_id: 'hoplite-cloud-agent', owned_by: 'Hoplite Agent' },
        { id: 'hoplite-agent/shared', provider_kind: 'cloud_agent', hoplite_account_id: 'hoplite-cloud-agent', owned_by: 'Hoplite Agent' },
        { id: 'hoplite-agent/shared', provider_kind: 'cloud_agent', hoplite_account_id: 'hoplite-cloud-agent-secondary', owned_by: 'Hoplite Agent' },
      ],
    });
    expect(rows).toHaveLength(2);
    expect(rows.map(row => row.connectionId)).toEqual(['hoplite-cloud-agent', 'hoplite-cloud-agent-secondary']);
    expect(rows.map(row => row.account)).toEqual(['hoplite-cloud-agent', 'hoplite-cloud-agent-secondary']);
    expect(new Set(rows.map(row => row.rowKey)).size).toBe(2);
  });

  it('keeps route, cloud-agent, and provider identities separate for the same model ID', () => {
    const rows = buildCallableCatalog({
      aliases: { same: 'target' },
      connections: [{ id: 'cloud-account', name: 'Cloud', provider_kind: 'cloud_agent' }],
      models: [
        { id: 'same', provider_kind: 'cloud_agent', hoplite_account_id: 'cloud-account' },
        { id: 'same', connection_id: 'provider-account', owned_by: 'Provider' },
      ],
    });
    expect(rows).toHaveLength(3);
    expect(new Set(rows.map(row => row.rowKey)).size).toBe(3);
    expect(rows.map(row => row.kind).sort()).toEqual(['cloud_agent', 'provider', 'route']);
  });
});
