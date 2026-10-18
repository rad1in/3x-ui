import { z } from 'zod';

import { ADMIN_ROLES } from '@/lib/adminRole';

// Mirrors applyAdminInput on the server; a blank password on edit keeps the old one.
export function adminFormSchema(isNew: boolean) {
  return z
    .object({
      username: z
        .string()
        .trim()
        .min(1, 'pages.admins.errUsername')
        .max(64, 'pages.admins.errUsername'),
      password: z.string(),
      role: z.enum(ADMIN_ROLES),
      inboundIds: z.array(z.number().int()),
      groups: z.array(z.string()),
    })
    .superRefine((v, ctx) => {
      if (isNew && v.password === '') {
        ctx.addIssue({ code: 'custom', message: 'pages.admins.errPassword', path: ['password'] });
      }
    });
}

export type AdminFormValues = z.infer<ReturnType<typeof adminFormSchema>>;
