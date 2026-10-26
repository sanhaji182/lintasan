import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import FallbackPage from '../src/routes/dashboard/fallback/+page.svelte';

const mocks = vi.hoisted(() => ({
  get: vi.fn(async (path: string): Promise<any> => {
    if (path === '/api/fallback') return { data: {
      model_chains: [{ id: 'm1', name: 'Model A', chain: ['model-a', 'model-b'], usage_count: 3 }],
      connection_chains: [{ id: 'c1', name: 'Conn A', chain: ['conn-a', 'conn-b'], usage_count: 1 }],
    } };
    if (path === '/api/connections') return { data: [
      { id: 'conn-a', name: 'Primary One', is_active: 1 },
      { id: 'conn-b', name: 'Backup One', is_active: 1 },
    ] };
    if (path === '/api/models/discovered') return { data: [
      { model_id: 'model-a', is_active: 1 },
      { model_id: 'model-b', is_active: 1 },
    ] };
    if (path === '/api/aliases') return { data: {} };
    if (path === '/api/combos') return { data: [] };
    return { data: [] };
  }),
  post: vi.fn(async (path: string, body: any) => ({ chain: { id: `${path}-id`, name: body.name, chain: body.chain, usage_count: 0 } })),
  del: vi.fn(async () => ({ status: 'deleted' })),
  toast: vi.fn(),
  confirm: vi.fn(() => true),
}));

vi.mock('$lib/api', () => ({ api: { get: mocks.get, post: mocks.post, delete: mocks.del } }));
vi.mock('$lib/toast', () => ({ showToast: mocks.toast }));
vi.mock('$lib/confirm', () => ({ confirmAction: mocks.confirm }));

async function renderPage() {
  render(FallbackPage);
  await screen.findByText('Model A');
  await screen.findByText('Primary One · conn-a');
}

describe('Fallback chain builder', () => {
  beforeEach(() => {
    for (const mock of [mocks.get, mocks.post, mocks.del, mocks.toast, mocks.confirm]) {
      mock.mockClear();
      mock.mockReset();
    }
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/fallback') return { data: {
        model_chains: [{ id: 'm1', name: 'Model A', chain: ['model-a', 'model-b'], usage_count: 3 }],
        connection_chains: [{ id: 'c1', name: 'Conn A', chain: ['conn-a', 'conn-b'], usage_count: 1 }],
      } };
      if (path === '/api/connections') return { data: [
        { id: 'conn-a', name: 'Primary One', is_active: 1 },
        { id: 'conn-b', name: 'Backup One', is_active: 1 },
      ] };
      if (path === '/api/models/discovered') return { data: [
        { model_id: 'model-a', is_active: 1 },
        { model_id: 'model-b', is_active: 1 },
      ] };
      if (path === '/api/aliases') return { data: {} };
      if (path === '/api/combos') return { data: [] };
      return { data: [] };
    });
    mocks.post.mockImplementation(async (path: string, body: any) => ({
      chain: { id: `${path}-id`, name: body.name, chain: body.chain, usage_count: 0 },
    }));
    mocks.del.mockResolvedValue({ status: 'deleted' });
    mocks.confirm.mockReturnValue(true);
    vi.stubGlobal('confirm', mocks.confirm);
  });
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  it('renders model and connection chains with human-readable connection labels', async () => {
    await renderPage();
    expect(screen.getByText('Model A')).toBeInTheDocument();
    expect(screen.getByText('model-a')).toBeInTheDocument();
    expect(screen.getByText('model-b')).toBeInTheDocument();
    expect(screen.getByText('Conn A')).toBeInTheDocument();
    expect(screen.getByText('Primary One · conn-a')).toBeInTheDocument();
    expect(screen.getByText('Backup One · conn-b')).toBeInTheDocument();
  });

  it('creates a model chain from ordered rows and trims the payload', async () => {
    await renderPage();
    await fireEvent.click(screen.getAllByRole('button', { name: /New Chain/i })[0]);
    await fireEvent.input(screen.getByLabelText('Chain name'), { target: { value: '  GPT fallback  ' } });
    await fireEvent.input(screen.getByLabelText('Primary model'), { target: { value: ' model-a ' } });
    await fireEvent.input(screen.getByLabelText('Fallback 1 model'), { target: { value: 'model-b' } });
    await fireEvent.click(screen.getByRole('button', { name: /Create Chain/i }));

    await waitFor(() => expect(mocks.post).toHaveBeenCalledWith('/api/fallback/model-chains', {
      name: 'GPT fallback',
      chain: ['model-a', 'model-b'],
    }));
    expect(screen.queryByText('Create Model Fallback Chain')).not.toBeInTheDocument();
    expect(screen.getByText('GPT fallback')).toBeInTheDocument();
    expect(mocks.toast).toHaveBeenCalledWith('Model fallback chain “GPT fallback” created', 'success');
  });

  it('rejects duplicate steps before calling the API', async () => {
    await renderPage();
    await fireEvent.click(screen.getAllByRole('button', { name: /New Chain/i })[0]);
    await fireEvent.input(screen.getByLabelText('Chain name'), { target: { value: 'dupe' } });
    await fireEvent.input(screen.getByLabelText('Primary model'), { target: { value: 'model-a' } });
    await fireEvent.input(screen.getByLabelText('Fallback 1 model'), { target: { value: 'model-a' } });
    await fireEvent.click(screen.getByRole('button', { name: /Create Chain/i }));

    expect(screen.getByRole('alert')).toHaveTextContent('Each step must be unique.');
    expect(mocks.post).not.toHaveBeenCalled();
  });

  it('creates connection chains with connection IDs from the picker', async () => {
    await renderPage();
    await fireEvent.click(screen.getAllByRole('button', { name: /New Chain/i })[1]);
    await fireEvent.input(screen.getByLabelText('Chain name'), { target: { value: 'Failover' } });
    await fireEvent.change(screen.getByLabelText('Primary connection'), { target: { value: 'conn-a' } });
    await fireEvent.change(screen.getByLabelText('Fallback 1 connection'), { target: { value: 'conn-b' } });
    await fireEvent.click(screen.getByRole('button', { name: /Create Chain/i }));

    await waitFor(() => expect(mocks.post).toHaveBeenCalledWith('/api/fallback/connection-chains', {
      name: 'Failover',
      chain: ['conn-a', 'conn-b'],
    }));
    expect(screen.getByText('Failover')).toBeInTheDocument();
  });

  it('confirms before deleting a chain', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /Delete model chain Model A/i }));
    await waitFor(() => expect(mocks.del).toHaveBeenCalledWith('/api/fallback/model-chains/m1'));
    expect(mocks.confirm).toHaveBeenCalledWith(expect.objectContaining({ title: expect.stringMatching(/Delete model chain/i) }));
    expect(screen.queryByText('Model A')).not.toBeInTheDocument();
  });
});
