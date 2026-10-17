import { z } from 'zod';

export const HOSTED_PASSWORD_MIN = 8;
const GB = 1024 ** 3;

const port = z
  .number({ error: 'pages.hostedPanels.errPort' })
  .int('pages.hostedPanels.errPort')
  .min(1, 'pages.hostedPanels.errPort')
  .max(65535, 'pages.hostedPanels.errPort');
const count = z.number().int().min(0);

// Mirrors HostedPanelInput.apply and CheckHostedPanelPorts on the server.
export function hostedPanelFormSchema(original: { username: string } | null) {
  return z
    .object({
      name: z
        .string()
        .trim()
        .min(1, 'pages.hostedPanels.errName')
        .max(64, 'pages.hostedPanels.errName'),
      enable: z.boolean(),
      username: z
        .string()
        .trim()
        .min(1, 'pages.hostedPanels.errUsername')
        .max(64, 'pages.hostedPanels.errUsername')
        .regex(/^\S+$/, 'pages.hostedPanels.errUsername'),
      password: z.string(),
      webPort: port,
      subPort: port,
      webBasePath: z.string().regex(/^\/?([A-Za-z0-9._~-]+\/?)*$/, 'pages.hostedPanels.errPath'),
      portMin: port,
      portMax: port,
      maxInbounds: count,
      maxClients: count,
      trafficGB: z.number().min(0),
      expiresAt: z.number().int().min(0),
    })
    .superRefine((v, ctx) => {
      const needsPassword = original === null || v.username.trim() !== original.username;
      if ((needsPassword || v.password !== '') && v.password.length < HOSTED_PASSWORD_MIN) {
        ctx.addIssue({
          code: 'custom',
          message: 'pages.hostedPanels.errPassword',
          path: ['password'],
        });
      }
      if (v.webPort === v.subPort) {
        ctx.addIssue({
          code: 'custom',
          message: 'pages.hostedPanels.errPortsSame',
          path: ['subPort'],
        });
      }
      if (v.portMin > v.portMax) {
        ctx.addIssue({ code: 'custom', message: 'pages.hostedPanels.errRange', path: ['portMax'] });
      }
      const inRange = (p: number) => p >= v.portMin && p <= v.portMax;
      if (inRange(v.webPort) || inRange(v.subPort)) {
        ctx.addIssue({
          code: 'custom',
          message: 'pages.hostedPanels.errAddrInRange',
          path: ['webPort'],
        });
      }
    });
}

export type HostedPanelFormValues = z.infer<ReturnType<typeof hostedPanelFormSchema>>;

export function gbToBytes(gb: number): number {
  return Math.round(gb * GB);
}

export function bytesToGb(bytes: number): number {
  return Math.round((bytes / GB) * 100) / 100;
}
