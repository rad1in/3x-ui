import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import ShopPage from '@/pages/shop/ShopPage';
import { ThemeProvider } from '@/hooks/useTheme';

const plan = {
  id: 1,
  name: 'Monthly 50GB',
  description: '',
  price: 250000,
  currency: 'IRT',
  totalGB: 50 * 1024 ** 3,
  durationDays: 30,
  limitIp: 2,
};

function orderView(over: Record<string, unknown> = {}) {
  return {
    token: 'tok123',
    planName: plan.name,
    kind: 'new',
    clientEmail: 'alice',
    amount: 250000,
    listAmount: 250000,
    discountCode: '',
    currency: 'IRT',
    gateway: 'card',
    status: 'pending',
    rejectReason: '',
    createdAt: 1,
    fulfilled: false,
    subscriptionUrl: '',
    card: {
      enabled: true,
      cardNumber: '6037991812345678',
      holderName: 'Ali Rezaei',
      bankName: '',
      instructions: '',
    },
    ...over,
  };
}

interface Route {
  method: string;
  path: string;
  reply: unknown;
}

function stubShop(routes: Route[]) {
  const calls: { method: string; path: string; body: unknown }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const path = String(url).replace('/shop/api', '');
      const method = init?.method ?? 'GET';
      calls.push({ method, path, body: init?.body ? JSON.parse(String(init.body)) : undefined });
      const hit = routes.find((r) => r.method === method && path.startsWith(r.path));
      return { json: async () => hit?.reply ?? { success: false, msg: 'network' } } as Response;
    }),
  );
  return calls;
}

function renderShop() {
  return render(
    <ThemeProvider>
      <ShopPage />
    </ThemeProvider>,
  );
}

async function chooseMonthlyPlanAndTypeUsername(name: string) {
  fireEvent.click(await screen.findByText('Monthly 50GB'));
  fireEvent.change(await screen.findByPlaceholderText(/Choose a username/), {
    target: { value: name },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
  window.history.replaceState(null, '', '/');
});

describe('ShopPage', () => {
  it('walks a card purchase from the plan to the receipt the admin will review', async () => {
    const calls = stubShop([
      {
        method: 'GET',
        path: '/plans',
        reply: { success: true, obj: { plans: [plan], methods: ['card'] } },
      },
      {
        method: 'POST',
        path: '/orders/tok123/receipt',
        reply: { success: true, obj: orderView({ status: 'review' }) },
      },
      { method: 'POST', path: '/orders', reply: { success: true, obj: orderView() } },
    ]);
    renderShop();

    await chooseMonthlyPlanAndTypeUsername('alice');
    fireEvent.click(screen.getByRole('button', { name: 'Buy' }));

    expect(await screen.findByText('6037991812345678')).toBeTruthy();
    expect(calls.find((c) => c.method === 'POST' && c.path === '/orders')?.body).toMatchObject({
      planId: 1,
      kind: 'new',
      email: 'alice',
      gateway: 'card',
      referralCode: '',
    });
    expect(window.location.search).toBe('?order=tok123');

    fireEvent.change(screen.getByPlaceholderText(/Tracking number/), {
      target: { value: '482913' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Send receipt' }));
    expect(await screen.findByText(/Receipt sent/)).toBeTruthy();
    expect(calls.find((c) => c.path.endsWith('/receipt'))?.body).toMatchObject({ ref: '482913' });
  });

  it('turns a refusal code from the shop into a readable message and keeps the form', async () => {
    stubShop([
      {
        method: 'GET',
        path: '/plans',
        reply: { success: true, obj: { plans: [plan], methods: ['card'] } },
      },
      { method: 'POST', path: '/orders', reply: { success: false, msg: 'email_taken' } },
    ]);
    renderShop();

    await chooseMonthlyPlanAndTypeUsername('taken');
    fireEvent.click(screen.getByRole('button', { name: 'Buy' }));
    expect(await screen.findByText('That username is already taken.')).toBeTruthy();
    expect(screen.getByPlaceholderText(/Choose a username/)).toBeTruthy();
  });

  it('shows the subscription link of a delivered order opened from its own url', async () => {
    window.history.replaceState(null, '', '/?order=tok123');
    stubShop([
      {
        method: 'GET',
        path: '/plans',
        reply: { success: true, obj: { plans: [plan], methods: ['card'] } },
      },
      {
        method: 'GET',
        path: '/orders/tok123',
        reply: {
          success: true,
          obj: orderView({
            status: 'paid',
            fulfilled: true,
            card: undefined,
            subscriptionUrl: 'https://sub.example.com/s/abc',
          }),
        },
      },
    ]);
    renderShop();
    await waitFor(() => expect(screen.getByText('https://sub.example.com/s/abc')).toBeTruthy());
  });
});
