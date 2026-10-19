// Browser side of WebAuthn: the server speaks JSON with unpadded base64url
// binary fields, the browser API wants ArrayBuffers.

export interface PasskeyCredentialPayload {
  id: string;
  response: Record<string, string>;
}

export function passkeysSupported(): boolean {
  return typeof window !== 'undefined' && !!window.PublicKeyCredential && !!navigator.credentials;
}

export function b64urlToBytes(value: string): Uint8Array<ArrayBuffer> {
  const padded = value
    .replace(/-/g, '+')
    .replace(/_/g, '/')
    .padEnd(Math.ceil(value.length / 4) * 4, '=');
  const bin = atob(padded);
  const out = new Uint8Array(new ArrayBuffer(bin.length));
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export function bytesToB64url(buf: ArrayBuffer): string {
  let bin = '';
  for (const b of new Uint8Array(buf)) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

interface DescriptorJSON {
  type: PublicKeyCredentialType;
  id: string;
}

function descriptors(list: DescriptorJSON[] | undefined): PublicKeyCredentialDescriptor[] {
  return (list ?? []).map((d) => ({ type: d.type, id: b64urlToBytes(d.id) }));
}

export interface CreationOptionsJSON {
  rp: PublicKeyCredentialRpEntity;
  user: { id: string; name: string; displayName: string };
  challenge: string;
  pubKeyCredParams: PublicKeyCredentialParameters[];
  timeout?: number;
  excludeCredentials?: DescriptorJSON[];
  authenticatorSelection?: AuthenticatorSelectionCriteria;
  attestation?: AttestationConveyancePreference;
}

export interface RequestOptionsJSON {
  challenge: string;
  rpId: string;
  timeout?: number;
  userVerification?: UserVerificationRequirement;
  allowCredentials?: DescriptorJSON[];
}

export async function createPasskey(opts: CreationOptionsJSON): Promise<PasskeyCredentialPayload> {
  const cred = (await navigator.credentials.create({
    publicKey: {
      ...opts,
      challenge: b64urlToBytes(opts.challenge),
      user: { ...opts.user, id: b64urlToBytes(opts.user.id) },
      excludeCredentials: descriptors(opts.excludeCredentials),
    },
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error('cancelled');
  const r = cred.response as AuthenticatorAttestationResponse;
  return {
    id: bytesToB64url(cred.rawId),
    response: {
      clientDataJSON: bytesToB64url(r.clientDataJSON),
      attestationObject: bytesToB64url(r.attestationObject),
    },
  };
}

export async function getPasskey(opts: RequestOptionsJSON): Promise<PasskeyCredentialPayload> {
  const cred = (await navigator.credentials.get({
    publicKey: {
      ...opts,
      challenge: b64urlToBytes(opts.challenge),
      allowCredentials: descriptors(opts.allowCredentials),
    },
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error('cancelled');
  const r = cred.response as AuthenticatorAssertionResponse;
  return {
    id: bytesToB64url(cred.rawId),
    response: {
      clientDataJSON: bytesToB64url(r.clientDataJSON),
      authenticatorData: bytesToB64url(r.authenticatorData),
      signature: bytesToB64url(r.signature),
      userHandle: r.userHandle ? bytesToB64url(r.userHandle) : '',
    },
  };
}
