import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor } from '@testing-library/react';

import RotationModal from '@/pages/inbounds/RotationModal';
import { DBInbound } from '@/models/dbinbound';
import { HttpUtil } from '@/utils';

import { renderWithProviders } from './test-utils';

const stored = {
  id: 1,
  inboundId: 7,
  rotatePort: true,
  portMin: 20000,
  portMax: 40000,
  rotateSni: false,
  sniList: '',
  intervalHours: 0,
  lastRotatedAt: 0,
  lastPort: 0,
  lastSni: '',
  nextRotationAt: 0,
};

function inbound(security: string) {
  return new DBInbound({
    id: 7,
    port: 443,
    listen: '',
    protocol: 'vless',
    remark: 'edge',
    enable: true,
    settings: '{"clients":[]}',
    streamSettings: JSON.stringify({ network: 'tcp', security }),
    sniffing: '',
  });
}

function mockApi() {
  vi.spyOn(HttpUtil, 'get').mockResolvedValue({ success: true, msg: '', obj: stored });
  return vi
    .spyOn(HttpUtil, 'post')
    .mockResolvedValue({ success: true, msg: '', obj: { ...stored, rotateSni: false } });
}

describe('RotationModal', () => {
  afterEach(() => vi.restoreAllMocks());

  it('offers SNI rotation only for REALITY inbounds', async () => {
    mockApi();
    const { unmount } = renderWithProviders(
      <RotationModal dbInbound={inbound('tls')} onClose={() => {}} onRotated={() => {}} />,
    );
    expect(await screen.findByText('Rotate the port within a range')).toBeTruthy();
    expect(screen.queryByText('Rotate the REALITY SNI')).toBeNull();
    unmount();

    renderWithProviders(
      <RotationModal dbInbound={inbound('reality')} onClose={() => {}} onRotated={() => {}} />,
    );
    expect(await screen.findByText('Rotate the REALITY SNI')).toBeTruthy();
  });

  it('saves the setup before rotating now', async () => {
    const post = mockApi();
    const onRotated = vi.fn();
    renderWithProviders(
      <RotationModal dbInbound={inbound('reality')} onClose={() => {}} onRotated={onRotated} />,
    );
    await screen.findByText('Rotate the port within a range');
    fireEvent.click(screen.getByRole('button', { name: 'Save and rotate now' }));

    await waitFor(() => expect(onRotated).toHaveBeenCalled());
    expect(post.mock.calls.map((c) => c[0])).toEqual([
      '/panel/api/inbounds/7/rotation',
      '/panel/api/inbounds/7/rotate',
    ]);
  });
});
