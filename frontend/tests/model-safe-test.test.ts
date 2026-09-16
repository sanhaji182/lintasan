import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ModelsPage from '../src/routes/dashboard/models/+page.svelte';
import { formatModelTestError, formatModelTestResponse } from '../src/lib/model-test-result';

const mocks = vi.hoisted(() => ({
  get: vi.fn(async (path: string) => {
    if (path === '/v1/models') return { data: [{ id: 'demo-model', connection_id: 'conn-1', owned_by: 'Demo' }] };
    if (path === '/api/connections') return { data: [{ id: 'conn-1', name: 'Demo Account', format: 'openai', is_active: 1 }] };
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
  beforeEach(() => { mocks.get.mockClear(); mocks.post.mockReset(); mocks.showToast.mockClear(); });
  afterEach(() => cleanup());

  async function renderAndTest() {
    render(ModelsPage);
    const button = await screen.findByRole('button', { name: 'Safe test' });
    await fireEvent.click(button);
  }

  it('renders and toasts actionable HTTP-200 success:false diagnostics', async () => {
    mocks.post.mockResolvedValue({ success: false, status: 'rate_limited', http_status: 429, latency_ms: 87, message: 'Too many requests', body: '<b>Retry later</b>', hint: 'Wait before retrying.' });
    await renderAndTest();
    expect(await screen.findByText('Too many requests')).not.toBeNull();
    expect(screen.getByText('rate_limited')).not.toBeNull();
    expect(screen.getByText('HTTP 429')).not.toBeNull();
    expect(screen.getByText('87 ms')).not.toBeNull();
    expect(screen.getByText('Retry later')).not.toBeNull();
    expect(screen.getByText('Wait before retrying.')).not.toBeNull();
    expect(mocks.showToast).toHaveBeenCalledWith('Safe test failed: Too many requests', 'error', 6000, expect.objectContaining({ code: 'rate_limited', hint: 'Wait before retrying.' }));
  });

  it('renders a non-2xx ApiError envelope without exposing raw HTML', async () => {
    mocks.post.mockRejectedValue(Object.assign(new Error('upstream refused'), {
      status: 502,
      detail: { code: 'upstream_error', message: 'upstream refused', body: '<script>steal()</script><b>Bad gateway</b>' },
      envelope: { success: false, latency_ms: 123, error: { code: 'upstream_error', message: 'upstream refused', body: '<script>steal()</script><b>Bad gateway</b>' }, hint: 'Check provider status.' },
    }));
    await renderAndTest();
    expect(await screen.findByText('upstream refused')).not.toBeNull();
    expect(screen.getByText('Bad gateway')).not.toBeNull();
    expect(document.body.textContent).not.toContain('<script>');
    expect(mocks.showToast).toHaveBeenCalledWith('Safe test failed: upstream refused', 'error', 6000, expect.objectContaining({ code: 'upstream_error' }));
  });

  it('renders success without an error toast', async () => {
    mocks.post.mockResolvedValue({ success: true, status: 'ok', http_status: 200, latency_ms: 31, message: 'Available' });
    await renderAndTest();
    await waitFor(() => expect(screen.getByText('Available')).not.toBeNull());
    expect(screen.getByText('HTTP 200')).not.toBeNull();
    expect(screen.getByText('31 ms')).not.toBeNull();
    expect(mocks.showToast).not.toHaveBeenCalled();
  });
});
