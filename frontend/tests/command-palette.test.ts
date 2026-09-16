import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import CommandPalette from '../src/lib/components/CommandPalette.svelte';

const mocks = vi.hoisted(() => ({ goto: vi.fn(async () => {}) }));
vi.mock('$app/navigation', () => ({ goto: mocks.goto }));

describe('CommandPalette', () => {
  afterEach(() => { cleanup(); mocks.goto.mockClear(); });

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

  it('loops focus in the modal and never lets Tab reach the background trigger', async () => {
    render(CommandPalette);
    const trigger = screen.getByRole('button', { name: /Search navigation/i });
    await fireEvent.click(trigger);
    const dialog = screen.getByRole('dialog');
    const input = screen.getByRole('searchbox');
    await waitFor(() => expect(document.activeElement).toBe(input));
    await fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true });
    expect(document.activeElement).toBe(screen.getAllByRole('link').at(-1));
    screen.getAllByRole('link').at(-1)?.focus();
    await fireEvent.keyDown(dialog, { key: 'Tab' });
    expect(document.activeElement).toBe(input);
    expect(document.activeElement).not.toBe(trigger);
  });

  it('moves an active result with arrows, opens it with Enter, and resets selection after filtering', async () => {
    render(CommandPalette);
    await fireEvent.click(screen.getByRole('button', { name: /Search navigation/i }));
    let dialog = screen.getByRole('dialog');
    const input = screen.getByRole('searchbox');
    await fireEvent.keyDown(input, { key: 'ArrowDown' });
    const activeAfterDown = screen.getAllByRole('link').find(link => link.getAttribute('aria-current') === 'true');
    expect(activeAfterDown).toBeTruthy();
    await fireEvent.keyDown(input, { key: 'Enter' });
    expect(mocks.goto).toHaveBeenCalledWith(activeAfterDown?.getAttribute('href'));

    await fireEvent.click(screen.getByRole('button', { name: /Search navigation/i }));
    dialog = screen.getByRole('dialog');
    await fireEvent.input(screen.getByRole('searchbox'), { target: { value: 'routing' } });
    expect(screen.getByRole('link', { name: /Routing/i })).toHaveAttribute('aria-current', 'true');
  });
});
