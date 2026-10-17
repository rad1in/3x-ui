import { z } from 'zod';

import { OPENVPN_DEFAULT_DNS } from './openvpn';

// Clients log in with their email and this password (EAP-MSCHAPv2); the backend fills an empty one.
export const IKEv2ClientSchema = z.object({
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
export type IKEv2Client = z.infer<typeof IKEv2ClientSchema>;

// The server issues the certificates for serverAddress; the form only carries them back.
const IKEv2PkiSchema = z.object({
  ca: z.string().default(''),
  caKey: z.string().default(''),
  serverCert: z.string().default(''),
  serverKey: z.string().default(''),
});

export const IKEv2InboundSettingsSchema = z.object({
  serverAddress: z.string().default(''),
  // Empty asks the server to pick a free 10.89.N.0/24.
  pool: z.string().default(''),
  dns: z.array(z.string()).default(OPENVPN_DEFAULT_DNS),
  pki: IKEv2PkiSchema.optional(),
  clients: z.array(IKEv2ClientSchema).default([]),
});
export type IKEv2InboundSettings = z.infer<typeof IKEv2InboundSettingsSchema>;
