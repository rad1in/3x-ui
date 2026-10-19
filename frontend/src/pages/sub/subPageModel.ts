const DAY_MS = 86_400_000;

export type SubStatus = 'active' | 'unlimited' | 'expired' | 'depleted' | 'disabled';

export interface SubUsage {
  enabled: boolean;
  usedByte: number;
  totalByte: number;
  expireMs: number;
}

export function resolveSubStatus(sub: SubUsage, now: number): SubStatus {
  if (!sub.enabled) return 'disabled';
  if (sub.expireMs > 0 && now >= sub.expireMs) return 'expired';
  if (sub.totalByte > 0 && sub.usedByte >= sub.totalByte) return 'depleted';
  if (sub.totalByte <= 0 && sub.expireMs === 0) return 'unlimited';
  return 'active';
}

export function daysUntil(expireMs: number, now: number): number | null {
  if (expireMs <= 0) return null;
  return Math.max(0, Math.ceil((expireMs - now) / DAY_MS));
}

export function usagePercent(usedByte: number, totalByte: number): number {
  if (totalByte <= 0) return 0;
  const pct = (usedByte / totalByte) * 100;
  return Number.isFinite(pct) ? Math.min(100, Math.max(0, pct)) : 0;
}

export type AppPlatform = 'android' | 'ios' | 'windows' | 'macos' | 'linux';

export function detectPlatform(userAgent: string, maxTouchPoints = 0): AppPlatform {
  if (/iphone|ipad|ipod/i.test(userAgent)) return 'ios';
  // iPadOS sends a Macintosh UA; only its touch screen tells it apart from a Mac.
  if (/macintosh/i.test(userAgent)) return maxTouchPoints > 1 ? 'ios' : 'macos';
  if (/android/i.test(userAgent)) return 'android';
  if (/windows/i.test(userAgent)) return 'windows';
  if (/linux|x11|cros/i.test(userAgent)) return 'linux';
  return 'android';
}

export interface InstallGuide {
  app: string;
  store: string;
  url: string;
}

export const INSTALL_GUIDES: Record<AppPlatform, InstallGuide> = {
  android: {
    app: 'V2RayNG',
    store: 'Google Play',
    url: 'https://play.google.com/store/apps/details?id=com.v2ray.ang',
  },
  ios: { app: 'Streisand', store: 'App Store', url: 'https://apps.apple.com/app/id6450534064' },
  windows: {
    app: 'Hiddify',
    store: 'GitHub',
    url: 'https://github.com/hiddify/hiddify-app/releases',
  },
  macos: {
    app: 'Hiddify',
    store: 'GitHub',
    url: 'https://github.com/hiddify/hiddify-app/releases',
  },
  linux: {
    app: 'Hiddify',
    store: 'GitHub',
    url: 'https://github.com/hiddify/hiddify-app/releases',
  },
};

export interface SubApp {
  name: string;
  url: string;
}

export interface SubAppSource {
  subUrl: string;
  singboxUrl?: string;
  sId: string;
  subTitle: string;
}

export function buildSubApps({
  subUrl,
  singboxUrl = '',
  sId,
  subTitle,
}: SubAppSource): Record<AppPlatform, SubApp[]> {
  const encSub = encodeURIComponent(subUrl);
  const profileName = encodeURIComponent(subTitle || sId);

  const v2box = {
    name: 'V2Box',
    url: `v2box://install-sub?url=${encSub}&name=${encodeURIComponent(sId)}`,
  };
  // The sing-box apps only import a sing-box profile, never the raw link list.
  const singBox = singboxUrl
    ? [
        {
          name: 'Sing-box',
          url: `sing-box://import-remote-profile?url=${encodeURIComponent(singboxUrl)}#${profileName}`,
        },
      ]
    : [];
  const v2raytun = { name: 'V2RayTun', url: `v2raytun://import/${subUrl}` };
  const happ = { name: 'Happ', url: `happ://add/${subUrl}` };
  const incy = { name: 'Incy', url: `incy://add/${subUrl}` };
  const hiddify = { name: 'Hiddify', url: `hiddify://import/${subUrl}#${profileName}` };
  const karing = {
    name: 'Karing',
    url: `karing://install-config?url=${encSub}&name=${profileName}`,
  };
  const streisand = { name: 'Streisand', url: `streisand://import/${encSub}` };
  const rocketSource = `${subUrl}${subUrl.includes('?') ? '&' : '?'}flag=shadowrocket`;
  const rocketRemark = encodeURIComponent(subTitle || sId || 'Subscription');

  return {
    android: [
      v2box,
      { name: 'V2RayNG', url: `v2rayng://install-config?url=${encSub}` },
      ...singBox,
      v2raytun,
      happ,
      incy,
      hiddify,
      karing,
    ],
    ios: [
      {
        name: 'Shadowrocket',
        url: `shadowrocket://add/sub://${btoa(rocketSource)}?remark=${rocketRemark}`,
      },
      v2box,
      streisand,
      ...singBox,
      v2raytun,
      happ,
      incy,
      hiddify,
      karing,
    ],
    windows: [hiddify, karing],
    macos: [hiddify, karing, ...singBox, streisand],
    linux: [hiddify],
  };
}

// Profile routes hang under the subscription path: <sub>/<subid>/openvpn/<inbound>
// and <sub>/<subid>/ikev2/<inbound>/<format>.
export function vpnProfileUrl(
  subUrl: string,
  protocol: string,
  inboundId: number,
  format?: string,
): string {
  const base = subUrl.split('?')[0].replace(/\/+$/, '');
  const tail = format ? `/${format}` : '';
  return `${base}/${protocol}/${inboundId}${tail}`;
}
