import { screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';

import SponsorsPage from '@/pages/sponsors/SponsorsPage';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

vi.mock('@/api/queries/useAllSettings', () => ({
  useAllSettings: () => ({ allSetting: {} }),
}));

afterEach(() => {
  vi.restoreAllMocks();
});

const limits = {
  maxInbounds: 0,
  maxClients: 0,
  trafficBytes: 0,
  expiresAt: 0,
  portMin: 0,
  portMax: 0,
  usageEpoch: 0,
};

function mockPanel(hosted: boolean) {
  return vi.spyOn(HttpUtil, 'get').mockImplementation(async (url: string) => {
    if (url === '/panel/api/hosted/status') {
      return new Msg(true, '', {
        enabled: hosted,
        limits,
        inbounds: 0,
        clients: 0,
        usedBytes: 0,
        blocked: '',
      });
    }
    if (url === '/panel/api/sponsors/list') {
      return new Msg(true, '', { records: [], config: { feedUrl: '', contact: '' } });
    }
    return new Msg(true, '', { sponsors: [] });
  });
}

// The owner's sponsors are published into hosted panels; their admins only see them.
test('a hosted panel has no sponsor management section', async () => {
  const get = mockPanel(false);
  const own = renderWithProviders(
    <MemoryRouter>
      <SponsorsPage />
    </MemoryRouter>,
  );
  expect(await screen.findByText('Manage sponsors')).not.toBeNull();
  own.unmount();

  get.mockRestore();
  const hostedGet = mockPanel(true);
  renderWithProviders(
    <MemoryRouter>
      <SponsorsPage />
    </MemoryRouter>,
  );
  await waitFor(() =>
    expect(hostedGet).toHaveBeenCalledWith('/panel/api/hosted/status', undefined, { silent: true }),
  );
  await waitFor(() => expect(screen.queryByText('Manage sponsors')).toBeNull());
  expect(hostedGet).not.toHaveBeenCalledWith('/panel/api/sponsors/list', undefined, {
    silent: true,
  });
});
