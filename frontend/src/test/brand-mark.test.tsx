import { afterEach, describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';

import BrandMark from '@/components/ui/BrandMark';

afterEach(() => {
  delete window.X_UI_BRAND;
});

describe('BrandMark', () => {
  it('shows the Eternal mark and name when nothing is configured', () => {
    const { container } = render(<BrandMark />);
    expect(container.querySelector('svg.brand-mark-logo')).not.toBeNull();
    expect(container.querySelector('.brand-mark-name')?.textContent).toBe('Eternal');
  });

  it('keeps the collapsed rail to the bare mark on the default brand', () => {
    const { container } = render(<BrandMark compact />);
    expect(container.querySelector('svg.brand-mark-logo')).not.toBeNull();
    expect(container.querySelector('.brand-mark-name')).toBeNull();
  });

  it('falls back to initials in the collapsed rail for a custom name without a logo', () => {
    window.X_UI_BRAND = { name: 'Acme VPN' };
    const { container } = render(<BrandMark compact />);
    expect(container.querySelector('svg')).toBeNull();
    expect(container.querySelector('.brand-mark-name')?.textContent).toBe('AV');
  });

  it('prefers an uploaded logo over the built-in mark', () => {
    window.X_UI_BRAND = { name: 'Acme VPN', logo: 'data:image/png;base64,AAAA' };
    const { container } = render(<BrandMark />);
    expect(container.querySelector('img.brand-mark-logo')).not.toBeNull();
    expect(container.querySelector('svg')).toBeNull();
  });
});
