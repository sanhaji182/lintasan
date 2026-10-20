import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import DiscoverPage from '../src/routes/dashboard/discover/+page.svelte';

const mocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));
vi.mock('$lib/api', () => ({ api: { get: mocks.get, post: mocks.post } }));
vi.mock('$lib/toast', () => ({ showToast: vi.fn() }));

const openCode = {
  name: 'OpenCode Zen',
  prefix: 'oc/',
  base_url: 'https://opencode.ai/zen/v1',
  logo_url: 'https://opencode.ai/favicon.ico',
  models: ['big-pickle'],
  auth_type: 'apikey',
  quota_info: 'Free tier',
  enabled: true,
  signup_url: 'https://opencode.ai/auth',
};

describe('Discover API-key dialog accessibility', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.post.mockReset();
    mocks.get.mockResolvedValue({ providers: [openCode] });
  });

  afterEach(cleanup);

  it('focuses the key input and dismisses the dialog with Escape', async () => {
    render(DiscoverPage);
    const trigger = await screen.findByRole('button', { name: 'Connect with API Key' });
    await fireEvent.click(trigger);

    const dialog = screen.getByRole('dialog', { name: 'API key required' });
    expect(dialog).toHaveAttribute('tabindex', '-1');
    await waitFor(() => expect(document.activeElement).toBe(screen.getByPlaceholderText('sk-…')));

    await fireEvent.keyDown(dialog, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('only dismisses from a click on the backdrop itself', async () => {
    render(DiscoverPage);
    await fireEvent.click(await screen.findByRole('button', { name: 'Connect with API Key' }));

    const dialog = screen.getByRole('dialog', { name: 'API key required' });
    await fireEvent.click(dialog);
    expect(screen.getByRole('dialog')).toBeInTheDocument();

    await fireEvent.click(screen.getByTestId('api-key-backdrop'));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});
