import { describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor } from '@testing-library/react';

import SalesSettingsTab from '@/pages/sales/SalesSettingsTab';
import { HttpUtil, Msg } from '@/utils';

import { renderWithProviders } from './test-utils';

const view = {
  publicEnabled: true,
  publicBaseUrl: 'https://shop.example.com',
  card: { enabled: false, cardNumber: '', holderName: '', bankName: '', instructions: '' },
  zarinpal: { enabled: true, merchantId: '', sandbox: false },
  idpay: { enabled: false, apiKey: '', sandbox: false },
  crypto: { enabled: false, apiKey: '', ipnSecret: '', sandbox: false },
  hasZarinpalMerchantId: true,
  hasIdpayApiKey: false,
  hasCryptoApiKey: false,
  hasCryptoIpnSecret: false,
};

function savedBodies() {
  return vi
    .mocked(HttpUtil.post)
    .mock.calls.filter(([url]) => url === '/panel/api/sales/config/save')
    .map(([, body]) => body as Record<string, unknown>);
}

async function renderLoaded() {
  vi.mocked(HttpUtil.get).mockImplementation(
    async (url: string) => new Msg(true, '', url.includes('/panel/api/sales/config') ? view : {}),
  );
  vi.mocked(HttpUtil.post).mockClear();
  renderWithProviders(<SalesSettingsTab />);
  await screen.findByPlaceholderText('Saved. Leave blank to keep it');
}

describe('SalesSettingsTab secrets', () => {
  it('never shows a stored secret and saves it as blank so the server keeps it', async () => {
    await renderLoaded();
    const input = screen.getByPlaceholderText('Saved. Leave blank to keep it') as HTMLInputElement;
    expect(input.value).toBe('');

    fireEvent.click(screen.getByText('Save'));
    await waitFor(() => expect(savedBodies()).toHaveLength(1));
    const body = savedBodies()[0] as { zarinpal: { merchantId: string }; clearSecrets: string[] };
    expect(body.zarinpal.merchantId).toBe('');
    expect(body.clearSecrets).toEqual([]);
  });

  it('names the secret to empty only when the operator ticks remove', async () => {
    await renderLoaded();
    fireEvent.click(screen.getByLabelText('Remove the saved value'));
    fireEvent.click(screen.getByText('Save'));
    await waitFor(() => expect(savedBodies()).toHaveLength(1));
    expect((savedBodies()[0] as { clearSecrets: string[] }).clearSecrets).toEqual([
      'zarinpal.merchantId',
    ]);
  });
});
