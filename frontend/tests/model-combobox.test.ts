import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import ModelCombobox from '../src/lib/components/ModelCombobox.svelte';

const models = [
  {
    id: 'gpt-mini',
    label: 'gpt-mini',
    kind: 'provider' as const,
    route: null,
    provider: 'OpenAI',
    account: 'Production',
    connectionId: 'openai-1',
    health: 'healthy',
    capabilities: [],
    contextWindow: null,
    price: null,
    supportsStreaming: true,
  },
];

describe('ModelCombobox keyboard workflow', () => {
  afterEach(cleanup);

  it.each(['Enter', ' '])('opens once with %s and focuses the searchable combobox', async (key) => {
    render(ModelCombobox, { models, selected: '', onselect: vi.fn() });
    const trigger = screen.getByRole('button', { name: /choose a model/i });

    trigger.focus();
    await fireEvent.keyDown(trigger, { key });
    await fireEvent.click(trigger);

    const search = await screen.findByRole('combobox', { name: /search models/i });
    await waitFor(() => expect(search).toHaveFocus());
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
  });

  it('focuses search after mouse opening so typing filters immediately', async () => {
    render(ModelCombobox, { models, selected: '', onselect: vi.fn() });

    await fireEvent.click(screen.getByRole('button', { name: /choose a model/i }));

    const search = await screen.findByRole('combobox', { name: /search models/i });
    await waitFor(() => expect(search).toHaveFocus());
    await fireEvent.input(search, { target: { value: 'missing' } });
    expect(screen.getByText(/no callable models match/i)).toBeInTheDocument();
  });
});
