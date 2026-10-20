import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';

import { AllSetting } from '@/models/setting';
import { HttpUtil, Msg } from '@/utils';
import GeneralTab from '@/pages/settings/GeneralTab';
import { renderWithProviders } from './test-utils';

const hostedView = (enabled: boolean) => ({
  enabled,
  limits: {
    maxInbounds: 0,
    maxClients: 0,
    trafficBytes: 0,
    expiresAt: 0,
    portMin: 0,
    portMax: 0,
    usageEpoch: 0,
  },
  inbounds: 0,
  clients: 0,
  usedBytes: 0,
  blocked: '',
});

// Mounting fetches inbound options with no visible change; settle it inside act().
async function renderGeneralTab(updateSetting: (patch: Partial<AllSetting>) => void) {
  renderWithProviders(
    <MemoryRouter initialEntries={['/settings']}>
      <GeneralTab allSetting={new AllSetting({ pageSize: 25 })} updateSetting={updateSetting} />
    </MemoryRouter>,
  );
  await act(async () => {});
}

describe('GeneralTab', () => {
  it('keeps the stored page size when the field is cleared', async () => {
    const updateSetting = vi.fn();

    await renderGeneralTab(updateSetting);

    const pageSizeInput = screen.getByDisplayValue('25');
    fireEvent.change(pageSizeInput, { target: { value: '' } });
    fireEvent.blur(pageSizeInput);

    expect(updateSetting).not.toHaveBeenCalled();
    expect((pageSizeInput as HTMLInputElement).value).toBe('25');
  });

  it('forwards typed page sizes unchanged, zero included', async () => {
    const updateSetting = vi.fn();

    await renderGeneralTab(updateSetting);

    fireEvent.change(screen.getByDisplayValue('25'), { target: { value: '0' } });

    expect(updateSetting).toHaveBeenCalledWith({ pageSize: 0 });
  });

  it('shows the panel address fields on an ordinary panel and hides them on a hosted one', async () => {
    const get = vi.spyOn(HttpUtil, 'get');
    get.mockResolvedValue(new Msg(true, '', hostedView(false)));
    const ordinary = renderWithProviders(
      <MemoryRouter initialEntries={['/settings']}>
        <GeneralTab allSetting={new AllSetting({ pageSize: 25 })} updateSetting={vi.fn()} />
      </MemoryRouter>,
    );
    await act(async () => {});
    expect(screen.getByText('Listen Port')).not.toBeNull();
    expect(screen.getByText('URI Path')).not.toBeNull();
    ordinary.unmount();

    get.mockResolvedValue(new Msg(true, '', hostedView(true)));
    renderWithProviders(
      <MemoryRouter initialEntries={['/settings']}>
        <GeneralTab allSetting={new AllSetting({ pageSize: 25 })} updateSetting={vi.fn()} />
      </MemoryRouter>,
    );
    await act(async () => {});
    await waitFor(() => expect(screen.queryByText('Listen Port')).toBeNull());
    expect(screen.queryByText('URI Path')).toBeNull();
    expect(screen.queryByText('Listen IP')).toBeNull();
    get.mockRestore();
  });
});
