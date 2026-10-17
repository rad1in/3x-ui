import { fireEvent, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';

import ClientsEmptyState from '@/pages/clients/ClientsEmptyState';
import { renderWithProviders } from '@/test/test-utils';

function renderState(props: { hasAnyClient: boolean; hasInbound: boolean }) {
  const handlers = { onAdd: vi.fn(), onClear: vi.fn() };
  renderWithProviders(
    <MemoryRouter>
      <ClientsEmptyState {...props} {...handlers} />
    </MemoryRouter>,
  );
  return handlers;
}

function currentStep() {
  return document.querySelector('.getting-started-step[data-state="current"]');
}

describe('ClientsEmptyState', () => {
  it('points a panel without inbounds at creating one first', () => {
    renderState({ hasAnyClient: false, hasInbound: false });

    expect(currentStep()?.textContent).toBe('1Create an inbound');
    expect(currentStep()?.querySelector('a')?.getAttribute('href')).toBe('/inbounds');
  });

  it('marks the inbound step done and moves on to adding clients', () => {
    const { onAdd } = renderState({ hasAnyClient: false, hasInbound: true });

    const done = document.querySelector('.getting-started-step[data-state="done"]');
    expect(done?.textContent).toContain('Create an inbound');
    expect(currentStep()?.textContent).toBe('2Add clients to it');
    fireEvent.click(screen.getByRole('button', { name: /Add Clients/ }));
    expect(onAdd).toHaveBeenCalledTimes(1);
  });

  it('offers to clear filters, not to add clients, when clients exist', () => {
    const { onClear } = renderState({ hasAnyClient: true, hasInbound: true });

    expect(screen.queryByText('No clients yet')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /Clear/ }));
    expect(onClear).toHaveBeenCalledTimes(1);
  });
});
