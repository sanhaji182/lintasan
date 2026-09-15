import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import RoutingPage from '../src/routes/dashboard/routing/+page.svelte';

const mocks = vi.hoisted(() => ({
  beforeNavigateHandler: undefined as undefined | ((navigation: { cancel: () => void; willUnload?: boolean }) => void),
  get: vi.fn(async (path: string) => {
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
    mocks.get.mockImplementation(async (path: string) => {
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
});
