import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import PortProbeSettings from '@/pages/settings/PortProbeSettings';
import { AllSetting } from '@/models/setting';

function setting(patch: Partial<AllSetting>) {
  return Object.assign(new AllSetting(), patch);
}

describe('PortProbeSettings', () => {
  it('shows only the switch while the probe is off', () => {
    render(<PortProbeSettings allSetting={setting({})} updateSetting={vi.fn()} />);
    expect(screen.getByRole('switch')).toBeTruthy();
    expect(screen.queryByText('Probe URL')).toBeNull();
  });

  it('asks for the expected text only once a probe URL is set, and saves the typed values', () => {
    const update = vi.fn();
    const { rerender } = render(
      <PortProbeSettings allSetting={setting({ portProbeEnable: true })} updateSetting={update} />,
    );
    expect(screen.getByText('Probe URL')).toBeTruthy();
    expect(screen.queryByText('Expected text')).toBeNull();

    fireEvent.change(screen.getByPlaceholderText(/checker\.example/), {
      target: { value: 'https://c.example/?h={host}' },
    });
    expect(update).toHaveBeenCalledWith({ portProbeUrl: 'https://c.example/?h={host}' });

    rerender(
      <PortProbeSettings
        allSetting={setting({ portProbeEnable: true, portProbeUrl: 'https://c.example/?h={host}' })}
        updateSetting={update}
      />,
    );
    expect(screen.getByText('Expected text')).toBeTruthy();
  });
});
