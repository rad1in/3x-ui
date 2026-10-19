import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, screen } from '@testing-library/react';

import NodeJoinModal from '@/pages/nodes/NodeJoinModal';
import { HttpUtil } from '@/utils';

import { renderWithProviders } from './test-utils';

describe('NodeJoinModal', () => {
  afterEach(() => vi.restoreAllMocks());

  it('asks the master for a command with the typed node name and shows it', async () => {
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue({
      success: true,
      msg: '',
      obj: {
        command: "XUI_JOIN_TOKEN='abc' bash <(curl -Ls https://example.test/join-node.sh)",
        joinUrl: 'https://panel.example.com/abc/join/node',
        expiresAt: 1760003600000,
      },
    });
    renderWithProviders(<NodeJoinModal open onClose={() => {}} />);

    fireEvent.change(screen.getByPlaceholderText(/Node name/), { target: { value: 'de-fra-1' } });
    fireEvent.click(screen.getByRole('button', { name: 'Generate command' }));

    expect(await screen.findByDisplayValue(/XUI_JOIN_TOKEN='abc'/)).toBeTruthy();
    expect(post.mock.calls[0][0]).toBe('/panel/api/nodes/joinCommand');
    expect(post.mock.calls[0][1]).toEqual({ name: 'de-fra-1' });
  });
});
