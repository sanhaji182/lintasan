import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ObservabilityPage from '../src/routes/dashboard/observability/+page.svelte';

const mocks = vi.hoisted(() => ({ get: vi.fn(), raw: vi.fn() }));
const { get, raw } = mocks;
vi.mock('$lib/api', () => ({ api: mocks }));

const memory = (collectedAt?: string) => ({
  total_memories: 2,
  available: true,
  backend: 'sqlite',
  search: { calls: 1, hits: 1, empty_exits: 0, rows_scanned: 2, capped_scans: 0, max_scan_rows: 100 },
  ...(collectedAt ? { collected_at: collectedAt } : {}),
});
const metrics = (timestamp?: number) => new Response(
  `lintasan_process_goroutines 4${timestamp == null ? '' : ` ${timestamp}`}`,
  { status: 200 },
);

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

describe('observability source state contracts', () => {
  beforeEach(() => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-09-15T18:00:31Z'));
    get.mockReset();
    raw.mockReset();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('labels both sources loading while initial collections are pending', () => {
    get.mockImplementation(() => new Promise(() => {}));
    raw.mockImplementation(() => new Promise(() => {}));

    render(ObservabilityPage);

    expect(screen.getByText('Memory stats: loading')).toBeInTheDocument();
    expect(screen.getByText('Runtime metrics: loading')).toBeInTheDocument();
  });

  it('does not turn client retrieval time into source freshness', async () => {
    get.mockResolvedValue(memory());
    raw.mockResolvedValue(metrics());

    render(ObservabilityPage);

    expect(await screen.findByText('Memory stats: available · freshness unknown')).toBeInTheDocument();
    expect(await screen.findByText('Runtime metrics: available · freshness unknown')).toBeInTheDocument();
    expect(screen.queryByText(/collected just now/i)).not.toBeInTheDocument();
  });

  it('marks genuinely timestamped source data stale at the refresh-based threshold', async () => {
    get.mockResolvedValue(memory('2026-09-15T18:00:00Z'));
    raw.mockResolvedValue(metrics(Date.parse('2026-09-15T18:00:00Z')));

    render(ObservabilityPage);

    expect(await screen.findByText('Memory stats: stale · source updated 31s ago')).toBeInTheDocument();
    expect(await screen.findByText('Runtime metrics: stale · source updated 31s ago')).toBeInTheDocument();
  });

  it('marks genuinely timestamped source data fresh within two refresh intervals', async () => {
    get.mockResolvedValue(memory('2026-09-15T18:00:21Z'));
    raw.mockResolvedValue(metrics(Date.parse('2026-09-15T18:00:21Z')));

    render(ObservabilityPage);

    expect(await screen.findByText('Memory stats: fresh · source updated 10s ago')).toBeInTheDocument();
    expect(await screen.findByText('Runtime metrics: fresh · source updated 10s ago')).toBeInTheDocument();
  });

  it('shows metrics failure as partial while retaining successful memory data', async () => {
    get.mockResolvedValue(memory());
    raw.mockResolvedValue(new Response('offline', { status: 503 }));

    render(ObservabilityPage);

    expect(await screen.findByText('Memory stats: available · freshness unknown')).toBeInTheDocument();
    expect(await screen.findByText('Runtime metrics: error · no data')).toBeInTheDocument();
    expect(await screen.findByText(/Partial observability data: runtime metrics failed/i)).toBeInTheDocument();
    expect(screen.queryByText(/all sources failed/i)).not.toBeInTheDocument();
  });

  it('shows memory failure as partial while retaining successful metrics data', async () => {
    get.mockRejectedValue(new Error('memory offline'));
    raw.mockResolvedValue(metrics());

    render(ObservabilityPage);

    expect(await screen.findByText('Memory stats: error · no data')).toBeInTheDocument();
    expect(await screen.findByText('Runtime metrics: available · freshness unknown')).toBeInTheDocument();
    expect(await screen.findByText(/Partial observability data: memory stats failed/i)).toBeInTheDocument();
  });

  it('distinguishes successful responses with missing payload fields', async () => {
    get.mockResolvedValue({});
    raw.mockResolvedValue(new Response('', { status: 200 }));

    render(ObservabilityPage);

    expect(await screen.findByText('Memory stats: missing · required payload fields absent')).toBeInTheDocument();
    expect(await screen.findByText('Runtime metrics: missing · required payload fields absent')).toBeInTheDocument();
    expect(await screen.findByText(/Observability data is missing from both sources/i)).toBeInTheDocument();
  });

  it('labels each source loading during manual refresh while retaining rendered data', async () => {
    get.mockResolvedValueOnce(memory());
    raw.mockResolvedValueOnce(metrics());
    const { container } = render(ObservabilityPage);
    await screen.findByText('Memory stats: available · freshness unknown');
    await screen.findByText('Runtime metrics: available · freshness unknown');

    const nextMemory = deferred<ReturnType<typeof memory>>();
    const nextMetrics = deferred<Response>();
    get.mockImplementationOnce(() => nextMemory.promise);
    raw.mockImplementationOnce(() => nextMetrics.promise);

    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));

    expect(screen.getByText('Memory stats: loading · showing previous data')).toBeInTheDocument();
    expect(await screen.findByText('Runtime metrics: loading · showing previous data')).toBeInTheDocument();
    expect(container.textContent).toContain('2 memories stored');

    nextMemory.resolve(memory());
    nextMetrics.resolve(metrics());
    await waitFor(() => expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled());
  });

  it('shows a distinct total error when neither source has usable data', async () => {
    get.mockRejectedValue(new Error('memory offline'));
    raw.mockRejectedValue(new Error('metrics offline'));

    render(ObservabilityPage);

    expect(await screen.findByText('Observability unavailable: all sources failed.')).toBeInTheDocument();
    expect(screen.queryByText(/Partial observability data/i)).not.toBeInTheDocument();
  });
});
