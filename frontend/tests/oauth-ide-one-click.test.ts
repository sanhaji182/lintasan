import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import OAuthIdePage from '../src/routes/dashboard/oauth-ide/+page.svelte';

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  del: vi.fn(),
  toast: vi.fn()
}));

vi.mock('$lib/api', () => ({
  api: { get: mocks.get, post: mocks.post, delete: mocks.del }
}));
vi.mock('$lib/toast', () => ({ showToast: mocks.toast }));

const status = {
  enabled: true,
  experimental: true,
  public_base: 'https://lintasan.example',
  disclaimer: 'Use only accounts you are authorized to connect.',
  source: 'Lintasan OAuth provider catalog',
  catalog: [
    { id: 'codex', name: 'Codex', flow: 'browser_redirect', implementation: 'ready' },
    { id: 'github', name: 'GitHub Copilot', flow: 'device_code', implementation: 'ready' },
    { id: 'cursor', name: 'Cursor', flow: 'token_import', implementation: 'import_only' }
  ]
};

const emptyAccounts = { enabled: true, accounts: [], providers: [] };

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function popup() {
  return {
    location: { href: '' },
    close: vi.fn(),
    closed: false
  } as unknown as Window;
}

async function renderPage() {
  render(OAuthIdePage);
  await screen.findByRole('button', { name: /^Connect Codex$/i });
}

describe('OAuth IDE one-click provider flow', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.post.mockReset();
    mocks.del.mockReset();
    mocks.toast.mockReset();
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/oauth/status') return status;
      if (path === '/api/oauth/accounts') return emptyAccounts;
      throw new Error(`unexpected GET ${path}`);
    });
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('starts a card browser flow without an acknowledgement gate and reserves the popup synchronously', async () => {
    const authorize = deferred<any>();
    mocks.post.mockReturnValueOnce(authorize.promise);
    const authPopup = popup();
    const open = vi.fn(() => authPopup);
    vi.stubGlobal('open', open);

    await renderPage();
    expect(screen.queryByRole('checkbox')).toBeNull();
    expect(screen.queryByText('Authorize (admin)')).toBeNull();

    await fireEvent.click(screen.getByRole('button', { name: /^Connect Codex$/i }));

    expect(open).toHaveBeenCalledWith('', '_blank');
    expect(mocks.post).toHaveBeenCalledWith('/api/oauth/authorize', {
      provider: 'codex',
      acknowledge_risk: true
    });
    expect(screen.getByText('Waiting for authorization…')).toBeInTheDocument();

    authorize.resolve({
      flow: 'browser_redirect',
      session_id: 'session-codex',
      redirect_url: 'https://provider.example/oauth'
    });
    await waitFor(() => expect(authPopup.location.href).toBe('https://provider.example/oauth'));
  });

  it('opens device verification and polls automatically until the account becomes healthy', async () => {
    vi.useFakeTimers();
    const authPopup = popup();
    vi.stubGlobal('open', vi.fn(() => authPopup));
    mocks.post
      .mockResolvedValueOnce({
        flow: 'device_code',
        session_id: 'session-github',
        device: {
          user_code: 'ABCD-EFGH',
          verification_uri: 'https://github.com/login/device',
          verification_uri_complete: 'https://github.com/login/device?user_code=ABCD-EFGH',
          interval: 1,
          expires_in: 30
        }
      })
      .mockResolvedValueOnce({ status: 'pending', hint: 'complete GitHub device login' })
      .mockResolvedValueOnce({ status: 'active', provider: 'github' });

    let accountLoads = 0;
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/oauth/status') return status;
      if (path === '/api/oauth/accounts') {
        accountLoads += 1;
        if (accountLoads < 2) return emptyAccounts;
        return {
          enabled: true,
          accounts: [{ id: 'account-1', provider: 'github', status: 'active', health: 'healthy' }],
          providers: [{ provider: 'github', name: 'GitHub Copilot', active: 1, expiring: 0, restricted: 0, expired: 0, revoked: 0, total: 1, wired: false }]
        };
      }
      throw new Error(`unexpected GET ${path}`);
    });

    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Connect GitHub Copilot$/i }));
    await waitFor(() => expect(authPopup.location.href).toContain('user_code=ABCD-EFGH'));
    expect(await screen.findByText('ABCD-EFGH')).toBeInTheDocument();

    await vi.advanceTimersByTimeAsync(1000);
    expect(mocks.post).toHaveBeenCalledWith('/api/oauth/device/poll?session_id=session-github', {});
    expect(screen.getByText(/Waiting for authorization/i)).toBeInTheDocument();

    await vi.advanceTimersByTimeAsync(1000);
    await vi.runAllTicks();
    expect(await screen.findByText('Healthy')).toBeInTheDocument();
    expect(screen.getAllByText('Not wired').length).toBeGreaterThan(0);
  });

  it('closes the reserved popup and shows the backend error when authorization fails', async () => {
    const authPopup = popup();
    vi.stubGlobal('open', vi.fn(() => authPopup));
    mocks.post.mockRejectedValueOnce(new Error('provider is not configured'));

    await renderPage();
    await fireEvent.click(screen.getByRole('button', { name: /^Connect Codex$/i }));

    expect(await screen.findByText('provider is not configured')).toBeInTheDocument();
    expect(authPopup.close).toHaveBeenCalledOnce();
  });

  it('labels an existing provider action as Add another account', async () => {
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/oauth/status') return status;
      if (path === '/api/oauth/accounts') {
        return {
          enabled: true,
          accounts: [{ id: 'account-1', provider: 'codex', status: 'active', health: 'healthy' }],
          providers: [{ provider: 'codex', name: 'Codex', active: 1, expiring: 0, restricted: 0, expired: 0, revoked: 0, total: 1, wired: false }]
        };
      }
      throw new Error(`unexpected GET ${path}`);
    });

    render(OAuthIdePage);
    expect(await screen.findByRole('button', { name: /^Add another Codex account$/i })).toBeInTheDocument();
  });
});
