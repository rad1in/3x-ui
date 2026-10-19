import { z } from 'zod';

import { TRAFFIC_RESETS } from '@/schemas/primitives';

export const ClientTemplateSchema = z
  .object({
    id: z.number().int().optional(),
    name: z.string().trim().min(1),
    totalGB: z.number().min(0).default(0),
    expiryDays: z.number().int().min(0).default(0),
    delayedStart: z.boolean().default(false),
    limitIp: z.number().int().min(0).default(0),
    flow: z.string().default(''),
    group: z.string().default(''),
    comment: z.string().default(''),
    reset: z.number().int().min(0).default(0),
    trafficReset: z.enum(TRAFFIC_RESETS).default('never'),
    trafficResetDay: z.number().int().min(1).max(31).default(1),
    inboundIds: z.array(z.number().int().positive()).min(1),
  })
  .loose();
export type ClientTemplate = z.infer<typeof ClientTemplateSchema>;

export const ClientTemplateListSchema = z.array(ClientTemplateSchema);

export const ClientTemplateApplyResultSchema = z.object({
  created: z.array(z.string()),
  failed: z.record(z.string(), z.string()),
  needRestart: z.boolean(),
});
export type ClientTemplateApplyResult = z.infer<typeof ClientTemplateApplyResultSchema>;
