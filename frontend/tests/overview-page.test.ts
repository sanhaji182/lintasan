import { cleanup, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import OverviewPage from '../src/routes/dashboard/+page.svelte';

const mocks = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('$lib/api', () => ({ api: { get: mocks.get } }));

describe('Command Center overview', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/dashboard/stats') throw new Error('stats offline');
      if (path === '/api/logs') return { data: [{ model: 'deepseek-chat', provider: 'DeepSeek', status: 200, latency_ms: 120, input_tokens: 10, output_tokens: 5 }] };
      if (path === '/api/connections') return { data: [{ id: 'deepseek', name: 'DeepSeek', format: 'openai', is_active: 1 }] };
      return { data: [] };
    });
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ status: 'ok', version: 'test' }), { status: 200 })));
  });
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  it('keeps successful sources usable and labels failed stats instead of inventing zeroes', async () => {
    render(OverviewPage);
    expect(await screen.findByText('Operational')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('Traffic stats');
    expect(screen.getByText('15')).toBeInTheDocument();
    expect(screen.getByText('1 / 1')).toBeInTheDocument();
    expect(screen.getAllByText('Unavailable').length).toBeGreaterThan(0);
    expect(screen.queryByText('Tokens Today')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Manage connections/i })).toHaveAttribute('href', '/dashboard/connections');
  });
});
