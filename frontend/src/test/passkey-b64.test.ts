import { describe, expect, it } from 'vitest';

import { b64urlToBytes, bytesToB64url } from '@/lib/passkey';

describe('passkey base64url', () => {
  it.each([[0], [1], [2], [3], [31], [32], [255]])(
    'round-trips %i bytes incl. url-unsafe values',
    (n) => {
      const bytes = new Uint8Array(new ArrayBuffer(n));
      for (let i = 0; i < n; i++) bytes[i] = (i * 37 + 251) & 0xff;
      const text = bytesToB64url(bytes.buffer);
      expect(text).not.toMatch(/[+/=]/);
      expect(Array.from(b64urlToBytes(text))).toEqual(Array.from(bytes));
    },
  );

  it('decodes the server unpadded form', () => {
    expect(Array.from(b64urlToBytes('-_8'))).toEqual([0xfb, 0xff]);
  });
});
