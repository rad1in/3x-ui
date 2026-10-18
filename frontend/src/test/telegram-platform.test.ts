import { describe, expect, it } from 'vitest';
import { BALE_API_SERVER, botPlatformOf } from '@/pages/settings/telegramPlatform';

describe('botPlatformOf', () => {
  it('treats a blank server as Telegram and the Bale host as Bale', () => {
    expect(botPlatformOf('  ')).toBe('telegram');
    expect(botPlatformOf(BALE_API_SERVER)).toBe('bale');
    expect(botPlatformOf('https://TAPI.bale.ai/')).toBe('bale');
  });

  it('does not take a lookalike host or a malformed URL for Bale', () => {
    expect(botPlatformOf('https://tapi.bale.ai.example.com')).toBe('custom');
    expect(botPlatformOf('not a url')).toBe('custom');
  });
});
