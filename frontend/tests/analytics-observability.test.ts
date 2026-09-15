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

  it.each([
    ['/api/dashboard/stats', '/api/logs'],
    ['/api/logs', '/api/dashboard/stats'],
  ])('keeps loading visible without stale cards while %s is pending and %s has settled', async (pendingPath, settledPath) => {
    get.mockImplementation((path: string) => path === pendingPath
      ? new Promise(() => {})
      : Promise.resolve(settledPath === '/api/dashboard/stats'
        ? { total_requests: 28, cache_hit_rate: 25, avg_latency: 120 }
        : { data: [{ id: '1', input_tokens: 1, output_tokens: 2, latency_ms: 100, cached: 0, status: 200 }] }));
    render(AnalyticsPage);
    expect(screen.getByRole('status', { name: 'Loading' })).toBeInTheDocument();
    expect(screen.queryByText('Total Requests')).not.toBeInTheDocument();
    expect(screen.queryByText(/retained request rows|all recorded requests/i)).not.toBeInTheDocument();
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

  it('keeps stats-backed cards unavailable instead of changing their scope when dashboard stats fail', async () => {
    get.mockImplementation(async (path: string) => {
      if (path === '/api/dashboard/stats') throw new Error('stats offline');
      return { data: Array.from({ length: 20 }, (_, i) => ({ id: `${i}`, input_tokens: 1, output_tokens: 2, latency_ms: 100, cached: 0, status: 200 })) };
    });
    render(AnalyticsPage);

    expect(await screen.findByText(/Dashboard stats unavailable: stats offline/i)).toBeInTheDocument();
    for (const label of ['Total Requests', 'Cache Hit Rate', 'Avg Latency']) {
      const card = screen.getAllByText(label)[0].closest('.card')!;
      expect(within(card as HTMLElement).getByText('All-recorded statistics unavailable')).toBeInTheDocument();
      expect(within(card as HTMLElement).getByText('—')).toBeInTheDocument();
      expect(within(card as HTMLElement).queryByText(/retained request rows/i)).not.toBeInTheDocument();
    }
    const totalTokensCard = screen.getByText('Total Tokens').closest('.card')!;
    expect(within(totalTokensCard as HTMLElement).getByText('20 retained request rows')).toBeInTheDocument();
    expect(within(totalTokensCard as HTMLElement).getByText('60')).toBeInTheDocument();
    expect(screen.queryByText(/independently collected counts match/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/gateway’s all-recorded counter/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/filter(?:ed|ing)?|caused by|due to/i)).not.toBeInTheDocument();
  });

  it('keeps retained-row token scope unavailable when request logs fail', async () => {
    get.mockImplementation(async (path: string) => {
      if (path === '/api/logs') throw new Error('logs offline');
      return { total_requests: 28, cache_hit_rate: 25, avg_latency: 120 };
    });
    render(AnalyticsPage);

    expect(await screen.findByText(/Request logs unavailable: logs offline/i)).toBeInTheDocument();
    for (const label of ['Total Requests', 'Cache Hit Rate', 'Avg Latency']) {
      const card = screen.getAllByText(label)[0].closest('.card')!;
      expect(within(card as HTMLElement).getByText('All recorded requests')).toBeInTheDocument();
    }
    const totalTokensCard = screen.getByText('Total Tokens').closest('.card')!;
    expect(within(totalTokensCard as HTMLElement).getByText('Retained request rows unavailable')).toBeInTheDocument();
    expect(within(totalTokensCard as HTMLElement).getByText('—')).toBeInTheDocument();
    expect(within(totalTokensCard as HTMLElement).queryByText(/\d+ retained request rows/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/filter(?:ed|ing)?|caused by|due to/i)).not.toBeInTheDocument();
  });

  it('keeps an empty retained snapshot truthful when dashboard stats fail', async () => {
    get.mockImplementation(async (path: string) => {
      if (path === '/api/dashboard/stats') throw new Error('stats offline');
      return { data: [] };
    });
    render(AnalyticsPage);

    expect(await screen.findByText(/Dashboard stats unavailable: stats offline/i)).toBeInTheDocument();
    const totalRequestsCard = screen.getByText('Total Requests').closest('.card')!;
    expect(within(totalRequestsCard as HTMLElement).getByText('All-recorded statistics unavailable')).toBeInTheDocument();
    expect(within(totalRequestsCard as HTMLElement).getByText('—')).toBeInTheDocument();
    expect(within(totalRequestsCard as HTMLElement).queryByText(/retained request rows/i)).not.toBeInTheDocument();
    const totalTokensCard = screen.getByText('Total Tokens').closest('.card')!;
    expect(within(totalTokensCard as HTMLElement).getByText('0 retained request rows')).toBeInTheDocument();
    expect(within(totalTokensCard as HTMLElement).getByText('0')).toBeInTheDocument();
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

  it('still shows the generic empty state when both sources succeed with no rows', async () => {
    get.mockImplementation(async (path: string) => path === '/api/dashboard/stats' ? null : { data: [] });
    render(AnalyticsPage);
    expect(await screen.findByText('No analytics data')).toBeInTheDocument();
    expect(screen.queryByText(/Partial data:/i)).not.toBeInTheDocument();
  });

  it('shows zero-valued all-recorded scope instead of a partial warning when both sources succeed empty', async () => {
    get.mockImplementation(async (path: string) => path === '/api/dashboard/stats'
      ? { total_requests: 0, cache_hit_rate: 0, avg_latency: 0 }
      : { data: [] });
    render(AnalyticsPage);
    const totalRequestsCard = (await screen.findAllByText('Total Requests'))[0].closest('.card')!;
    expect(within(totalRequestsCard as HTMLElement).getByText('All recorded requests')).toBeInTheDocument();
    expect(screen.queryByText(/Partial data:/i)).not.toBeInTheDocument();
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
