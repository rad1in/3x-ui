import { describe, expect, it } from 'vitest';

import { BRAND_LOGO_MAX_BYTES, brandInitials, readBrandLogo } from '@/lib/brand/brand';

const PNG_1PX = Uint8Array.from(
  atob(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNgYGD4DwABBAEAwS2OUAAAAABJRU5ErkJggg==',
  ),
  (c) => c.charCodeAt(0),
);

describe('readBrandLogo', () => {
  it('turns a small PNG into the data URL the server accepts', async () => {
    const result = await readBrandLogo(new File([PNG_1PX], 'logo.png', { type: 'image/png' }));
    expect(result).toEqual({
      ok: true,
      dataUrl:
        'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNgYGD4DwABBAEAwS2OUAAAAABJRU5ErkJggg==',
    });
  });

  it('refuses SVG, which can carry script', async () => {
    const svg = new File(['<svg onload="alert(1)"/>'], 'logo.svg', { type: 'image/svg+xml' });
    expect(await readBrandLogo(svg)).toEqual({ ok: false, reason: 'type' });
  });

  it('refuses an image over the size the server will store', async () => {
    const big = new File([new Uint8Array(BRAND_LOGO_MAX_BYTES + 1)], 'logo.png', {
      type: 'image/png',
    });
    expect(await readBrandLogo(big)).toEqual({ ok: false, reason: 'size' });
  });
});

describe('brandInitials', () => {
  it.each([
    ['', '3X'],
    ['3X-UI', '3X'],
    ['Acme VPN', 'AV'],
    ['nexora-panel', 'NP'],
    ['Zed', 'ZE'],
    ['نکسورا', 'نک'],
  ])('shortens %j to %j for the collapsed rail', (name, want) => {
    expect(brandInitials(name)).toBe(want);
  });
});
