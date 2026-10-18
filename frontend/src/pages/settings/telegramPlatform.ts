export type BotPlatform = 'telegram' | 'bale' | 'custom';

export const BALE_API_SERVER = 'https://tapi.bale.ai';

// Matches the host exactly: a lookalike such as tapi.bale.ai.example.com is a custom server.
export function botPlatformOf(apiServer: string): BotPlatform {
  const raw = apiServer.trim();
  if (raw === '') return 'telegram';
  try {
    return new URL(raw).hostname.toLowerCase() === 'tapi.bale.ai' ? 'bale' : 'custom';
  } catch {
    return 'custom';
  }
}
