import { cleanup, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import AnalyticsPage from '../src/routes/dashboard/analytics/+page.svelte';
import ObservabilityPage from '../src/routes/dashboard/observability/+page.svelte';

const mocks = vi.hoisted(() => ({ get: vi.fn(), raw: vi.fn() }));
const { get, raw } = mocks;
vi.mock('$lib/api', () => ({ api: mocks }));

class FakeEventSource {
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  addEventListener() {}
  close() {}
}

describe('analytics scope presentation', () => {
  beforeEach(() => {
    get.mockReset(); raw.mockReset();
    vi.stubGlobal('EventSource', FakeEventSource);
  });
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  it('keeps loading visible while both analytics sources are pending', () => {
    get.mockImplementation(() => new Promise(() => {}));
    render(AnalyticsPage);
    expect(screen.getByRole('status', { name: 'Loading' })).toBeInTheDocument();
  });

  it('labels every metric card with its contract-backed scope', async () => {
    get.mockImplementation(async (path: string) => path === '/api/dashboard/stats'
      ? { total_requests: 28, cache_hit_rate: 25, avg_latency: 120 }
      : { data: Array.from({ length: 20 }, (_, i) => ({ id: `${i}`, input_tokens: 1, output_tokens: 2, latency_ms: 100, cached: 0, status: 200 })) });
    render(AnalyticsPage);
    for (const [label, scope] of [
      ['Total Requests', 'All recorded requests'],
      ['Total Tokens', '20 retained request rows'],
      ['Cache Hit Rate', 'All recorded requests'],
      ['Avg Latency', 'All recorded requests'],
    ]) {
      const card = (await screen.findAllByText(label))[0].closest('.card')!;
      expect(within(card as HTMLElement).getByText(scope)).toBeInTheDocument();
    }
    expect(screen.getByText(/all-recorded counter exceeds the retained rows by 8/i)).toBeInTheDocument();
  });

  it('does not fabricate all-recorded reconciliation when dashboard stats fail', async () => {
    get.mockImplementation(async (path: string) => {
      if (path === '/api/dashboard/stats') throw new Error('stats offline');
      return { data: Array.from({ length: 20 }, (_, i) => ({ id: `${i}`, input_tokens: 1, output_tokens: 2, latency_ms: 100, cached: 0, status: 200 })) };
    });
    render(AnalyticsPage);

    expect(await screen.findByText(/Dashboard stats unavailable: stats offline/i)).toBeInTheDocument();
    const totalRequestsCard = screen.getByText('Total Requests').closest('.card')!;
    expect(within(totalRequestsCard as HTMLElement).getByText('20 retained request rows')).toBeInTheDocument();
    expect(screen.queryByText(/independently collected counts match/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/gateway’s all-recorded counter/i)).not.toBeInTheDocument();
  });

  it('shows a truthful missing-data state when both analytics sources fail', async () => {
    get.mockRejectedValue(new Error('offline'));
    render(AnalyticsPage);
    expect(await screen.findByText('Failed to load analytics')).toBeInTheDocument();
    expect(screen.getByText(/dashboard stats: offline/i)).toBeInTheDocument();
    expect(screen.getByText(/request logs: offline/i)).toBeInTheDocument();
  });
});

describe('observability source freshness', () => {
  beforeEach(() => { get.mockReset(); raw.mockReset(); });
  afterEach(() => { cleanup(); vi.useRealTimers(); });

  it('shows unknown source states while initial collections are pending', () => {
    get.mockImplementation(() => new Promise(() => {}));
    raw.mockImplementation(() => new Promise(() => {}));
    render(ObservabilityPage);
    expect(screen.getByText('Memory stats: freshness unknown')).toBeInTheDocument();
    expect(screen.getByText('Runtime metrics: freshness unknown')).toBeInTheDocument();
  });

  it('surfaces a partial metrics failure without hiding fresh memory collection', async () => {
    get.mockResolvedValue({ total_memories: 2, available: true, backend: 'sqlite' });
    raw.mockResolvedValue(new Response('nope', { status: 503 }));
    render(ObservabilityPage);
    expect(await screen.findByText(/Runtime metrics: freshness unknown · collection failed/i)).toBeInTheDocument();
    expect(screen.getByText(/Memory stats: fresh · collected/i)).toBeInTheDocument();
    expect(screen.getByText(/Some observability sources failed/i)).toBeInTheDocument();
  });

  it('marks previously collected data stale after a refresh failure', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-09-15T18:00:00Z'));
    get.mockResolvedValue({ total_memories: 2, available: true, backend: 'sqlite' });
    raw.mockResolvedValue(new Response('lintasan_process_goroutines 4', { status: 200 }));
    render(ObservabilityPage);
    await waitFor(() => expect(screen.getByText(/Runtime metrics: fresh · collected/i)).toBeInTheDocument());
    vi.setSystemTime(new Date('2026-09-15T18:00:31Z'));
    get.mockRejectedValue(new Error('memory down'));
    raw.mockRejectedValue(new Error('metrics down'));
    await vi.advanceTimersByTimeAsync(15_000);
    await waitFor(() => expect(screen.getByText(/Runtime metrics: stale · last collected/i)).toBeInTheDocument());
    expect(screen.getByText(/Memory stats: stale · last collected/i)).toBeInTheDocument();
  });
});