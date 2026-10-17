import { screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import HostedUsageCard from '@/pages/index/HostedUsageCard';
import { renderWithProviders } from '@/test/test-utils';
import { HttpUtil, Msg } from '@/utils';

afterEach(() => {
  vi.restoreAllMocks();
});

const GB = 1024 ** 3;

function mockStatus(obj: unknown) {
  vi.spyOn(HttpUtil, 'get').mockResolvedValue(new Msg(true, '', obj));
}

const limits = {
  maxInbounds: 3,
  maxClients: 50,
  trafficBytes: 100 * GB,
  expiresAt: 0,
  portMin: 20000,
  portMax: 20099,
  usageEpoch: 0,
};

describe('HostedUsageCard', () => {
  it('renders nothing on an ordinary panel', async () => {
    mockStatus({
      enabled: false,
      limits: { ...limits, maxInbounds: 0 },
      inbounds: 0,
      clients: 0,
      usedBytes: 0,
      blocked: '',
    });
    const { container } = renderWithProviders(<HostedUsageCard />);
    await waitFor(() => expect(HttpUtil.get).toHaveBeenCalled());
    expect(container.querySelector('[data-testid="hosted-usage"]')).toBeNull();
  });

  it('shows usage against each owner limit and the port range', async () => {
    mockStatus({
      enabled: true,
      limits,
      inbounds: 2,
      clients: 40,
      usedBytes: 25 * GB,
      blocked: '',
    });
    renderWithProviders(<HostedUsageCard />);
    const card = await screen.findByTestId('hosted-usage');
    expect(card.textContent).toContain('2 / 3');
    expect(card.textContent).toContain('40 / 50');
    expect(card.textContent).toContain('25.00 GB / 100.00 GB');
    expect(card.textContent).toContain('20000–20099');
  });

  it('explains a suspension instead of leaving clients silently cut off', async () => {
    mockStatus({
      enabled: true,
      limits,
      inbounds: 2,
      clients: 40,
      usedBytes: 100 * GB,
      blocked: 'traffic',
    });
    renderWithProviders(<HostedUsageCard />);
    expect(
      await screen.findByText(
        'This panel has used all of its traffic. Contact your provider to renew it.',
      ),
    ).toBeTruthy();
  });
});
