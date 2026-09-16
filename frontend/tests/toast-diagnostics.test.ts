import { cleanup, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import Toast from '../src/lib/components/Toast.svelte';
import { showToast } from '../src/lib/toast';

describe('Toast diagnostics', () => {
  afterEach(() => cleanup());

  it('renders generic HTTP status and latency fields as text', async () => {
    render(Toast);
    showToast('Safe test failed', 'error', 0, {
      code: 'upstream_error', type: 'model_test_error', message: '<b>Unavailable</b>',
      httpStatus: 503, latencyMs: 321, hint: 'Try another account.',
    });
    expect(await screen.findByText('HTTP 503')).not.toBeNull();
    expect(screen.getByText('321 ms')).not.toBeNull();
    expect(screen.getByText('<b>Unavailable</b>')).not.toBeNull();
    expect(document.body.querySelector('b')).toBeNull();
  });
});