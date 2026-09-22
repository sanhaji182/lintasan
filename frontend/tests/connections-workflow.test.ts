import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ConnectionsPage from '../src/routes/dashboard/connections/+page.svelte';

const mocks = vi.hoisted(() => ({
  post: vi.fn(async () => ({})),
  patch: vi.fn(async () => ({})),
  del: vi.fn(async () => ({})),
  get: vi.fn(async (path: string) => {
  if (path === '/api/connections') return {
    data: [
      { id: 'openai-1', name: 'OpenAI Production', base_url: 'https://api.openai.com/v1', format: 'openai', is_active: 1, priority: 1, models_count: 3, api_key: 'sk-secret-one' },
      { id: 'openai-2', name: 'OpenAI Backup', base_url: 'https://api.openai.com/v1', format: 'openai', is_active: 0, priority: 2, models_count: 3, api_key: 'sk-secret-two' },
    ],
  };
  if (path === '/api/providers/presets') return { data: [] };
  if (path === '/api/preset-categories') return { data: [] };
  if (path === '/api/oauth/status') return { enabled: false };
  if (path === '/api/connections/pools') return { data: [] };
  if (path === '/api/connections/balances') return { data: [] };
  return { data: [] };
  }),
}));
const { get, post, patch, del } = mocks;

vi.mock('$lib/api', () => ({ api: { get: mocks.get, post: mocks.post, patch: mocks.patch, delete: mocks.del } }));
vi.mock('$lib/toast', () => ({ showToast: vi.fn() }));

async function renderPage() {
  render(ConnectionsPage);
  await screen.findByText('OpenAI');
}

describe('Connections compact workflow', () => {
  beforeEach(() => {
    get.mockClear(); post.mockClear(); patch.mockClear(); del.mockClear();
    vi.stubGlobal('confirm', vi.fn(() => true));
  });
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  it('starts with provider summaries collapsed and expands from the keyboard with truthful state', async () => {
    await renderPage();
    const summary = screen.getByRole('button', { name: /OpenAI/i });
    expect(summary.getAttribute('aria-expanded')).toBe('false');
    expect(screen.queryByText('OpenAI Production')).toBeNull();
    summary.focus();
    await fireEvent.keyDown(summary, { key: 'Enter' });
    expect(summary.getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByText('OpenAI Production')).not.toBeNull();
  });

  it('filters providers without expanding account details', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /Inactive 1/i }));
    expect(screen.getByRole('button', { name: /OpenAI, 0 of 1 active/i })).not.toBeNull();
    expect(screen.queryByText('OpenAI Backup')).toBeNull();
  });

  it('enters explicit bulk selection mode, reveals accounts for selection, executes actions, and clears selection on exit', async () => {
    await renderPage();
    const bulk = screen.getByRole('button', { name: 'Bulk mode' });
    expect(screen.queryByRole('checkbox')).toBeNull();
    await fireEvent.click(bulk);
    await fireEvent.click(screen.getByRole('button', { name: /OpenAI/i }));
    const boxes = screen.getAllByRole('checkbox');
    expect(boxes).toHaveLength(2);
    await fireEvent.click(screen.getByRole('checkbox', { name: /OpenAI Production/i }));
    await fireEvent.click(screen.getByRole('button', { name: /Disable selected/i }));
    expect(post).toHaveBeenCalledWith('/api/connections/bulk-disable', { ids: ['openai-1'] });
    await fireEvent.click(screen.getByRole('button', { name: /Exit bulk mode/i }));
    expect(screen.queryByRole('checkbox')).toBeNull();
  });

  it('opens the contextual Add menu with keyboard focus and closes it with Escape', async () => {
    await renderPage();
    const add = screen.getByRole('button', { name: /^Add$/i });
    add.focus();
    await fireEvent.click(add);
    const menu = await screen.findByRole('menu');
    expect(add.getAttribute('aria-expanded')).toBe('true');
    expect(within(menu).getAllByRole('menuitem')).toHaveLength(4);
    await waitFor(() => expect(document.activeElement).toBe(within(menu).getByRole('menuitem', { name: /Provider API/i })));
    await fireEvent.keyDown(menu, { key: 'Escape' });
    expect(screen.queryByRole('menu')).toBeNull();
    expect(document.activeElement).toBe(add);
  });

  it('keeps ordinary per-provider and per-account actions reachable after expansion', async () => {
    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /OpenAI, 1 of 2 active/i }));
    expect(screen.getByRole('button', { name: 'Test Provider (2)' })).not.toBeNull();
    expect(screen.getAllByRole('button', { name: 'Test' })).toHaveLength(2);
    expect(screen.getByTitle('Click to deactivate')).not.toBeNull();
    expect(screen.getByTitle('Click to activate')).not.toBeNull();

    await fireEvent.click(screen.getAllByRole('button', { name: 'More' })[0]);
    expect(screen.getByRole('button', { name: 'Sync Models' })).not.toBeNull();
    expect(screen.getByRole('button', { name: 'View Models' })).not.toBeNull();
    expect(screen.getByRole('button', { name: 'Edit Pool' })).not.toBeNull();
    expect(screen.getByRole('button', { name: 'Copy Full Key' })).not.toBeNull();
    expect(screen.getByRole('button', { name: 'Delete' })).not.toBeNull();

    await fireEvent.click(screen.getAllByRole('button', { name: 'Group actions' })[0]);
    expect(screen.getByRole('button', { name: /Enable All/i })).not.toBeNull();
    expect(screen.getByRole('button', { name: /Disable All/i })).not.toBeNull();
  });


});
