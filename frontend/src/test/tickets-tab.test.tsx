import { describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor } from '@testing-library/react';

import TicketsTab from '@/pages/sales/TicketsTab';
import { HttpUtil, Msg } from '@/utils';

import { renderWithProviders } from './test-utils';

const ticket = {
  id: 4,
  clientEmail: 'alice',
  subject: 'Cannot connect',
  status: 'open',
  createdAt: 1,
  updatedAt: 2,
};
const markup = '<b onmouseover="alert(1)">hi</b>';
const detail = {
  ticket,
  messages: [{ id: 1, ticketId: 4, author: 'customer', body: markup, createdAt: 1 }],
};

function stubApi() {
  vi.mocked(HttpUtil.get).mockImplementation(async (url: string) => {
    if (url === '/panel/api/sales/tickets') return new Msg(true, '', { items: [ticket], total: 1 });
    if (url === '/panel/api/sales/tickets/4') return new Msg(true, '', detail);
    return new Msg(true, '', {});
  });
  vi.mocked(HttpUtil.post).mockReset();
  vi.mocked(HttpUtil.post).mockResolvedValue(new Msg(true, '', detail));
}

describe('TicketsTab', () => {
  it('opens a ticket, shows the customer text without interpreting it and posts the reply', async () => {
    stubApi();
    const { container } = renderWithProviders(<TicketsTab />);

    fireEvent.click(await screen.findByText('Cannot connect'));
    expect(await screen.findByText(markup)).toBeTruthy();
    expect(container.ownerDocument.querySelector('.ticket-message b')).toBeNull();

    fireEvent.change(screen.getByPlaceholderText('Write a reply (plain text)'), {
      target: { value: 'Try another server' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Send reply' }));
    await waitFor(() =>
      expect(vi.mocked(HttpUtil.post)).toHaveBeenCalledWith(
        '/panel/api/sales/tickets/reply/4',
        { body: 'Try another server' },
        expect.anything(),
      ),
    );
  });
});
