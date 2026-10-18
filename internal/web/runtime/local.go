package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/amneziawg"
	"github.com/mhsanaei/3x-ui/v3/internal/amneziawgnet"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
	"github.com/mhsanaei/3x-ui/v3/internal/l2tp"
	"github.com/mhsanaei/3x-ui/v3/internal/mtproto"
	"github.com/mhsanaei/3x-ui/v3/internal/ocserv"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/sockkill"
	"github.com/mhsanaei/3x-ui/v3/internal/sshtunnel"
	"github.com/mhsanaei/3x-ui/v3/internal/sstp"
	"github.com/mhsanaei/3x-ui/v3/internal/tuic"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

type LocalDeps struct {
	APIPort        func() int
	SetNeedRestart func()
}

type Local struct {
	deps LocalDeps
	mu   sync.Mutex
}

func NewLocal(deps LocalDeps) *Local {
	return &Local{deps: deps}
}

func (l *Local) Name() string { return "local" }

func (l *Local) withAPI(fn func(api *xray.XrayAPI) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	port := l.deps.APIPort()
	if port <= 0 {
		return errors.New("local xray is not running")
	}
	var api xray.XrayAPI
	if err := api.Init(port); err != nil {
		return err
	}
	defer api.Close()
	return fn(&api)
}

func (l *Local) AddInbound(_ context.Context, ib *model.Inbound) error {
	if ib.Protocol == model.OCServ {
		inst, ok := ocserv.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		return ocserv.GetManager().Ensure(inst)
	}
	if ib.Protocol == model.SSTP {
		inst, ok := sstp.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		return sstp.GetManager().Ensure(inst)
	}
	if ib.Protocol == model.SSH {
		inst, ok := sshtunnel.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		return sshtunnel.GetManager().Ensure(inst)
	}
	if ib.Protocol == model.L2TP {
		inst, ok := l2tp.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		return l2tp.GetManager().Ensure(inst)
	}
	if ib.Protocol == model.IKEv2 {
		inst, ok := ipsec.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		return ipsec.GetManager().Ensure(inst)
	}
	if ib.Protocol == model.OpenVPN {
		inst, ok := openvpn.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		return openvpn.GetManager().Ensure(inst)
	}
	if ib.Protocol == model.MTProto {
		inst, ok := mtproto.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		return mtproto.GetManager().Ensure(inst)
	}
	if ib.Protocol == model.AmneziaWG {
		inst, ok := amneziawg.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		err := amneziawgnet.GetManager().Ensure(amneziawgnet.Desired{
			Instance: inst,
			Options: amneziawgnet.DeviceOptions{
				HeaderProtectionKey:    inst.Obfuscation.HeaderProtectionKey,
				ContentPaddingAddition: inst.Obfuscation.ContentPaddingAddition,
				RekeyAfterTime:         inst.Obfuscation.RekeyAfterTime,
				RekeyTimeout:           inst.Obfuscation.RekeyTimeout,
				RejectAfterTime:        inst.Obfuscation.RejectAfterTime,
				KeepaliveTimeout:       inst.Obfuscation.KeepaliveTimeout,
				MaxHandshakeAttempts:   inst.Obfuscation.MaxHandshakeAttempts,
				RandomTrailers:         inst.Obfuscation.RandomTrailers,
				DisableCookies:         inst.Obfuscation.DisableCookies,
			},
		})
		// A brand new inbound can be the first one to qualify for
		// injectAmneziawgnetSocks's Xray-side relay inbound (e.g. its first
		// valid peer). Ensure only updates the embedded Device -- flag Xray
		// for a resync so the relay actually gets created within the next
		// ApplyPendingRestart tick instead of only at the next full restart.
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		return err
	}
	if ib.Protocol == model.TUIC {
		inst, ok := tuic.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		err := tuic.GetManager().Ensure(inst)
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		return err
	}
	body, err := json.MarshalIndent(ib.GenXrayInboundConfig(), "", "  ")
	if err != nil {
		return err
	}
	return l.withAPI(func(api *xray.XrayAPI) error {
		return api.AddInbound(body)
	})
}

func (l *Local) DelInbound(_ context.Context, ib *model.Inbound) error {
	if ib.Protocol == model.OCServ {
		ocserv.GetManager().Remove(ib.Id)
		return nil
	}
	if ib.Protocol == model.SSTP {
		sstp.GetManager().Remove(ib.Id)
		return nil
	}
	if ib.Protocol == model.SSH {
		sshtunnel.GetManager().Remove(ib.Id)
		return nil
	}
	if ib.Protocol == model.L2TP {
		l2tp.GetManager().Remove(ib.Id)
		return nil
	}
	if ib.Protocol == model.IKEv2 {
		ipsec.GetManager().Remove(ib.Id)
		return nil
	}
	if ib.Protocol == model.OpenVPN {
		openvpn.GetManager().Remove(ib.Id)
		return nil
	}
	if ib.Protocol == model.MTProto {
		mtproto.GetManager().Remove(ib.Id)
		return nil
	}
	if ib.Protocol == model.AmneziaWG {
		amneziawgnet.GetManager().Remove(ib.Id)
		// The removed inbound may have been the only one backing Xray's
		// injectAmneziawgnetSocks relay inbound for this tag -- flag a
		// resync so the now-stale relay gets torn down promptly.
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		return nil
	}
	if ib.Protocol == model.TUIC {
		tuic.GetManager().Remove(ib.Id)
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		return nil
	}
	return l.withAPI(func(api *xray.XrayAPI) error {
		return api.DelInbound(ib.Tag)
	})
}

func (l *Local) UpdateInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if oldIb.Protocol == model.SSH || newIb.Protocol == model.SSH {
		return l.updateManagedInbound(ctx, model.SSH, oldIb, newIb)
	}
	if oldIb.Protocol == model.SSTP || newIb.Protocol == model.SSTP {
		return l.updateManagedInbound(ctx, model.SSTP, oldIb, newIb)
	}
	if oldIb.Protocol == model.OCServ || newIb.Protocol == model.OCServ {
		return l.updateManagedInbound(ctx, model.OCServ, oldIb, newIb)
	}
	if oldIb.Protocol == model.OpenVPN || newIb.Protocol == model.OpenVPN {
		return l.updateOpenVPNInbound(ctx, oldIb, newIb)
	}
	if oldIb.Protocol == model.IKEv2 || newIb.Protocol == model.IKEv2 {
		return l.updateIKEv2Inbound(ctx, oldIb, newIb)
	}
	if oldIb.Protocol == model.L2TP || newIb.Protocol == model.L2TP {
		return l.updateL2TPInbound(ctx, oldIb, newIb)
	}
	if oldIb.Protocol == model.MTProto || newIb.Protocol == model.MTProto {
		return l.updateMtprotoInbound(ctx, oldIb, newIb)
	}
	if oldIb.Protocol == model.AmneziaWG || newIb.Protocol == model.AmneziaWG {
		return l.updateAmneziaWGInbound(ctx, oldIb, newIb)
	}
	if oldIb.Protocol == model.TUIC || newIb.Protocol == model.TUIC {
		return l.updateTuicInbound(ctx, oldIb, newIb)
	}
	_ = l.DelInbound(ctx, oldIb)
	if !newIb.Enable {
		return nil
	}
	return l.AddInbound(ctx, newIb)
}

// updateOpenVPNInbound lets Ensure apply an account change over the management
// socket, so a client edit never restarts the server and drops everyone.
func (l *Local) updateOpenVPNInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if oldIb.Protocol != model.OpenVPN {
		_ = l.DelInbound(ctx, oldIb)
	}
	if newIb.Protocol != model.OpenVPN {
		openvpn.GetManager().Remove(oldIb.Id)
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	inst, ok := openvpn.InstanceFromInbound(newIb)
	if !newIb.Enable || !ok {
		openvpn.GetManager().Remove(newIb.Id)
		return nil
	}
	return openvpn.GetManager().Ensure(inst)
}

// updateIKEv2Inbound lets Ensure load only what changed into charon, so a
// client edit never drops the other users' sessions.
func (l *Local) updateIKEv2Inbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if oldIb.Protocol != model.IKEv2 {
		_ = l.DelInbound(ctx, oldIb)
	}
	if newIb.Protocol != model.IKEv2 {
		ipsec.GetManager().Remove(oldIb.Id)
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	inst, ok := ipsec.InstanceFromInbound(newIb)
	if !newIb.Enable || !ok {
		ipsec.GetManager().Remove(newIb.Id)
		return nil
	}
	return ipsec.GetManager().Ensure(inst)
}

// updateL2TPInbound rewrites the logins and ends revoked links without
// restarting xl2tpd, so a client edit leaves everyone else connected.
func (l *Local) updateL2TPInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if oldIb.Protocol != model.L2TP {
		_ = l.DelInbound(ctx, oldIb)
	}
	if newIb.Protocol != model.L2TP {
		l2tp.GetManager().Remove(oldIb.Id)
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	inst, ok := l2tp.InstanceFromInbound(newIb)
	if !newIb.Enable || !ok {
		l2tp.GetManager().Remove(newIb.Id)
		return nil
	}
	return l2tp.GetManager().Ensure(inst)
}

// updateManagedInbound applies an update to a protocol served by a panel-side
// manager: Ensure swaps accounts in place, so a client edit does not drop
// everyone else's connections.
func (l *Local) updateManagedInbound(ctx context.Context, proto model.Protocol, oldIb, newIb *model.Inbound) error {
	if oldIb.Protocol != proto || newIb.Protocol != proto {
		_ = l.DelInbound(ctx, oldIb)
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	if !newIb.Enable || !managedServesAnyone(newIb) {
		return l.DelInbound(ctx, newIb)
	}
	return l.AddInbound(ctx, newIb)
}

func managedServesAnyone(ib *model.Inbound) bool {
	switch ib.Protocol {
	case model.SSH:
		_, ok := sshtunnel.InstanceFromInbound(ib)
		return ok
	case model.SSTP:
		_, ok := sstp.InstanceFromInbound(ib)
		return ok
	case model.OCServ:
		_, ok := ocserv.InstanceFromInbound(ib)
		return ok
	}
	return false
}

// updateMtprotoInbound applies an inbound update without the Del+Add sequence
// the xray path uses: Remove would drop the manager's fingerprint state, which
// is what lets Ensure keep the running mtg process (and its live connections)
// when nothing in the generated config changed. The sidecar is only stopped
// when the inbound is disabled, loses its last active secret, or moves to a
// different protocol.
func (l *Local) updateMtprotoInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if oldIb.Protocol == model.MTProto && newIb.Protocol != model.MTProto {
		mtproto.GetManager().Remove(oldIb.Id)
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	if oldIb.Protocol != model.MTProto {
		_ = l.DelInbound(ctx, oldIb)
	}
	if !newIb.Enable {
		mtproto.GetManager().Remove(newIb.Id)
		return nil
	}
	inst, ok := mtproto.InstanceFromInbound(newIb)
	if !ok {
		mtproto.GetManager().Remove(newIb.Id)
		return nil
	}
	return mtproto.GetManager().Ensure(inst)
}

// updateAmneziaWGInbound mirrors updateMtprotoInbound: it skips the
// Remove+Ensure sequence a plain Del+Add would force so that, on an
// AmneziaWG-to-AmneziaWG edit, Manager.Ensure's own fingerprint comparison
// can reconfigure the running embedded Device in place via IpcSet instead
// of always rebuilding it (see internal/amneziawgnet.Manager.ensureLocked --
// only an address or effective-MTU change forces a rebuild there, S4
// included, not a peer edit).
//
// Every exit path below only touches the embedded Device via
// amneziawgnet.GetManager() -- none of it rebuilds Xray's own config, which
// is what actually creates/removes injectAmneziawgnetSocks's relay inbound.
// A peer edit that changes whether this inbound has a qualifying peer at
// all (its first peer added, or its last one removed) must still get that
// relay created or torn down, so flag Xray for a resync unconditionally
// here rather than trying to enumerate which of the branches below need it.
func (l *Local) updateAmneziaWGInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if l.deps.SetNeedRestart != nil {
		l.deps.SetNeedRestart()
	}
	if oldIb.Protocol == model.AmneziaWG && newIb.Protocol != model.AmneziaWG {
		amneziawgnet.GetManager().Remove(oldIb.Id)
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	if oldIb.Protocol != model.AmneziaWG {
		_ = l.DelInbound(ctx, oldIb)
	}
	if !newIb.Enable {
		amneziawgnet.GetManager().Remove(newIb.Id)
		return nil
	}
	inst, ok := amneziawg.InstanceFromInbound(newIb)
	if !ok {
		amneziawgnet.GetManager().Remove(newIb.Id)
		return nil
	}
	return amneziawgnet.GetManager().Ensure(amneziawgnet.Desired{
		Instance: inst,
		Options: amneziawgnet.DeviceOptions{
			HeaderProtectionKey:    inst.Obfuscation.HeaderProtectionKey,
			ContentPaddingAddition: inst.Obfuscation.ContentPaddingAddition,
			RekeyAfterTime:         inst.Obfuscation.RekeyAfterTime,
			RekeyTimeout:           inst.Obfuscation.RekeyTimeout,
			RejectAfterTime:        inst.Obfuscation.RejectAfterTime,
			KeepaliveTimeout:       inst.Obfuscation.KeepaliveTimeout,
			MaxHandshakeAttempts:   inst.Obfuscation.MaxHandshakeAttempts,
			RandomTrailers:         inst.Obfuscation.RandomTrailers,
			DisableCookies:         inst.Obfuscation.DisableCookies,
		},
	})
}

func (l *Local) updateTuicInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if oldIb.Protocol == model.TUIC && newIb.Protocol != model.TUIC {
		tuic.GetManager().Remove(oldIb.Id)
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	if oldIb.Protocol != model.TUIC {
		_ = l.DelInbound(ctx, oldIb)
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
	}
	if oldIb.Protocol == model.TUIC && newIb.Protocol == model.TUIC && oldIb.Enable && newIb.Enable && oldIb.Tag != newIb.Tag && l.deps.SetNeedRestart != nil {
		l.deps.SetNeedRestart()
	}
	if !newIb.Enable {
		tuic.GetManager().Remove(newIb.Id)
		if oldIb.Enable && l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		return nil
	}
	if !oldIb.Enable && newIb.Enable {
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
	}
	inst, ok := tuic.InstanceFromInbound(newIb)
	if !ok {
		tuic.GetManager().Remove(newIb.Id)
		return nil
	}
	return tuic.GetManager().Ensure(inst)
}

// servedBySidecar: these protocols run outside Xray and take client changes by
// re-applying the whole inbound, so Xray's per-user ops do not apply to them.
func servedBySidecar(p model.Protocol) bool {
	return p == model.MTProto || p == model.AmneziaWG || p == model.TUIC || p == model.OpenVPN || p == model.IKEv2 || p == model.L2TP || p == model.SSH || p == model.SSTP || p == model.OCServ
}

func (l *Local) AddUser(_ context.Context, ib *model.Inbound, userMap map[string]any) error {
	if servedBySidecar(ib.Protocol) {
		return nil
	}
	return l.withAPI(func(api *xray.XrayAPI) error {
		return api.AddUser(string(ib.Protocol), ib.Tag, userMap)
	})
}

func (l *Local) RemoveUser(_ context.Context, ib *model.Inbound, email string) error {
	if servedBySidecar(ib.Protocol) {
		return nil
	}
	return l.withAPI(func(api *xray.XrayAPI) error {
		return api.RemoveUser(ib.Tag, email)
	})
}

func (l *Local) AddClient(ctx context.Context, ib *model.Inbound, client model.Client) error {
	if !client.Enable {
		return nil
	}
	user := map[string]any{
		"email":        client.Email,
		"id":           client.ID,
		"security":     client.Security,
		"flow":         client.Flow,
		"auth":         client.Auth,
		"password":     client.Password,
		"publicKey":    client.PublicKey,
		"allowedIPs":   client.AllowedIPs,
		"preSharedKey": client.PreSharedKey,
		"keepAlive":    wgKeepAlive(client.KeepAliveSeconds()),
	}
	return l.AddUser(ctx, ib, user)
}

func (l *Local) DeleteUser(ctx context.Context, ib *model.Inbound, email string) error {
	if email == "" {
		return nil
	}
	if err := l.RemoveUser(ctx, ib, email); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return err
	}
	return nil
}

func (l *Local) DeleteClient(context.Context, string) error {
	return nil
}

func (l *Local) UpdateUser(ctx context.Context, ib *model.Inbound, oldEmail string, payload model.Client) error {
	if oldEmail != "" {
		if err := l.RemoveUser(ctx, ib, oldEmail); err != nil && !strings.Contains(err.Error(), "not found") {
			return err
		}
	}
	if !payload.Enable {
		return nil
	}
	user := map[string]any{
		"email":        payload.Email,
		"id":           payload.ID,
		"security":     payload.Security,
		"flow":         payload.Flow,
		"auth":         payload.Auth,
		"password":     payload.Password,
		"publicKey":    payload.PublicKey,
		"allowedIPs":   payload.AllowedIPs,
		"preSharedKey": payload.PreSharedKey,
		"keepAlive":    wgKeepAlive(payload.KeepAliveSeconds()),
	}
	return l.AddUser(ctx, ib, user)
}

func wgKeepAlive(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	return strconv.Itoa(seconds)
}

// ErrDisconnectUnsupported marks protocols whose live sessions cannot be ended
// one client at a time: UDP transports and sidecars without a kick command.
var ErrDisconnectUnsupported = errors.New("live connections of this protocol cannot be disconnected")

func (l *Local) DisconnectClient(ctx context.Context, ib *model.Inbound, email string, ips []string) (int, error) {
	switch ib.Protocol {
	case model.OpenVPN:
		return openvpn.GetManager().KickUser(email), nil
	case model.IKEv2:
		return ipsec.GetManager().KickUser(email), nil
	case model.L2TP:
		return l2tp.GetManager().KickUser(email), nil
	case model.SSH:
		return sshtunnel.GetManager().KickUser(email), nil
	case model.SSTP:
		return sstp.GetManager().KickUser(email), nil
	case model.OCServ:
		return ocserv.GetManager().KickUser(email), nil
	case model.VLESS, model.VMESS, model.Trojan, model.Shadowsocks:
		return sockkill.Kill(ctx, ib.Port, ips)
	default:
		return 0, ErrDisconnectUnsupported
	}
}

func (l *Local) RestartXray(_ context.Context) error {
	if l.deps.SetNeedRestart != nil {
		l.deps.SetNeedRestart()
	}
	return nil
}

func (l *Local) ResetClientTraffic(_ context.Context, _ *model.Inbound, _ string) error {
	return nil
}

func (l *Local) ResetAllTraffics(_ context.Context) error {
	return nil
}

func (l *Local) ResetInboundTraffic(_ context.Context, _ *model.Inbound) error {
	return nil
}
