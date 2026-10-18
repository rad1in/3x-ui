import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';

import CleanIPScanModal from '@/pages/hosts/CleanIPScanModal';
import { HttpUtil } from '@/utils';

import { renderWithProviders } from './test-utils';

const finishedStatus = {
  running: false,
  total: 2,
  done: 2,
  startedAt: 1,
  finishedAt: 2,
  error: '',
};

const rows = [
  {
    id: 1,
    ip: '104.16.1.2',
    port: 443,
    ok: true,
    latencyMs: 84,
    lossPct: 0,
    reason: '',
    scannedAt: 2,
  },
  {
    id: 2,
    ip: '104.16.9.9',
    port: 443,
    ok: false,
    latencyMs: 0,
    lossPct: 100,
    reason: 'timeout',
    scannedAt: 2,
  },
];

function mockApi() {
  vi.spyOn(HttpUtil, 'get').mockImplementation(async (url: string) => {
    if (url.endsWith('/cleanip/status')) return { success: true, msg: '', obj: finishedStatus };
    return { success: true, msg: '', obj: rows };
  });
  return vi.spyOn(HttpUtil, 'post').mockResolvedValue({ success: true, msg: '', obj: null });
}

describe('CleanIPScanModal', () => {
  afterEach(() => vi.restoreAllMocks());

  it('applies a reachable address to the chosen host group and keeps unreachable ones disabled', async () => {
    const post = mockApi();
    renderWithProviders(
      <CleanIPScanModal
        open
        hosts={[{ groupId: 'g1', remark: 'cdn-front' } as never]}
        inboundOptions={[]}
        onClose={() => {}}
      />,
    );

    expect(await screen.findByText('104.16.1.2')).toBeTruthy();
    const buttons = screen.getAllByRole('button', { name: 'Use this address' });
    expect((buttons[1] as HTMLButtonElement).disabled).toBe(true);

    fireEvent.click(buttons[0]);
    const title = await screen.findByText('Use 104.16.1.2 for');
    const dialog = title.closest('[role="dialog"]') as HTMLElement;
    fireEvent.mouseDown(within(dialog).getByRole('combobox'));
    fireEvent.click(await screen.findByText('cdn-front'));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Use this address' }));

    await waitFor(() => expect(post).toHaveBeenCalled());
    expect(post.mock.calls[0][0]).toBe('/panel/api/hosts/cleanip/apply');
    expect(post.mock.calls[0][1]).toEqual({ ip: '104.16.1.2', mode: 'replace', groupId: 'g1' });
  });
});
