import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import SupportTickets from '@/pages/shop/SupportTickets';
import { ThemeProvider } from '@/hooks/useTheme';

const ticket = {
  id: 7,
  clientEmail: 'alice',
  subject: 'Help',
  status: 'open',
  createdAt: 1,
  updatedAt: 1,
};
const markup = '<img src=x onerror="alert(1)">';
const detail = {
  ticket,
  messages: [{ id: 1, ticketId: 7, author: 'customer', body: markup, createdAt: 1 }],
};

function stubFetch(handler: (path: string, init?: RequestInit) => unknown) {
  const calls: { path: string; body: unknown }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const path = String(url).replace('/shop/api', '');
      calls.push({ path, body: init?.body ? JSON.parse(String(init.body)) : undefined });
      return { json: async () => handler(path, init) } as Response;
    }),
  );
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

describe('SupportTickets', () => {
  it('asks for the subscription id, lists the tickets and shows message markup as text', async () => {
    const calls = stubFetch((path) =>
      path.startsWith('/tickets/7')
        ? { success: true, obj: detail }
        : { success: true, obj: [ticket] },
    );
    const { container } = render(
      <ThemeProvider>
        <SupportTickets orderToken="" />
      </ThemeProvider>,
    );

    fireEvent.change(screen.getByPlaceholderText('Your current username'), {
      target: { value: 'alice' },
    });
    fireEvent.change(screen.getByPlaceholderText(/Subscription id/), {
      target: { value: 'sub-1' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Show my tickets' }));
    fireEvent.click(await screen.findByText('Help'));

    expect(await screen.findByText(markup)).toBeTruthy();
    expect(container.querySelector('img')).toBeNull();
    expect(calls[0].path).toBe('/tickets?email=alice&proof=sub-1');
  });

  it('shows the translated refusal when the proof is wrong', async () => {
    stubFetch(() => ({ success: false, msg: 'proof_invalid' }));
    render(
      <ThemeProvider>
        <SupportTickets orderToken="" />
      </ThemeProvider>,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Show my tickets' }));
    await waitFor(() =>
      expect(screen.getByText('The subscription id does not match this account.')).toBeTruthy(),
    );
  });

  it('uses the order token instead of asking for a username', async () => {
    const calls = stubFetch(() => ({ success: true, obj: [] }));
    render(
      <ThemeProvider>
        <SupportTickets orderToken="tok123" />
      </ThemeProvider>,
    );
    expect(screen.queryByPlaceholderText('Your current username')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Show my tickets' }));
    await waitFor(() => expect(calls[0]?.path).toBe('/tickets?order=tok123'));
  });
});
