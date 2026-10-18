import { z } from 'zod';

import { AmneziawgInboundSettingsSchema } from './amneziawg';
import { HttpInboundSettingsSchema } from './http';
import { HysteriaInboundSettingsSchema } from './hysteria';
import { MasqueInboundSettingsSchema } from './masque';
import { OpenVPNInboundSettingsSchema } from './openvpn';
import { IKEv2InboundSettingsSchema } from './ikev2';
import { L2TPInboundSettingsSchema } from './l2tp';
import { SSHInboundSettingsSchema } from './ssh';
import { MixedInboundSettingsSchema } from './mixed';
import { MtprotoInboundSettingsSchema } from './mtproto';
import { ShadowsocksInboundSettingsSchema } from './shadowsocks';
import { TrojanInboundSettingsSchema } from './trojan';
import { TuicInboundSettingsSchema } from './tuic';
import { TunInboundSettingsSchema } from './tun';
import { TunnelInboundSettingsSchema } from './tunnel';
import { VlessInboundSettingsSchema } from './vless';
import { VmessInboundSettingsSchema } from './vmess';
import { WireguardInboundSettingsSchema } from './wireguard';

export * from './amneziawg';
export * from './http';
export * from './hysteria';
export * from './masque';
export * from './openvpn';
export * from './ikev2';
export * from './l2tp';
export * from './ssh';
export * from './mixed';
export * from './mtproto';
export * from './shadowsocks';
export * from './trojan';
export * from './tuic';
export * from './tun';
export * from './tunnel';
export * from './vless';
export * from './vmess';
export * from './wireguard';

// Tagged-wrapper discriminated union. The discriminator (`protocol`) lives on
// the wrapper, not inside `settings`, mirroring the wire format Xray emits:
//   { protocol: 'vless', settings: { clients: [...], ... }, ... }
// Consumers narrow on `.protocol` and TypeScript narrows `.settings` to the
// matching leaf type.
export const InboundSettingsSchema = z.discriminatedUnion('protocol', [
  z.object({ protocol: z.literal('vmess'), settings: VmessInboundSettingsSchema }),
  z.object({ protocol: z.literal('vless'), settings: VlessInboundSettingsSchema }),
  z.object({ protocol: z.literal('trojan'), settings: TrojanInboundSettingsSchema }),
  z.object({ protocol: z.literal('shadowsocks'), settings: ShadowsocksInboundSettingsSchema }),
  z.object({ protocol: z.literal('wireguard'), settings: WireguardInboundSettingsSchema }),
  z.object({ protocol: z.literal('hysteria'), settings: HysteriaInboundSettingsSchema }),
  z.object({ protocol: z.literal('http'), settings: HttpInboundSettingsSchema }),
  z.object({ protocol: z.literal('mixed'), settings: MixedInboundSettingsSchema }),
  z.object({ protocol: z.literal('tunnel'), settings: TunnelInboundSettingsSchema }),
  z.object({ protocol: z.literal('tun'), settings: TunInboundSettingsSchema }),
  z.object({ protocol: z.literal('mtproto'), settings: MtprotoInboundSettingsSchema }),
  z.object({ protocol: z.literal('amneziawg'), settings: AmneziawgInboundSettingsSchema }),
  z.object({ protocol: z.literal('tuic'), settings: TuicInboundSettingsSchema }),
  z.object({ protocol: z.literal('masque'), settings: MasqueInboundSettingsSchema }),
  z.object({ protocol: z.literal('openvpn'), settings: OpenVPNInboundSettingsSchema }),
  z.object({ protocol: z.literal('ikev2'), settings: IKEv2InboundSettingsSchema }),
  z.object({ protocol: z.literal('l2tp'), settings: L2TPInboundSettingsSchema }),
  z.object({ protocol: z.literal('ssh'), settings: SSHInboundSettingsSchema }),
]);
export type InboundSettings = z.infer<typeof InboundSettingsSchema>;
