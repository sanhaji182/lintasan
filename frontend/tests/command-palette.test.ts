import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import CommandPalette from '../src/lib/components/CommandPalette.svelte';

vi.mock('$app/navigation', () => ({ goto: vi.fn(async () => {}) }));

describe('CommandPalette', () => {
  afterEach(cleanup);

  it('opens from the keyboard, focuses search, filters real routes, and restores trigger focus', async () => {
    render(CommandPalette);
    const trigger = screen.getByRole('button', { name: /Search navigation/i });
    trigger.focus();

    await fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    const dialog = screen.getByRole('dialog', { name: /Command Center navigation/i });
    expect(dialog).toBeInTheDocument();
    const input = screen.getByRole('searchbox', { name: /Search pages/i });
    await waitFor(() => expect(document.activeElement).toBe(input));

    await fireEvent.input(input, { target: { value: 'routing' } });
    const result = screen.getByRole('link', { name: /Routing/i });
    expect(result).toHaveAttribute('href', '/dashboard/routing');
    expect(screen.queryByRole('link', { name: /Connections/i })).not.toBeInTheDocument();

    await fireEvent.keyDown(dialog, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it('opens with slash outside form fields and exposes no invented actions', async () => {
    render(CommandPalette);
    await fireEvent.keyDown(window, { key: '/' });
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getAllByRole('link').length).toBeGreaterThan(0);
    expect(screen.queryByRole('button', { name: /restart|deploy|clear cache/i })).not.toBeInTheDocument();
  });
});
