import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import RoutingPage from '../src/routes/dashboard/routing/+page.svelte';

const mocks = vi.hoisted(() => ({
  beforeNavigateHandler: undefined as undefined | ((navigation: { cancel: () => void; willUnload?: boolean }) => void),
  get: vi.fn(async (path: string): Promise<any> => {
    if (path === '/api/combos') return { data: [{ id: 'primary', name: 'Primary', strategy: 'priority', entries: [], order: 0 }] };
    if (path === '/api/aliases') return { data: { automatic: { model: 'target-model' } } };
    if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
    if (path === '/api/smart-routing') return { data: {
      ml_router_enabled: false,
      ml_router_cheap_model: 'gpt-4o-mini',
      ml_router_expensive_model: 'gpt-4o',
      ml_router_threshold: '0.5',
      cost_quality_floor: '0.3',
      cost_expensive_anchor: '0.02',
      quota_limits: {},
    } };
    if (path === '/v1/models') return { data: [{ id: 'target-model' }] };
    return { data: [] };
  }),
  post: vi.fn(async (path: string, body: unknown) => path === '/api/routing/aliases'
    ? { alias: { id: (body as any).alias, alias: (body as any).alias, target: (body as any).target } }
    : {}),
  patch: vi.fn(async () => ({})),
  put: vi.fn(async () => ({})),
  del: vi.fn(async () => ({})),
  toast: vi.fn(),
  confirm: vi.fn(() => true),
}));

vi.mock('$app/navigation', () => ({
  beforeNavigate: (handler: typeof mocks.beforeNavigateHandler) => { mocks.beforeNavigateHandler = handler; },
}));
vi.mock('$lib/api', () => ({ api: { get: mocks.get, post: mocks.post, patch: mocks.patch, put: mocks.put, delete: mocks.del } }));
vi.mock('$lib/toast', () => ({ showToast: mocks.toast }));

async function renderPage() {
  render(RoutingPage);
  await screen.findByText(/Current strategy:/i);
}

async function makePolicyDirty() {
  await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
  await fireEvent.change(screen.getByRole('combobox'), { target: { value: 'round-robin' } });
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('1 unsaved scope'));
}

function navigate() {
  const cancel = vi.fn();
  mocks.beforeNavigateHandler?.({ cancel, willUnload: false });
  return cancel;
}

describe('Routing save boundaries', () => {
  beforeEach(() => {
    mocks.beforeNavigateHandler = undefined;
    for (const mock of [mocks.get, mocks.post, mocks.patch, mocks.put, mocks.del, mocks.toast, mocks.confirm]) {
      mock.mockClear();
      mock.mockReset();
    }
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/combos') return { data: [{ id: 'primary', name: 'Primary', strategy: 'priority', entries: [], order: 0 }] };
      if (path === '/api/aliases') return { data: { automatic: { model: 'target-model' } } };
      if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
      if (path === '/api/smart-routing') return { data: {
        ml_router_enabled: false,
        ml_router_cheap_model: 'gpt-4o-mini',
        ml_router_expensive_model: 'gpt-4o',
        ml_router_threshold: '0.5',
        cost_quality_floor: '0.3',
        cost_expensive_anchor: '0.02',
        quota_limits: {},
      } };
      if (path === '/v1/models') return { data: [{ id: 'target-model' }] };
      if (path === '/api/connections') return { data: [
        { id: 'qoder-a', name: 'Qoder Alice', format: 'qoder', base_url: 'https://api.qoder.com', chat_path: '/chat/completions', is_active: 1 },
        { id: 'qoder-b', name: 'Qoder Bob', format: 'qoder', base_url: 'https://api.qoder.com/', chat_path: '/chat/completions', is_active: 1 },
        { id: 'other', name: 'Other', format: 'openai', base_url: 'https://other.example/v1', chat_path: '/v1/chat/completions', is_active: 1 },
      ] };
      if (path === '/api/presets') return { data: [
        { name: 'Qoder', domain: 'qoder.com', base_url: 'https://api.qoder.com/v1' },
      ] };
      if (path === '/api/models/discovered') return { data: [
        { model_id: 'qoder-only-a', connection_id: 'qoder-a', is_active: 1 },
        { model_id: 'qoder-only-b', connection_id: 'qoder-b', is_active: 1 },
        { model_id: 'shared-model', connection_id: 'qoder-a', is_active: 1 },
        { model_id: 'shared-model', connection_id: 'other', is_active: 1 },
      ] };
      return { data: [] };
    });
    mocks.post.mockImplementation(async (path: string, body: unknown) => path === '/api/routing/aliases'
      ? { alias: { id: (body as any).alias, alias: (body as any).alias, target: (body as any).target } }
      : {});
    mocks.patch.mockResolvedValue({});
    mocks.put.mockResolvedValue({});
    mocks.del.mockResolvedValue({});
    mocks.confirm.mockReturnValue(true);
    vi.stubGlobal('confirm', mocks.confirm);
  });
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  it('does not warn or cancel SPA navigation while the page is clean', async () => {
    await renderPage();
    const cancel = navigate();
    expect(mocks.confirm).not.toHaveBeenCalled();
    expect(cancel).not.toHaveBeenCalled();
  });

  it('asks before SPA navigation and cancels when dirty changes are rejected', async () => {
    mocks.confirm.mockReturnValue(false);
    await renderPage();
    await makePolicyDirty();
    const cancel = navigate();
    expect(mocks.confirm).toHaveBeenCalledWith(expect.stringMatching(/unsaved routing changes/i));
    expect(mocks.confirm).toHaveBeenCalledTimes(1);
    expect(cancel).toHaveBeenCalledOnce();
  });

  it('allows SPA navigation when dirty changes are confirmed for discard', async () => {
    await renderPage();
    await makePolicyDirty();
    const cancel = navigate();
    expect(mocks.confirm).toHaveBeenCalledWith(expect.stringMatching(/unsaved routing changes/i));
    expect(cancel).not.toHaveBeenCalled();
  });

  it('leaves full-page navigation to the native unload guard without a duplicate confirm', async () => {
    await renderPage();
    await makePolicyDirty();
    const cancel = vi.fn();
    mocks.beforeNavigateHandler?.({ cancel, willUnload: true });
    expect(mocks.confirm).not.toHaveBeenCalled();
    expect(cancel).not.toHaveBeenCalled();
  });

  it('protects browser refresh exactly once while dirty and removes the guard on unmount', async () => {
    const addSpy = vi.spyOn(window, 'addEventListener');
    const removeSpy = vi.spyOn(window, 'removeEventListener');
    const view = render(RoutingPage);
    await screen.findByText(/Current strategy:/i);
    const unloadAdds = addSpy.mock.calls.filter(([type]) => type === 'beforeunload');
    expect(unloadAdds).toHaveLength(1);

    const clean = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(clean);
    expect(clean.defaultPrevented).toBe(false);

    await makePolicyDirty();
    const dirty = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(dirty);
    expect(dirty.defaultPrevented).toBe(true);

    view.unmount();
    expect(removeSpy).toHaveBeenCalledWith('beforeunload', unloadAdds[0][1]);
    const afterUnmount = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(afterUnmount);
    expect(afterUnmount.defaultPrevented).toBe(false);
    addSpy.mockRestore();
    removeSpy.mockRestore();
  });

  it('stops warning immediately after a successful scoped save', async () => {
    await renderPage();
    await makePolicyDirty();
    await fireEvent.click(screen.getByRole('button', { name: /Save Combos/i }));
    await waitFor(() => expect(screen.queryByText(/unsaved scope/i)).not.toBeInTheDocument());

    const cancel = navigate();
    expect(mocks.confirm).not.toHaveBeenCalled();
    expect(cancel).not.toHaveBeenCalled();
    const refresh = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(refresh);
    expect(refresh.defaultPrevented).toBe(false);
  });

  it('keeps failed scoped saves dirty and protected', async () => {
    mocks.patch.mockRejectedValueOnce(new Error('save rejected'));
    await renderPage();
    await makePolicyDirty();
    await fireEvent.click(screen.getByRole('button', { name: /Save Combos/i }));
    await waitFor(() => expect(mocks.toast).toHaveBeenCalledWith('Combo save stopped; unsaved changes remain', 'error'));
    expect(screen.getByRole('status')).toHaveTextContent('1 unsaved scope');

    mocks.confirm.mockReturnValue(false);
    const cancel = navigate();
    expect(cancel).toHaveBeenCalledOnce();
    const refresh = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(refresh);
    expect(refresh.defaultPrevented).toBe(true);
  });

  it('stops warning immediately after discarding staged changes', async () => {
    await renderPage();
    await makePolicyDirty();
    await fireEvent.click(screen.getByRole('button', { name: /Discard strategy edits/i }));
    await waitFor(() => expect(screen.queryByText(/unsaved scope/i)).not.toBeInTheDocument());

    const cancel = navigate();
    expect(mocks.confirm).not.toHaveBeenCalled();
    expect(cancel).not.toHaveBeenCalled();
    const refresh = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(refresh);
    expect(refresh.defaultPrevented).toBe(false);
  });

  it('discards only Policies without losing unsaved Quota edits', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Quotas/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Add limit/i }));
    await fireEvent.input(screen.getByPlaceholderText(/connection id/i), { target: { value: 'quota-connection' } });
    await fireEvent.input(screen.getByPlaceholderText(/max tokens \/ day/i), { target: { value: '42000' } });
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('1 unsaved scope'));

    await fireEvent.click(screen.getByRole('button', { name: /^Policies/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Round Robin/i }));
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('2 unsaved scopes'));
    await fireEvent.click(screen.getByRole('button', { name: /^Discard$/i }));

    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('1 unsaved scope'));
    expect(screen.getByRole('status')).toHaveTextContent('Quotas');
    await fireEvent.click(screen.getByRole('button', { name: /^Quotas/i }));
    expect(screen.getByDisplayValue('quota-connection')).toBeInTheDocument();
    expect(screen.getByDisplayValue('42000')).toBeInTheDocument();
  });

  it('states that alias create and delete apply immediately outside staged Combo saves', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    expect(screen.getByText(/Creating or deleting an alias applies immediately/i)).toBeInTheDocument();
    expect(screen.getByText(/does not use Save Combos/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Delete alias auto$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Delete alias auto\/coding$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Delete alias auto\/fast$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Delete alias auto\/cheap$/i })).not.toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: /Add Alias/i }));
    await fireEvent.input(screen.getByPlaceholderText(/Alias name/i), { target: { value: 'friendly' } });
    await fireEvent.input(screen.getByPlaceholderText(/Target model/i), { target: { value: 'target-model' } });
    await fireEvent.click(screen.getByRole('button', { name: /Create alias/i }));

    await waitFor(() => expect(mocks.post).toHaveBeenCalledWith('/api/routing/aliases', { alias: 'friendly', target: 'target-model' }));
    expect(mocks.put).not.toHaveBeenCalled();
    expect(screen.queryByText(/unsaved scope/i)).not.toBeInTheDocument();
    expect(mocks.toast).toHaveBeenCalledWith('Alias “friendly” created and applied immediately', 'success');
    expect(screen.getByRole('status')).toHaveTextContent('Alias “friendly” created and applied immediately');

    await fireEvent.click(screen.getByRole('button', { name: /Delete alias friendly/i }));
    await waitFor(() => expect(mocks.del).toHaveBeenCalledWith('/api/routing/aliases/friendly'));
    expect(mocks.put).not.toHaveBeenCalled();
    expect(screen.queryByText(/unsaved scope/i)).not.toBeInTheDocument();
    expect(mocks.toast).toHaveBeenCalledWith('Alias “friendly” deleted immediately', 'success');
    expect(screen.getByRole('status')).toHaveTextContent('Alias “friendly” deleted immediately');
  });

  it('upserts an existing alias in place without rendering duplicate keyed rows', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    expect(screen.getByText('target-model')).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: /Add Alias/i }));
    await fireEvent.input(screen.getByPlaceholderText(/Alias name/i), { target: { value: 'automatic' } });
    await fireEvent.input(screen.getByPlaceholderText(/Target model/i), { target: { value: 'replacement-model' } });
    await fireEvent.click(screen.getByRole('button', { name: /Create alias/i }));

    await waitFor(() => expect(mocks.post).toHaveBeenCalledWith('/api/routing/aliases', {
      alias: 'automatic',
      target: 'replacement-model'
    }));
    expect(screen.getAllByText('automatic')).toHaveLength(1);
    expect(screen.getByText('replacement-model')).toBeInTheDocument();
    expect(screen.queryByText('target-model')).not.toBeInTheDocument();
    expect(mocks.toast).toHaveBeenCalledWith('Alias “automatic” updated and applied immediately', 'success');
    expect(screen.getByRole('button', { name: /Delete alias automatic/i })).toBeInTheDocument();
  });

  it('shows truthful failure feedback and keeps aliases unchanged when immediate operations fail', async () => {
    mocks.post.mockRejectedValueOnce(new Error('Create request rejected'));
    mocks.del.mockRejectedValueOnce(new Error('Delete request rejected'));
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));

    await fireEvent.click(screen.getByRole('button', { name: /Add Alias/i }));
    await fireEvent.input(screen.getByPlaceholderText(/Alias name/i), { target: { value: 'friendly' } });
    await fireEvent.input(screen.getByPlaceholderText(/Target model/i), { target: { value: 'target-model' } });
    await fireEvent.click(screen.getByRole('button', { name: /Create alias/i }));
    await waitFor(() => expect(mocks.toast).toHaveBeenCalledWith('Alias “friendly” was not created: Create request rejected', 'error'));
    expect(screen.getAllByRole('alert').some((alert) => alert.textContent?.includes('Alias “friendly” was not created: Create request rejected'))).toBe(true);
    expect(screen.queryByRole('button', { name: /Delete alias friendly/i })).not.toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: /Delete alias automatic/i }));
    await waitFor(() => expect(mocks.toast).toHaveBeenCalledWith('Alias “automatic” was not deleted: Delete request rejected', 'error'));
    expect(screen.getAllByRole('alert').some((alert) => alert.textContent?.includes('Alias “automatic” was not deleted: Delete request rejected'))).toBe(true);
    expect(screen.getByRole('button', { name: /Delete alias automatic/i })).toBeInTheDocument();
    expect(mocks.put).not.toHaveBeenCalled();
    expect(screen.queryByText(/unsaved scope/i)).not.toBeInTheDocument();
  });

  it('renders one provider for two accounts, unions models, and persists provider identity', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));
    const qoderProvider = await screen.findByRole('button', { name: /Qoder.*2 accounts.*3 models/i });
    expect(screen.getAllByRole('button', { name: /Qoder.*accounts.*models/i })).toHaveLength(1);

    await fireEvent.click(qoderProvider);
    expect(screen.getByRole('option', { name: /qoder-only-a/i })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: /qoder-only-b/i })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: /shared-model/i })).toBeInTheDocument();

    await fireEvent.input(screen.getByPlaceholderText(/Combo name/i), { target: { value: 'qoder-pool' } });
    await fireEvent.click(screen.getByRole('option', { name: /shared-model/i }));
    await fireEvent.click(screen.getByRole('button', { name: 'Add entry' }));
    const summary = screen.getByRole('region', { name: /Pre-save route summary/i });
    expect(summary).toHaveTextContent('Qoder');
    expect(summary).toHaveTextContent('Provider pool');
    await fireEvent.click(screen.getByRole('button', { name: 'Create Combo' }));
    await waitFor(() => expect(mocks.post).toHaveBeenCalledWith('/api/combos', expect.objectContaining({
      entries: [{ model: 'shared-model', provider_id: 'provider:qoder:https://api.qoder.com/chat/completions' }],
    })));
  });

  it('presents canonical provider hierarchy, explicit selection, searchable models, and a route summary', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));

    const provider = await screen.findByRole('button', { name: /Qoder.*2 accounts.*3 models/i });
    expect(provider).toHaveTextContent('Qoder');
    expect(provider).toHaveTextContent('Qoder Alice, Qoder Bob');
    expect(provider).toHaveAttribute('aria-pressed', 'false');
    await fireEvent.click(provider);
    expect(provider).toHaveAttribute('aria-pressed', 'true');
    expect(provider).toHaveTextContent('Selected');

    const search = screen.getByRole('combobox', { name: /Search Qoder models/i });
    await fireEvent.input(search, { target: { value: 'only-b' } });
    expect(screen.getByRole('option', { name: /qoder-only-b/i })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: /qoder-only-a/i })).not.toBeInTheDocument();
    await fireEvent.keyDown(search, { key: 'Escape' });
    expect(search).toHaveValue('');

    await fireEvent.click(screen.getByRole('option', { name: /shared-model/i }));
    await fireEvent.click(screen.getByRole('button', { name: 'Add entry' }));
    const summary = screen.getByRole('region', { name: /Pre-save route summary/i });
    expect(summary).toHaveTextContent('Qoder');
    expect(summary).toHaveTextContent('Provider pool');
    expect(summary).toHaveTextContent('shared-model');
  });

  it('clears a selected model hidden by a new search and disables Add entry', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Qoder.*2 accounts.*3 models/i }));

    await fireEvent.click(screen.getByRole('option', { name: /qoder-only-a/i }));
    expect(screen.getByRole('button', { name: 'Add entry' })).toBeEnabled();

    await fireEvent.input(screen.getByRole('combobox', { name: /Search Qoder models/i }), { target: { value: 'only-b' } });

    expect(screen.queryByRole('option', { name: /qoder-only-a/i })).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Add entry' })).toBeDisabled());
  });

  it('keeps a selected model addable while the search still matches it', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Qoder.*2 accounts.*3 models/i }));
    await fireEvent.click(screen.getByRole('option', { name: /qoder-only-a/i }));

    await fireEvent.input(screen.getByRole('combobox', { name: /Search Qoder models/i }), { target: { value: 'only-a' } });

    expect(screen.getByRole('option', { name: /qoder-only-a/i })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('button', { name: 'Add entry' })).toBeEnabled();
  });

  it('does not auto-select a previously hidden model when search is cleared', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Qoder.*2 accounts.*3 models/i }));
    const search = screen.getByRole('combobox', { name: /Search Qoder models/i });
    await fireEvent.click(screen.getByRole('option', { name: /qoder-only-a/i }));
    await fireEvent.input(search, { target: { value: 'only-b' } });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Add entry' })).toBeDisabled());

    await fireEvent.keyDown(search, { key: 'Escape' });

    expect(screen.getByRole('option', { name: /qoder-only-a/i })).toHaveAttribute('aria-selected', 'false');
    expect(screen.getByRole('button', { name: 'Add entry' })).toBeDisabled();
  });

  it('distinguishes no synced models and exposes the existing safe sync action', async () => {
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/connections') return { data: [{ id: 'qoder-a', name: 'Qoder Alice', format: 'qoder', base_url: 'https://api.qoder.com', chat_path: '/chat/completions', is_active: 1 }] };
      if (path === '/api/presets') return { data: [{ name: 'Qoder', domain: 'qoder.com', base_url: 'https://api.qoder.com/v1' }] };
      if (path === '/api/models/discovered') return { data: [] };
      if (path === '/api/combos') return { data: [] };
      if (path === '/api/aliases') return { data: {} };
      if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
      if (path === '/api/smart-routing') return { data: { quota_limits: {} } };
      return { data: [] };
    });
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Qoder.*0 models/i }));
    expect(screen.getByText('No synced models').closest('[role="status"]')).toHaveTextContent(/No active discovered models/i);
    expect(screen.getByRole('button', { name: /Sync Models/i })).toBeEnabled();
  });

  it('distinguishes inactive providers from an empty catalog', async () => {
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/connections') return { data: [{ id: 'off', name: 'Paused account', is_active: 0 }] };
      if (path === '/api/combos') return { data: [] };
      if (path === '/api/aliases') return { data: {} };
      if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
      if (path === '/api/smart-routing') return { data: { quota_limits: {} } };
      return { data: [] };
    });
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));
    expect(await screen.findByText('Providers unavailable')).toBeInTheDocument();
    expect(screen.getByText(/connections exist but none are active/i)).toBeInTheDocument();
  });

  it('shows an API failure with a retry action', async () => {
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/connections') throw new Error('Network unavailable');
      if (path === '/api/combos') return { data: [] };
      if (path === '/api/aliases') return { data: {} };
      if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
      if (path === '/api/smart-routing') return { data: { quota_limits: {} } };
      return { data: [] };
    });
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));
    expect(await screen.findByText('Provider catalog unavailable')).toBeInTheDocument();
    expect(screen.getByText('Network unavailable')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
  });

  it('advanced account pin persists the exact connection id', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Qoder.*2 accounts.*3 models/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Advanced: pin to specific account/i }));
    await fireEvent.change(screen.getByLabelText('Pin to specific account'), { target: { value: 'qoder-b' } });
    await fireEvent.click(screen.getByRole('option', { name: /qoder-only-b/i }));
    await fireEvent.click(screen.getByRole('button', { name: 'Add entry' }));
    const summary = screen.getByRole('region', { name: /Pre-save route summary/i });
    expect(summary).toHaveTextContent('Qoder Bob');
    expect(summary).toHaveTextContent('qoder-only-b');
  });

  it('uses the canonical provider name on existing combo rows', async () => {
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/combos') return { data: [{ id: 'existing', name: 'Existing', strategy: 'priority', models: ['shared-model'], entries: [{ model: 'shared-model', connection_id: 'qoder-b' }], order: 0 }] };
      if (path === '/api/connections') return { data: [
        { id: 'qoder-b', name: 'Qoder Bob', format: 'qoder', base_url: 'https://api.qoder.com', chat_path: '/chat/completions', is_active: 1 },
      ] };
      if (path === '/api/presets') return { data: [{ name: 'Qoder', domain: 'qoder.com', base_url: 'https://api.qoder.com/v1' }] };
      if (path === '/api/models/discovered') return { data: [{ model_id: 'shared-model', connection_id: 'qoder-b', is_active: 1 }] };
      if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
      if (path === '/api/smart-routing') return { data: { quota_limits: {} } };
      return { data: [] };
    });
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await waitFor(() => expect(screen.getByText('Qoder · shared-model')).toBeInTheDocument());
    expect(screen.getByText('Qoder Bob')).toBeInTheDocument();
  });

  it('opens one Edit form with ordered pool, exact-account, and legacy entries and preserves their identities through PUT', async () => {
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/combos') return { data: [{
        id: 'mixed', name: 'Mixed route', strategy: 'round-robin', description: 'Existing chain', order: 0,
        models: ['qoder-only-a', 'qoder-only-b', 'legacy-model'],
        entries: [
          { model: 'qoder-only-a', provider_id: 'provider:qoder:https://api.qoder.com/chat/completions' },
          { model: 'qoder-only-b', connection_id: 'qoder-b' },
          { model: 'legacy-model' },
        ],
      }] };
      if (path === '/api/connections') return { data: [
        { id: 'qoder-a', name: 'Qoder Alice', format: 'qoder', base_url: 'https://api.qoder.com', chat_path: '/chat/completions', is_active: 1 },
        { id: 'qoder-b', name: 'Qoder Bob', format: 'qoder', base_url: 'https://api.qoder.com', chat_path: '/chat/completions', is_active: 1 },
      ] };
      if (path === '/api/presets') return { data: [{ name: 'Qoder', base_url: 'https://api.qoder.com/v1' }] };
      if (path === '/api/models/discovered') return { data: [
        { model_id: 'qoder-only-a', connection_id: 'qoder-a', is_active: 1 },
        { model_id: 'qoder-only-b', connection_id: 'qoder-b', is_active: 1 },
      ] };
      if (path === '/api/aliases') return { data: {} };
      if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
      if (path === '/api/smart-routing') return { data: { quota_limits: {} } };
      return { data: [] };
    });
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Edit combo Mixed route/i }));

    expect(screen.getByText('Edit Combo')).toBeInTheDocument();
    expect(screen.getByDisplayValue('Mixed route')).toBeDisabled();
    expect(screen.getByDisplayValue('Existing chain')).toBeInTheDocument();
    const summary = screen.getByRole('region', { name: /Pre-save route summary/i });
    expect(summary).toHaveTextContent(/Qoder.*Provider pool.*qoder-only-a/s);
    expect(summary).toHaveTextContent(/Qoder.*Qoder Bob.*qoder-only-b/s);
    expect(summary).toHaveTextContent(/Legacy model-only.*legacy-model/s);

    await fireEvent.click(screen.getByRole('button', { name: /^Save Combo$/i }));
    await waitFor(() => expect(mocks.put).toHaveBeenCalledWith('/api/combos?id=mixed', expect.objectContaining({
      name: 'Mixed route', strategy: 'round-robin', description: 'Existing chain',
      models: ['qoder-only-a', 'qoder-only-b', 'legacy-model'],
      entries: [
        { model: 'qoder-only-a', provider_id: 'provider:qoder:https://api.qoder.com/chat/completions' },
        { model: 'qoder-only-b', connection_id: 'qoder-b' },
        { model: 'legacy-model' },
      ],
    })));
    expect(mocks.post).not.toHaveBeenCalledWith('/api/combos', expect.anything());
  });

  it('supports reorder and remove in Edit while a dirty cancel is guarded', async () => {
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/combos') return { data: [{ id: 'ordered', name: 'Ordered', strategy: 'priority', models: ['one', 'two'], entries: [{ model: 'one' }, { model: 'two' }], order: 0 }] };
      if (path === '/api/aliases') return { data: {} };
      if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
      if (path === '/api/smart-routing') return { data: { quota_limits: {} } };
      return { data: [] };
    });
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Edit combo Ordered/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Move two up/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Remove one from combo/i }));
    mocks.confirm.mockReturnValueOnce(false);
    await fireEvent.click(screen.getByRole('button', { name: /^Cancel$/i }));
    expect(mocks.confirm).toHaveBeenCalledWith(expect.stringMatching(/discard.*combo edits/i));
    expect(screen.getByText('Edit Combo')).toBeInTheDocument();
    mocks.confirm.mockReturnValueOnce(true);
    await fireEvent.click(screen.getByRole('button', { name: /^Cancel$/i }));
    expect(screen.queryByText('Edit Combo')).not.toBeInTheDocument();
  });

  it('keeps failed Edit state, then reloads and closes after a successful PUT', async () => {
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/combos') return { data: [{ id: 'retry', name: 'Retry route', strategy: 'priority', description: mocks.put.mock.calls.length >= 2 ? 'unsaved retry' : 'before', models: ['legacy'], entries: [{ model: 'legacy' }], order: 0, revision: 'rev-a' }] };
      if (path === '/api/aliases') return { data: {} };
      if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
      if (path === '/api/smart-routing') return { data: { quota_limits: {} } };
      return { data: [] };
    });
    mocks.put.mockRejectedValueOnce(new Error('persistence unavailable')).mockResolvedValueOnce({ status: 'updated' });
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Edit combo Retry route/i }));
    await fireEvent.input(screen.getByPlaceholderText(/Description/i), { target: { value: 'unsaved retry' } });
    await fireEvent.click(screen.getByRole('button', { name: /^Save Combo$/i }));
    await waitFor(() => expect(mocks.toast).toHaveBeenCalledWith(expect.stringMatching(/persistence unavailable/i), 'error'));
    expect(screen.getByDisplayValue('unsaved retry')).toBeInTheDocument();
    expect(screen.getByText('Edit Combo')).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: /^Save Combo$/i }));
    await waitFor(() => expect(mocks.put).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByText('Edit Combo')).not.toBeInTheDocument());
    expect(mocks.get.mock.calls.filter(([path]: [string]) => path === '/api/combos').length).toBeGreaterThan(1);
  });

  it('confirms backend-realistic lexicographic GET entries without erasing routing identity or array order', async () => {
    let comboReads = 0;
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/combos') {
        comboReads++;
        const before = {
          id: 'shapes', name: 'Shape route', strategy: 'priority', description: 'before', revision: 'a'.repeat(64),
          models: ['pool-model', 'pin-model', 'legacy-model'],
          entries: [
            { model: 'pool-model', provider_id: 'provider:qoder:https://api.qoder.com/chat/completions' },
            { model: 'pin-model', connection_id: 'qoder-b' },
            { model: 'legacy-model' },
          ],
        };
        if (comboReads === 1) return { data: [before] };
        return { data: [{
          description: 'after', entries: [
            { model: 'pool-model', provider_id: 'provider:qoder:https://api.qoder.com/chat/completions' },
            { connection_id: 'qoder-b', model: 'pin-model' },
            { model: 'legacy-model' },
          ], id: 'shapes', models: before.models, name: 'Shape route', revision: 'b'.repeat(64), strategy: 'priority',
        }] };
      }
      if (path === '/api/aliases') return { data: {} };
      if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
      if (path === '/api/smart-routing') return { data: { quota_limits: {} } };
      return { data: [] };
    });
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Edit combo Shape route/i }));
    await fireEvent.input(screen.getByPlaceholderText(/Description/i), { target: { value: 'after' } });
    await fireEvent.click(screen.getByRole('button', { name: /^Save Combo$/i }));
    await waitFor(() => expect(screen.queryByText('Edit Combo')).not.toBeInTheDocument());
    expect(mocks.put).toHaveBeenCalledTimes(1);
    expect(mocks.toast).toHaveBeenCalledWith(expect.stringMatching(/updated successfully/i), 'success');
  });

  it('sends the loaded revision and preserves a stale draft with a safe reload action on 409', async () => {
    const latest = { id: 'shared', name: 'Shared', strategy: 'priority', description: 'operator B', models: ['one'], entries: [{ model: 'one' }], order: 0, revision: 'rev-b' };
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/combos') return { data: [{ ...latest, description: 'original', revision: 'rev-a' }] };
      if (path === '/api/aliases') return { data: {} };
      if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
      if (path === '/api/smart-routing') return { data: { quota_limits: {} } };
      return { data: [] };
    });
    const conflict: any = new Error('Combo changed since this editor was opened.'); conflict.status = 409;
    mocks.put.mockRejectedValueOnce(conflict);
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Edit combo Shared/i }));
    await fireEvent.input(screen.getByPlaceholderText(/Description/i), { target: { value: 'operator A draft' } });
    await fireEvent.click(screen.getByRole('button', { name: /^Save Combo$/i }));
    await waitFor(() => expect(mocks.put).toHaveBeenCalledWith('/api/combos?id=shared', expect.objectContaining({ expected_revision: 'rev-a' })));
    expect(screen.getByDisplayValue('operator A draft')).toBeInTheDocument();
    expect(await screen.findByRole('alert')).toHaveTextContent(/changed.*reload/i);

    mocks.get.mockImplementation(async (path: string): Promise<any> => path === '/api/combos' ? { data: [latest] } : ({ data: [] }));
    mocks.confirm.mockReturnValueOnce(false);
    await fireEvent.click(screen.getByRole('button', { name: /Reload latest/i }));
    expect(screen.getByDisplayValue('operator A draft')).toBeInTheDocument();
    mocks.confirm.mockReturnValueOnce(true);
    await fireEvent.click(screen.getByRole('button', { name: /Reload latest/i }));
    expect(await screen.findByDisplayValue('operator B')).toBeInTheDocument();
    expect(mocks.put).toHaveBeenCalledTimes(1);
  });

  it('keeps the editor and list after PUT success plus reload failure, then retries a backend-realistic GET without another PUT', async () => {
    let comboReads = 0;
    const models = ['pool-model', 'pin-model', 'legacy-model'];
    const initialEntries = [
      { model: 'pool-model', provider_id: 'provider:qoder:https://api.qoder.com/chat/completions' },
      { model: 'pin-model', connection_id: 'qoder-b' },
      { model: 'legacy-model' },
    ];
    mocks.get.mockImplementation(async (path: string): Promise<any> => {
      if (path === '/api/combos') {
        comboReads++;
        if (comboReads === 2) throw new Error('refresh offline');
        const entries = comboReads === 1 ? initialEntries : [
          { model: 'pool-model', provider_id: 'provider:qoder:https://api.qoder.com/chat/completions' },
          { connection_id: 'qoder-b', model: 'pin-model' },
          { model: 'legacy-model' },
        ];
        return { data: [{ description: comboReads === 1 ? 'before' : 'after', entries, id: 'saved', models, name: 'Saved', order: 0, revision: comboReads === 1 ? 'a'.repeat(64) : 'b'.repeat(64), strategy: 'priority' }] };
      }
      if (path === '/api/aliases') return { data: {} };
      if (path === '/api/load-balancer') return { data: { strategy: 'priority' } };
      if (path === '/api/smart-routing') return { data: { quota_limits: {} } };
      return { data: [] };
    });
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Edit combo Saved/i }));
    await fireEvent.input(screen.getByPlaceholderText(/Description/i), { target: { value: 'after' } });
    await fireEvent.click(screen.getByRole('button', { name: /^Save Combo$/i }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/saved.*refresh failed/i);
    expect(screen.getByDisplayValue('after')).toBeInTheDocument();
    expect(screen.getByText('Saved')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: /Retry reload/i }));
    await waitFor(() => expect(screen.queryByText('Edit Combo')).not.toBeInTheDocument());
    expect(mocks.put).toHaveBeenCalledTimes(1);
  });

  it('guards form Escape and clears search before attempting to close', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Combos/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));
    const form = screen.getByRole('form', { name: /combo editor/i });
    await fireEvent.keyDown(form, { key: 'Escape' });
    expect(screen.queryByText('Create New Combo')).not.toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));
    await fireEvent.input(screen.getByPlaceholderText(/Combo name/i), { target: { value: 'dirty' } });
    mocks.confirm.mockReturnValueOnce(false);
    await fireEvent.keyDown(screen.getByRole('form', { name: /combo editor/i }), { key: 'Escape' });
    expect(screen.getByDisplayValue('dirty')).toBeInTheDocument();
    mocks.confirm.mockReturnValueOnce(true);
    await fireEvent.keyDown(screen.getByRole('form', { name: /combo editor/i }), { key: 'Escape' });
    expect(screen.queryByText('Create New Combo')).not.toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: /Add Combo/i }));
    await fireEvent.click(await screen.findByRole('button', { name: /Qoder.*2 accounts/i }));
    const search = screen.getByRole('combobox', { name: /Search Qoder models/i });
    await fireEvent.input(search, { target: { value: 'qoder-only' } });
    await fireEvent.keyDown(search, { key: 'Escape' });
    expect(search).toHaveValue('');
    expect(screen.getByText('Create New Combo')).toBeInTheDocument();
  });
});
