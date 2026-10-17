// Mirrors entity.PanelBrandLogoMaxBytes on the server.
export const BRAND_LOGO_MAX_BYTES = 64 * 1024;
export const DEFAULT_BRAND_NAME = '3X-UI';

const ALLOWED_LOGO_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp']);

export interface PanelBrand {
  name: string;
  color: string;
  logo: string;
}

export function panelBrand(): PanelBrand {
  const injected = window.X_UI_BRAND;
  return {
    name: injected?.name || '',
    color: injected?.color || '',
    logo: injected?.logo || '',
  };
}

export type BrandLogoResult =
  | { ok: true; dataUrl: string }
  | { ok: false; reason: 'type' | 'size' };

export async function readBrandLogo(file: File): Promise<BrandLogoResult> {
  if (!ALLOWED_LOGO_TYPES.has(file.type)) return { ok: false, reason: 'type' };
  if (file.size > BRAND_LOGO_MAX_BYTES) return { ok: false, reason: 'size' };
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return { ok: true, dataUrl: `data:${file.type};base64,${btoa(binary)}` };
}

export function brandInitials(name: string): string {
  const trimmed = name.trim();
  if (!trimmed || trimmed === DEFAULT_BRAND_NAME) return '3X';
  const words = trimmed.split(/[\s\-_.]+/).filter(Boolean);
  const letters =
    words.length >= 2 ? [...words[0]][0] + [...words[1]][0] : [...words[0]].slice(0, 2).join('');
  return letters.toUpperCase();
}
