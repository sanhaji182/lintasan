import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import ConfirmDialog from '../src/lib/components/ConfirmDialog.svelte';
import { confirmAction, resolveConfirm, setConfirmHostMounted } from '../src/lib/confirm';

afterEach(() => {
  resolveConfirm(false);
  cleanup();
  setConfirmHostMounted(false);
  vi.unstubAllGlobals();
});

describe('ConfirmDialog', () => {
  it('resolves true only when the confirm action is chosen', async () => {
    render(ConfirmDialog);
    const result = confirmAction({ title: 'Delete this key?', detail: 'This cannot be undone.', confirmLabel: 'Delete', tone: 'danger' });
    expect(await screen.findByRole('alertdialog')).toBeInTheDocument();
    expect(screen.getByText('Delete this key?')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    await expect(result).resolves.toBe(true);
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
  });

  it('resolves false on cancel and Escape', async () => {
    render(ConfirmDialog);
    const cancelled = confirmAction({ title: 'Discard edits?', confirmLabel: 'Discard' });
    expect(await screen.findByRole('alertdialog')).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await expect(cancelled).resolves.toBe(false);

    const escaped = confirmAction({ title: 'Delete user?' });
    expect(await screen.findByRole('alertdialog')).toBeInTheDocument();
    await fireEvent.keyDown(screen.getByRole('alertdialog'), { key: 'Escape' });
    await expect(escaped).resolves.toBe(false);
  });

  it('falls back to the native dialog when the host is not mounted', async () => {
    const nativeConfirm = vi.fn(() => true);
    vi.stubGlobal('confirm', nativeConfirm);
    await expect(confirmAction({ title: 'Delete?' })).resolves.toBe(true);
    expect(nativeConfirm).toHaveBeenCalledWith('Delete?');
  });
});
