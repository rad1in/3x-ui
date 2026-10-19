import { createContext, useCallback, useContext, useLayoutEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { theme as antdTheme } from 'antd';
import type { ThemeConfig } from 'antd';

import { panelBrand } from '@/lib/brand/brand';

const STORAGE_DARK = 'dark-mode';
const STORAGE_ULTRA = 'isUltraDarkThemeEnabled';

function readBool(key: string, fallback: boolean): boolean {
  const raw = localStorage.getItem(key);
  if (raw === null) return fallback;
  return raw === 'true';
}

function applyDom(isDark: boolean, isUltra: boolean) {
  document.body.classList.remove('dark', 'light');
  document.body.classList.add(isDark ? 'dark' : 'light');
  // Native scrollbars read color-scheme, not the body class.
  document.documentElement.style.colorScheme = isDark ? 'dark' : 'light';
  if (isUltra) {
    document.documentElement.setAttribute('data-theme', 'ultra-dark');
  } else {
    document.documentElement.removeAttribute('data-theme');
  }
  const msg = document.getElementById('message');
  if (msg) {
    msg.classList.remove('dark', 'light');
    msg.classList.add(isDark ? 'dark' : 'light');
  }
}

// module load so the document is in the right theme before React mounts.
const initialDark = readBool(STORAGE_DARK, true);
const initialUltra = readBool(STORAGE_ULTRA, false);
applyDom(initialDark, initialUltra);

const FONT_FAMILY =
  "ui-sans-serif, system-ui, -apple-system, 'Segoe UI Variable', 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'Noto Sans', 'Vazirmatn', 'Noto Naskh Arabic', sans-serif, 'Apple Color Emoji', 'Segoe UI Emoji'";
const BASE_TOKENS = {
  fontFamily: FONT_FAMILY,
  borderRadius: 10,
  borderRadiusLG: 16,
  borderRadiusSM: 7,
  fontWeightStrong: 600,
  controlHeight: 36,
  colorLink: '#5b4fe0',
};
const DARK_TOKENS = {
  ...BASE_TOKENS,
  colorPrimary: '#8a80f4',
  colorInfo: '#8a80f4',
  colorLink: '#a8a1f8',
  colorBgBase: '#0d0c14',
  colorBgLayout: '#0d0c14',
  colorBgContainer: '#15141e',
  colorBgElevated: '#1d1c28',
  colorBorder: 'rgba(255, 255, 255, 0.12)',
  colorBorderSecondary: 'rgba(255, 255, 255, 0.07)',
};
const ULTRA_DARK_TOKENS = {
  ...BASE_TOKENS,
  colorPrimary: '#8a80f4',
  colorInfo: '#8a80f4',
  colorLink: '#a8a1f8',
  colorBgBase: '#000',
  colorBgLayout: '#000',
  colorBgContainer: '#0f0e16',
  colorBgElevated: '#191822',
};
const DARK_LAYOUT_TOKENS = {
  bodyBg: '#0d0c14',
  headerBg: '#0d0c14',
  headerColor: '#ffffff',
  footerBg: '#0d0c14',
  siderBg: '#100f19',
  triggerBg: '#15141e',
  triggerColor: '#ffffff',
};
const ULTRA_DARK_LAYOUT_TOKENS = {
  bodyBg: '#000',
  headerBg: '#050509',
  headerColor: '#ffffff',
  footerBg: '#000',
  siderBg: '#050509',
  triggerBg: '#191822',
  triggerColor: '#ffffff',
};
const MENU_SHAPE_TOKENS = {
  itemBorderRadius: 10,
};
const DARK_MENU_TOKENS = {
  darkItemBg: '#100f19',
  darkSubMenuItemBg: '#100f19',
  darkPopupBg: '#15141e',
  ...MENU_SHAPE_TOKENS,
  darkItemSelectedBg: 'rgba(138, 128, 244, 0.18)',
  darkItemSelectedColor: '#c4bffb',
};
const ULTRA_DARK_MENU_TOKENS = {
  darkItemBg: '#050509',
  darkSubMenuItemBg: '#000',
  darkPopupBg: '#0f0e16',
  ...MENU_SHAPE_TOKENS,
  darkItemSelectedBg: 'rgba(138, 128, 244, 0.18)',
  darkItemSelectedColor: '#c4bffb',
};
const DARK_CARD_TOKENS = {
  colorBorderSecondary: 'rgba(255, 255, 255, 0.07)',
};
const ULTRA_DARK_CARD_TOKENS = {
  colorBorderSecondary: 'rgba(255, 255, 255, 0.04)',
};
const STATISTIC_TOKENS = {
  contentFontSize: 18,
  titleFontSize: 11,
};
const LIGHT_CONTRAST_TOKENS = {
  ...BASE_TOKENS,
  colorPrimary: '#5b4fe0',
  colorInfo: '#5b4fe0',
  colorBgLayout: '#f4f3fa',
  colorBorder: '#d9d7e8',
  colorBorderSecondary: '#e7e5f1',
  colorTextDescription: 'rgba(0, 0, 0, 0.58)',
  colorTextTertiary: 'rgba(0, 0, 0, 0.58)',
  colorTextPlaceholder: '#767676',
  colorError: '#cf1322',
  colorErrorText: '#cf1322',
  colorSuccessText: '#237804',
};
const LIGHT_BUTTON_TOKENS = {
  colorPrimary: '#4f43d4',
  colorPrimaryHover: '#6459e6',
  colorPrimaryActive: '#4338ca',
};
const LIGHT_MENU_TOKENS = {
  ...MENU_SHAPE_TOKENS,
  itemSelectedBg: 'rgba(91, 79, 224, 0.1)',
  itemSelectedColor: '#4338ca',
};

// hashed:false drops the `:where(.css-<hash>)` wrapper antd puts around every
// rule. It costs nothing in specificity — `:where()` contributes zero, so the
// panel's own `.ant-*` overrides still win — and it removes roughly 5,700
// wrappers, 16% of the generated stylesheet, from what the browser has to parse.
//
// cssVar.key pins the CSS-variable scope. Every panel page mounts its own
// ConfigProvider (there is no root one), and without a fixed key each mints a
// fresh useId-derived scope, so navigating re-serialises and re-injects the whole
// token block under a new class instead of reusing the one already in the head.
const SHARED_STYLE_CONFIG = {
  hashed: false,
  cssVar: { key: 'xui' },
} as const;

export function buildAntdThemeConfig(isDark: boolean, isUltra: boolean): ThemeConfig {
  const brandColor = panelBrand().color;
  const brandTokens = brandColor ? { colorPrimary: brandColor, colorInfo: brandColor } : {};
  if (!isDark) {
    return {
      ...SHARED_STYLE_CONFIG,
      algorithm: antdTheme.defaultAlgorithm,
      token: { ...LIGHT_CONTRAST_TOKENS, ...brandTokens },
      components: {
        Statistic: STATISTIC_TOKENS,
        Menu: LIGHT_MENU_TOKENS,
        // The stock-blue contrast fix would paint over a brand colour.
        Button: brandColor ? {} : LIGHT_BUTTON_TOKENS,
      },
    };
  }
  return {
    ...SHARED_STYLE_CONFIG,
    algorithm: antdTheme.darkAlgorithm,
    token: { ...(isUltra ? ULTRA_DARK_TOKENS : DARK_TOKENS), ...brandTokens },
    components: {
      Layout: isUltra ? ULTRA_DARK_LAYOUT_TOKENS : DARK_LAYOUT_TOKENS,
      Menu: isUltra ? ULTRA_DARK_MENU_TOKENS : DARK_MENU_TOKENS,
      Card: isUltra ? ULTRA_DARK_CARD_TOKENS : DARK_CARD_TOKENS,
      Statistic: STATISTIC_TOKENS,
    },
  };
}

export function pauseAnimationsUntilLeave(elementId: string): void {
  document.documentElement.setAttribute('data-theme-animations', 'off');
  const el = document.getElementById(elementId);
  if (!el) return;
  const restore = () => {
    document.documentElement.removeAttribute('data-theme-animations');
    el.removeEventListener('mouseleave', restore);
    el.removeEventListener('touchend', restore);
  };
  el.addEventListener('mouseleave', restore);
  el.addEventListener('touchend', restore);
}

interface ThemeContextValue {
  isDark: boolean;
  isUltra: boolean;
  toggleTheme: () => void;
  toggleUltra: () => void;
  antdThemeConfig: ThemeConfig;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [isDark, setIsDark] = useState<boolean>(initialDark);
  const [isUltra, setIsUltra] = useState<boolean>(initialUltra);

  useLayoutEffect(() => {
    applyDom(isDark, isUltra);
    localStorage.setItem(STORAGE_DARK, String(isDark));
    localStorage.setItem(STORAGE_ULTRA, String(isUltra));
  }, [isDark, isUltra]);

  const toggleTheme = useCallback(() => setIsDark((v) => !v), []);
  const toggleUltra = useCallback(() => setIsUltra((v) => !v), []);

  const antdThemeConfig = useMemo(() => buildAntdThemeConfig(isDark, isUltra), [isDark, isUltra]);

  const value = useMemo<ThemeContextValue>(
    () => ({ isDark, isUltra, toggleTheme, toggleUltra, antdThemeConfig }),
    [isDark, isUltra, toggleTheme, toggleUltra, antdThemeConfig],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used inside <ThemeProvider>');
  return ctx;
}
