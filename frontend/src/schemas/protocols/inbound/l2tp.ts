import { z } from 'zod';

import { OPENVPN_DEFAULT_DNS } from './openvpn';

// Clients log in with their email and this password (MS-CHAPv2 over PPP); the backend fills an empty one.
export const L2TPClientSchema = z.object({
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
export type L2TPClient = z.infer<typeof L2TPClientSchema>;

export const L2TPInboundSettingsSchema = z.object({
  // Empty asks the server to generate one; the same key is shared by every client.
  psk: z.string().default(''),
  // Empty asks the server to pick a free 10.90.N.0/24.
  pool: z.string().default(''),
  dns: z.array(z.string()).default(OPENVPN_DEFAULT_DNS),
  clients: z.array(L2TPClientSchema).default([]),
});
export type L2TPInboundSettings = z.infer<typeof L2TPInboundSettingsSchema>;
