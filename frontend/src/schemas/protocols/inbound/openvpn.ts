import { z } from 'zod';

// Clients log in with their email and this password; the backend fills an empty one.
export const OpenVPNClientSchema = z.object({
  password: z.string().default(''),
  email: z.string().min(1),
  limitIp: z.number().int().min(0).default(0),
  totalGB: z.number().int().min(0).default(0),
  expiryTime: z.number().int().default(0),
  enable: z.boolean().default(true),
  tgId: z
    .union([z.number(), z.string()])
    .transform((v) => Number(v) || 0)
    .default(0),
  subId: z.string().default(''),
  comment: z.string().default(''),
  reset: z.number().int().min(0).default(0),
  created_at: z.number().int().optional(),
  updated_at: z.number().int().optional(),
});
export type OpenVPNClient = z.infer<typeof OpenVPNClientSchema>;

export const OPENVPN_DEFAULT_DNS = ['1.1.1.1', '8.8.8.8'];

// The server generates and keeps the certificates; the form only carries them back.
const OpenVPNPkiSchema = z.object({
  ca: z.string().default(''),
  serverCert: z.string().default(''),
  serverKey: z.string().default(''),
  tlsCrypt: z.string().default(''),
});

export const OpenVPNInboundSettingsSchema = z.object({
  proto: z.enum(['udp', 'tcp']).default('udp'),
  // Empty asks the server to pick a free 10.88.N.0/24.
  network: z.string().default(''),
  dns: z.array(z.string()).default(OPENVPN_DEFAULT_DNS),
  pki: OpenVPNPkiSchema.optional(),
  clients: z.array(OpenVPNClientSchema).default([]),
});
export type OpenVPNInboundSettings = z.infer<typeof OpenVPNInboundSettingsSchema>;
