import { describe, expect, it } from 'vitest';
import { fireEvent, screen } from '@testing-library/react';

import WorldMap from '@/pages/index/WorldMap';
import world from '@/assets/world-map.json';

import { renderWithProviders } from './test-utils';

describe('bundled world map data', () => {
  it('carries a shape for the countries clients actually connect from', () => {
    const codes = new Set(world.countries.map((c) => c.code));
    for (const code of ['US', 'DE', 'RU', 'CN', 'IR', 'BR', 'AU', 'ZA', 'IN', 'JP']) {
      expect(codes.has(code), code).toBe(true);
    }
  });
});

describe('WorldMap', () => {
  const countries = [
    { country: 'DE', clients: 7, addresses: 9 },
    { country: 'IR', clients: 2, addresses: 2 },
  ];

  it('marks only countries with clients as focusable regions with their counts', () => {
    renderWithProviders(<WorldMap countries={countries} />);
    const regions = screen.getAllByRole('img', { name: /clients/ });
    expect(regions).toHaveLength(2);
    const de = screen.getByRole('img', { name: /Germany.*7 clients.*9 addresses/ });
    expect(de.getAttribute('tabindex')).toBe('0');
    expect(de.getAttribute('data-country')).toBe('DE');
  });

  it('shades a busier country darker than a quieter one', () => {
    renderWithProviders(<WorldMap countries={countries} />);
    const opacity = (name: RegExp) =>
      Number(screen.getByRole('img', { name }).getAttribute('fill-opacity'));
    expect(opacity(/Germany/)).toBeGreaterThan(opacity(/Iran/));
  });

  it('shows the counts in a tooltip on hover', async () => {
    renderWithProviders(<WorldMap countries={countries} />);
    fireEvent.mouseEnter(screen.getByRole('img', { name: /Germany/ }));
    expect(await screen.findByText(/7 clients · 9 addresses/)).toBeTruthy();
  });
});
