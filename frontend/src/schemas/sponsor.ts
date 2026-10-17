import { z } from 'zod';

export const SPONSOR_SLOTS = ['dashboard', 'login', 'page'] as const;

// Mirrors validateSponsorRecord on the server so mistakes surface in the form.
export const SponsorFormSchema = z
  .object({
    name: z
      .string()
      .trim()
      .min(1, 'pages.sponsors.admin.errName')
      .max(64, 'pages.sponsors.admin.errName'),
    enable: z.boolean(),
    slots: z.array(z.enum(SPONSOR_SLOTS)).min(1, 'pages.sponsors.admin.errSlots'),
    from: z.number().int().min(0),
    until: z.number().int().min(0),
    title: z.string().max(120),
    text: z.string().max(300),
    link: z
      .string()
      .trim()
      .refine((v) => {
        try {
          const u = new URL(v);
          return u.protocol === 'https:' && u.host !== '';
        } catch {
          return false;
        }
      }, 'pages.sponsors.admin.errLink'),
    logo: z.string(),
  })
  .refine((v) => !(v.from > 0 && v.until > 0 && v.until <= v.from), {
    message: 'pages.sponsors.admin.errWindow',
    path: ['until'],
  });

export type SponsorFormValues = z.infer<typeof SponsorFormSchema>;
