import { screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';

import ClientsInsightCard from '@/pages/index/ClientsInsightCard';
import { renderWithProviders } from '@/test/test-utils';
import { HttpUtil, Msg } from '@/utils';

afterEach(() => {
  vi.restoreAllMocks();
});

const GB = 1024 * 1024 * 1024;

function row(email: string, usedGB: number, totalGB: number, expiryTime = 0) {
  return {
    email,
    enable: true,
    createdAt: 0,
    updatedAt: 0,
    expiryTime,
    inboundIds: [1],
    limitHwid: 0,
    limitIp: 0,
    reset: 0,
    resetDay: 0,
    resetMax: 0,
    resetWeekday: 0,
    subId: email,
    totalGB: totalGB * GB,
    traffic: {
      id: 1,
      inboundId: 1,
      email,
      enable: true,
      up: 0,
      down: usedGB * GB,
      total: totalGB * GB,
      expiryTime,
      lastOnline: 0,
      lastSubFetch: 0,
      reset: 0,
      resetCount: 0,
      resetDay: 0,
      resetMax: 0,
      resetWeekday: 0,
      subId: email,
      uuid: '',
    },
  };
}

function page(items: ReturnType<typeof row>[]) {
  return {
    items,
    total: 12,
    filtered: items.length,
    page: 1,
    pageSize: 5,
    groups: [],
    summary: {
      total: 12,
      active: 9,
      onlineCount: 4,
      depletedCount: 2,
      expiringCount: 3,
      deactiveCount: 1,
      online: [],
      depleted: [],
      expiring: [],
      deactive: [],
    },
  };
}

function mockPaged() {
  const urls: string[] = [];
  vi.spyOn(HttpUtil, 'get').mockImplementation(async (url: string) => {
    urls.push(url);
    const qs = new URLSearchParams(url.split('?')[1] ?? '');
    if (qs.get('sort') === 'traffic' && qs.get('order') === 'descend') {
      return new Msg(true, '', page([row('heavy@x', 90, 100), row('light@x', 3, 0)]));
    }
    if (qs.get('sort') === 'expiryTime') {
      return new Msg(
        true,
        '',
        page([row('soon@x', 1, 10, Date.now() + 2 * 86_400_000), row('forever@x', 1, 10, 0)]),
      );
    }
    return new Msg(true, '', page([]));
  });
  return urls;
}

function renderCard() {
  return renderWithProviders(
    <MemoryRouter>
      <ClientsInsightCard />
    </MemoryRouter>,
  );
}

describe('ClientsInsightCard', () => {
  it('lists the heaviest users from a traffic-descending query', async () => {
    mockPaged();
    renderCard();

    const list = await screen.findByTestId('insight-top-usage');
    await waitFor(() => expect(within(list).getAllByRole('link')).toHaveLength(2));
    const emails = within(list)
      .getAllByRole('link')
      .map((a) => a.textContent);
    expect(emails).toEqual(['heavy@x', 'light@x']);
    expect(within(list).getAllByRole('link')[0].getAttribute('href')).toBe(
      '/clients?search=heavy%40x',
    );
  });

  it('lists live clients by soonest expiry and drops the never-expiring tail', async () => {
    const urls = mockPaged();
    renderCard();

    const list = await screen.findByTestId('insight-expiring');
    await waitFor(() => expect(within(list).getByRole('link').textContent).toBe('soon@x'));
    expect(within(list).queryByText('forever@x')).toBeNull();
    const qs = new URLSearchParams(urls.find((u) => u.includes('sort=expiryTime'))!.split('?')[1]);
    expect(qs.get('order')).toBe('ascend');
    expect(qs.get('filter')?.split(',').sort()).toEqual(['active', 'expiring']);
  });

  it('shows the panel-wide bucket counts from the summary', async () => {
    mockPaged();
    renderCard();

    const counts = await screen.findByTestId('insight-counts');
    await waitFor(() => expect(within(counts).getByText('12')).toBeTruthy());
    expect(within(counts).getByText('4')).toBeTruthy();
    expect(within(counts).getByText('3')).toBeTruthy();
    expect(within(counts).getByText('2')).toBeTruthy();
  });
});
