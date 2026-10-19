import { z } from 'zod';

// Clients log in with their email and this password; the backend fills an empty one.
export const SSHClientSchema = z.object({
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
export type SSHClient = z.infer<typeof SSHClientSchema>;

export const SSHInboundSettingsSchema = z.object({
  // The server generates the host key and keeps it; the form only carries it back.
  hostKey: z.string().default(''),
  banner: z.string().default(''),
  allowLocal: z.boolean().default(false),
  clients: z.array(SSHClientSchema).default([]),
});
export type SSHInboundSettings = z.infer<typeof SSHInboundSettingsSchema>;
