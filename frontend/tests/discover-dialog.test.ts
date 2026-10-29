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
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/discover/free-providers') return { providers: [openCode] };
      if (path === '/api/connections') return { data: [] };
      throw new Error(`unexpected GET ${path}`);
    });
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

describe('Discover connection state', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.post.mockReset();
  });

  afterEach(cleanup);

  it('marks an existing active connection as Connected after reload', async () => {
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/discover/free-providers') return { providers: [openCode] };
      if (path === '/api/connections') {
        return { data: [{ id: 'c1', name: openCode.name, base_url: openCode.base_url, is_active: 1 }] };
      }
      throw new Error(`unexpected GET ${path}`);
    });

    render(DiscoverPage);
    const button = await screen.findByRole('button', { name: /Connected/i });
    expect(button).toBeDisabled();
  });

  it('hides disabled providers and tolerates a null model list', async () => {
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/discover/free-providers') {
        return {
          providers: [
            { ...openCode, models: null },
            { ...openCode, name: 'Unsupported Provider', enabled: false }
          ]
        };
      }
      if (path === '/api/connections') return { data: [] };
      throw new Error(`unexpected GET ${path}`);
    });

    render(DiscoverPage);
    expect(await screen.findByRole('button', { name: 'Connect with API Key' })).toBeInTheDocument();
    expect(screen.queryByText('Unsupported Provider')).not.toBeInTheDocument();
  });
});

describe('Discover live free-model refresh', () => {
  afterEach(cleanup);

  it('fetches free models only when asked and seeds them on connect', async () => {
    const freeProvider = {
      ...openCode,
      name: 'OpenRouter',
      prefix: 'or/',
      base_url: 'https://openrouter.ai/api/v1',
      models: ['seed-model:free'],
      free_only: true,
      seed_only: true,
    };
    mocks.get.mockReset();
    mocks.post.mockReset();
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/discover/free-providers') return { providers: [freeProvider] };
      if (path === '/api/connections') return { data: [] };
      if (path === '/api/discover/free-providers/or/models') {
        return { models: ['alpha:free', 'beta:free'], total: 2, source: 'live', free_only: true };
      }
      throw new Error(`unexpected GET ${path}`);
    });
    mocks.post.mockResolvedValue({ status: 'created', id: 'conn-openrouter', models_count: 2 });

    render(DiscoverPage);
    await screen.findByText('seed-model:free');
    expect(screen.queryByText('alpha:free')).not.toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Fetch models' }));
    expect(await screen.findByText(/2 free models/)).toBeInTheDocument();
    expect(screen.getByText('alpha:free')).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Connect with API Key' }));
    await fireEvent.input(screen.getByLabelText('Provider API key'), { target: { value: 'sk-test' } });
    await fireEvent.click(screen.getByRole('button', { name: /Save & Connect/ }));

    await waitFor(() => expect(mocks.post).toHaveBeenCalledWith('/api/connections', expect.objectContaining({
      name: 'OpenRouter',
      models: ['alpha:free', 'beta:free']
    })));
  });
});

describe('Discover provider logo rendering and fallback', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.post.mockReset();
  });

  afterEach(cleanup);

  it('renders provider logo from logo or icon_url', async () => {
    const providerWithLogo = {
      ...openCode,
      name: 'NVIDIA NIM',
      prefix: 'nvidia/',
      logo: '/provider-logos/nvidia.svg',
    };
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/discover/free-providers') return { providers: [providerWithLogo] };
      if (path === '/api/connections') return { data: [] };
      throw new Error(`unexpected GET ${path}`);
    });

    render(DiscoverPage);
    const img = await screen.findByRole('img', { name: 'NVIDIA NIM logo' });
    expect(img).toBeInTheDocument();
    expect(img).toHaveAttribute('src', '/provider-logos/nvidia.svg');
    expect(screen.getByText('nvidia')).toBeInTheDocument();
  });

  it('falls back to prefix monogram on image error', async () => {
    const providerWithErrorLogo = {
      ...openCode,
      name: 'Failing Logo Provider',
      prefix: 'fail/',
      icon_url: 'https://invalid-domain.example/logo.svg',
    };
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/discover/free-providers') return { providers: [providerWithErrorLogo] };
      if (path === '/api/connections') return { data: [] };
      throw new Error(`unexpected GET ${path}`);
    });

    render(DiscoverPage);
    const img = await screen.findByRole('img', { name: 'Failing Logo Provider logo' });
    expect(img).toBeInTheDocument();
    expect(screen.getByText('fail')).toBeInTheDocument();

    await fireEvent.error(img);
    expect(img).toHaveStyle({ display: 'none' });
  });

  it('renders monogram directly when no logo or icon_url is provided', async () => {
    const providerNoLogo = {
      ...openCode,
      name: 'Plain Provider',
      prefix: 'plain/',
      logo: undefined,
      icon_url: undefined,
    };
    mocks.get.mockImplementation(async (path: string) => {
      if (path === '/api/discover/free-providers') return { providers: [providerNoLogo] };
      if (path === '/api/connections') return { data: [] };
      throw new Error(`unexpected GET ${path}`);
    });

    render(DiscoverPage);
    await screen.findByText('Plain Provider');
    expect(screen.queryByRole('img', { name: 'Plain Provider logo' })).not.toBeInTheDocument();
    expect(screen.getByText('plain')).toBeInTheDocument();
  });
});

