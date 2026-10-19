import { describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor } from '@testing-library/react';

import OrdersTab from '@/pages/sales/OrdersTab';
import { HttpUtil, Msg } from '@/utils';

import { renderWithProviders } from './test-utils';

function order(id: number, status: string, fulfilledAt: number) {
  return {
    id,
    token: `tok${id}`,
    planId: 1,
    planName: `plan-${id}`,
    kind: 'new',
    clientEmail: `user${id}`,
    contact: '',
    amount: 1000,
    currency: 'IRT',
    gateway: 'card',
    status,
    gatewayRef: '',
    receiptRef: '',
    receiptNote: '',
    rejectReason: '',
    totalGB: 0,
    durationDays: 30,
    limitIp: 0,
    createdAt: 1_700_000_000_000,
    updatedAt: 0,
    paidAt: 0,
    fulfilledAt,
  };
}

describe('OrdersTab actions', () => {
  it('offers approve and reject only on open orders and a retry only on paid-but-undelivered ones', async () => {
    const items = [
      order(1, 'review', 0),
      order(2, 'paid', 1_700_000_001_000),
      order(3, 'paid', 0),
      order(4, 'rejected', 0),
    ];
    vi.mocked(HttpUtil.get).mockImplementation(
      async (url: string) =>
        new Msg(true, '', url.includes('/panel/api/sales/orders') ? { items, total: 4 } : {}),
    );
    renderWithProviders(<OrdersTab />);

    expect(await screen.findByText('plan-1')).toBeTruthy();
    expect(screen.getAllByLabelText('Approve payment')).toHaveLength(1);
    expect(screen.getAllByLabelText('Reject')).toHaveLength(1);
    expect(screen.getAllByLabelText('Retry delivery')).toHaveLength(1);
    expect(screen.getByText('Paid, not delivered')).toBeTruthy();

    fireEvent.click(screen.getByLabelText('Retry delivery'));
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/sales/orders/approve/3'),
    );
  });
});
