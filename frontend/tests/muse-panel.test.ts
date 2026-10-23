import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import MusePage from '../src/routes/dashboard/muse/+page.svelte';

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  patch: vi.fn(),
  del: vi.fn(),
  toast: vi.fn()
}));

vi.mock('$lib/api', () => ({
  api: { get: mocks.get, post: mocks.post, patch: mocks.patch, delete: mocks.del }
}));
vi.mock('$lib/toast', () => ({ showToast: mocks.toast }));

const status = {
  configured: true,
  reachable: true,
  ok: true,
  base_url: 'http://127.0.0.1:18610',
  status: {
    version: '0.2.0',
    driver: { driver: 'browser', ok: true },
    capabilities: { renew_session: true, quota: false },
    accounts: { total: 2, available: 1, invalid: 1, inflight: 0 }
  }
};

const accounts = {
  data: [
    {
      id: 'acc_active',
      label: 'muse-main',
      enabled: true,
      status: 'active',
      cookies: { hatch_sess: 'abcd...wxyz', hatch_gw: 'efgh...1234' },
      cookie_expires: { hatch_sess: Math.floor(Date.now() / 1000) + 86400 },
      last_used_at: Math.floor(Date.now() / 1000) - 120,
      success_count: 12,
      fail_count: 1
    },
    {
      id: 'acc_invalid',
      label: 'muse-backup',
      enabled: true,
      status: 'invalid',
      cookies: { hatch_sess: 'zzzz...9999' },
      last_error: 'session rejected by upstream (HTTP 403)',
      success_count: 0,
      fail_count: 3
    }
  ]
};

async function renderPage() {
  render(MusePage);
  await screen.findByRole('heading', { name: /Muse Accounts/i });
}

describe('Muse accounts panel', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.post.mockReset();
    mocks.patch.mockReset();
    mocks.del.mockReset();
    mocks.toast.mockReset();
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/muse/status') return status;
      if (path === '/api/muse/accounts') return accounts;
      throw new Error(`unexpected GET ${path}`);
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('shows sidecar status and problem accounts first', async () => {
    await renderPage();
    expect(await screen.findByText('muse-main')).toBeInTheDocument();
    expect(screen.getByText('muse-backup')).toBeInTheDocument();
    expect(screen.getByText('Invalid session')).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: /Renew session/i }).length).toBe(2);
    expect(screen.getByText(/session rejected by upstream/i)).toBeInTheDocument();
    expect(screen.getByText(/v0.2.0/)).toBeInTheDocument();
  });

  it('imports a raw cookie header through the proxy endpoint', async () => {
    mocks.post.mockResolvedValueOnce({ account: { id: 'acc_new' }, missing_cookies: [] });
    await renderPage();

    const labelInput = await screen.findByLabelText('Label (optional)');
    await fireEvent.input(labelInput, { target: { value: 'new-account' } });
    await fireEvent.input(screen.getByLabelText('Cookies'), {
      target: { value: 'hatch_sess=abc; hatch_gw=def; hatch_vml=ghi; hatch_native_auth_device=jkl' }
    });
    await fireEvent.click(screen.getByRole('button', { name: /Import account/i }));

    await waitFor(() => {
      expect(mocks.post).toHaveBeenCalledWith('/api/muse/accounts', {
        label: 'new-account',
        cookies: 'hatch_sess=abc; hatch_gw=def; hatch_vml=ghi; hatch_native_auth_device=jkl'
      });
    });
    expect(mocks.toast).toHaveBeenCalledWith('Muse account imported', 'success', 5000);
  });

  it('toggles an account through PATCH', async () => {
    await renderPage();
    const toggle = await screen.findByRole('checkbox', { name: /Disable muse-main/i });
    await fireEvent.change(toggle, { target: { checked: false } });
    await waitFor(() => {
      expect(mocks.patch).toHaveBeenCalledWith('/api/muse/accounts/acc_active', { enabled: false });
    });
  });

  it('generates a one-time token for the browser extension', async () => {
    mocks.post.mockResolvedValueOnce({ token: 'museimp_testtoken', expires_at: 123, expires_in: 600 });
    await renderPage();

    await fireEvent.click(await screen.findByRole('button', { name: /Generate import token/i }));
    await waitFor(() => expect(mocks.post).toHaveBeenCalledWith('/api/muse/import-token', {}));
    expect(await screen.findByText('museimp_testtoken')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Copy/i })).toBeInTheDocument();
  });

  it('shows setup instructions when the sidecar is not configured', async () => {
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/muse/status') {
        return { configured: false, reachable: false, ok: false, base_url: 'http://127.0.0.1:18610' };
      }
      throw new Error(`unexpected GET ${path}`);
    });
    render(MusePage);
    expect(await screen.findByText('Connect the muse2api sidecar')).toBeInTheDocument();
    expect(screen.getByText(/LINTASAN_MUSE2API_ADMIN_KEY/)).toBeInTheDocument();
  });
});
