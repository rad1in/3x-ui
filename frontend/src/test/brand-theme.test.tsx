import { afterEach, describe, expect, it } from 'vitest';

import { buildAntdThemeConfig } from '@/hooks/useTheme';

afterEach(() => {
  delete window.X_UI_BRAND;
});

describe('buildAntdThemeConfig with a panel brand colour', () => {
  it.each([
    ['light', false, false],
    ['dark', true, false],
    ['ultra-dark', true, true],
  ])('uses the brand colour as primary in %s mode', (_name, isDark, isUltra) => {
    window.X_UI_BRAND = { color: '#d4380d' };
    const cfg = buildAntdThemeConfig(isDark, isUltra);
    expect(cfg.token?.colorPrimary).toBe('#d4380d');
    expect(cfg.components?.Button?.colorPrimary).toBeUndefined();
  });

  it('keeps the Eternal light-mode button contrast fix without a brand', () => {
    const cfg = buildAntdThemeConfig(false, false);
    expect(cfg.token?.colorPrimary).toBe('#5b4fe0');
    expect(cfg.components?.Button?.colorPrimary).toBe('#4f43d4');
  });
});
