import { z } from 'zod';

import { OPENVPN_DEFAULT_DNS } from './openvpn';

// Clients log in with their email and this password; the backend fills an empty one.
export const OCSERVClientSchema = z.object({
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
export type OCSERVClient = z.infer<typeof OCSERVClientSchema>;

// The server issues the certificates for serverAddress; the form only carries them back.
const OCSERVPkiSchema = z.object({
  ca: z.string().default(''),
  caKey: z.string().default(''),
  serverCert: z.string().default(''),
  serverKey: z.string().default(''),
});

export const OCSERVInboundSettingsSchema = z.object({
  serverAddress: z.string().default(''),
  // Empty asks the server to pick a free 10.92.N.0/24.
  pool: z.string().default(''),
  dns: z.array(z.string()).default(OPENVPN_DEFAULT_DNS),
  pki: OCSERVPkiSchema.optional(),
  clients: z.array(OCSERVClientSchema).default([]),
});
export type OCSERVInboundSettings = z.infer<typeof OCSERVInboundSettingsSchema>;
