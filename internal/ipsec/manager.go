// Package ipsec runs the IKEv2 inbounds on one host-wide strongSwan charon,
// configured over its vici socket: the panel decides every login (EAP secrets
// are loaded per account) and meters every byte from the daemon's SA counters.
package ipsec

import (
	"context"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/strongswan/govici/vici"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/vpnnat"
)

// Traffic is one user's bytes on one inbound since the last collection.
type Traffic struct {
	Tag   string
	Email string
	Up    int64
	Down  int64
}

type childSA struct {
	id      uint64
	in, out int64
}

type ikeSA struct {
	conn        string
	id          uint64
	established int64 // seconds ago
	user        string
	children    []childSA
}

// Manager owns the charon daemon and everything loaded into it.
type Manager struct {
	mu       sync.Mutex
	want     map[int]Instance
	wantL2TP map[int]L2TPConn
	l2tpErr  string
	d        *daemon
	libipsec *bool
	startErr string

	insts        map[string]Instance // by connection name
	loadedConns  map[string]string
	loadedPools  map[string]bool
	loadedShared map[string]string
	loadedKeys   string
	routed       map[netip.Prefix]bool

	acct accounting
}

var (
	managerOnce sync.Once
	manager     *Manager
)

func GetManager() *Manager {
	managerOnce.Do(func() { manager = newManager() })
	return manager
}

func newManager() *Manager {
	m := &Manager{want: map[int]Instance{}, wantL2TP: map[int]L2TPConn{}, routed: map[netip.Prefix]bool{}}
	m.resetLoaded()
	m.acct.reset()
	return m
}

func (m *Manager) resetLoaded() {
	m.insts = map[string]Instance{}
	m.loadedConns = map[string]string{}
	m.loadedPools = map[string]bool{}
	m.loadedShared = map[string]string{}
	m.loadedKeys = ""
}

// Ensure makes inst one of the served inbounds, applying a user change without
// touching the other sessions.
func (m *Manager) Ensure(inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.want[inst.Id] = inst
	return m.applyWantLocked()
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.want, id)
	if err := m.applyWantLocked(); err != nil {
		logger.Warning("ipsec: remove inbound", id, err)
	}
}

// Reconcile makes the daemon serve exactly insts, starting or stopping it as
// needed, and drops sessions of users who lost access or changed password.
func (m *Manager) Reconcile(insts []Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.want = map[int]Instance{}
	for _, inst := range insts {
		m.want[inst.Id] = inst
	}
	return m.applyWantLocked()
}

// EnsureL2TP adds the IPsec side of an L2TP inbound; L2TPUnavailable says
// when this kernel cannot carry it.
func (m *Manager) EnsureL2TP(c L2TPConn) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wantL2TP[c.Id] = c
	return m.applyWantLocked()
}

func (m *Manager) RemoveL2TP(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.wantL2TP, id)
	if err := m.applyWantLocked(); err != nil {
		logger.Warning("ipsec: remove l2tp", id, err)
	}
}

// L2TPUnavailable is why the L2TP connections are not loaded, or "".
func (m *Manager) L2TPUnavailable() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.l2tpErr
}

func (m *Manager) applyWantLocked() error {
	insts := make([]Instance, 0, len(m.want))
	for _, inst := range m.want {
		insts = append(insts, inst)
	}
	sort.Slice(insts, func(i, j int) bool { return insts[i].Id < insts[j].Id })
	l2tps := make([]L2TPConn, 0, len(m.wantL2TP))
	for _, c := range m.wantL2TP {
		l2tps = append(l2tps, c)
	}
	if len(insts) == 0 && len(l2tps) == 0 {
		m.l2tpErr = ""
		m.stopLocked()
		return nil
	}
	if err := m.ensureDaemonLocked(); err != nil {
		return err
	}
	m.l2tpErr = ""
	if len(l2tps) > 0 && *m.libipsec {
		// libipsec only does tunnel mode; L2TP/IPsec needs transport mode.
		m.l2tpErr = "L2TP/IPsec needs IPsec (ESP) support in the kernel, which this server lacks"
		l2tps = nil
	}
	want := desiredOf(insts, l2tps)
	changed, err := m.applyLocked(want)
	if err != nil {
		return err
	}
	m.insts = want.byConn
	m.routeLocked(insts)
	sas, err := m.listSAsLocked()
	if err != nil {
		return err
	}
	for _, id := range m.revokedLocked(sas, changed) {
		m.terminateLocked(id)
	}
	for _, id := range overLimit(sas, m.insts) {
		m.terminateLocked(id)
	}
	return nil
}

func (m *Manager) ensureDaemonLocked() error {
	if m.d != nil && m.d.running() {
		return nil
	}
	if m.d != nil {
		logger.Warning("ipsec: charon exited:", m.d.last())
		m.d.stop()
		m.d = nil
	}
	if m.libipsec == nil {
		userspace := !kernelESP()
		m.libipsec = &userspace
		if userspace {
			logger.Info("ipsec: the kernel has no ESP support; running ESP in userspace")
		}
	}
	d, err := startDaemon(*m.libipsec)
	if err != nil {
		m.startErr = err.Error()
		return err
	}
	m.startErr = ""
	m.d = d
	m.resetLoaded()
	m.acct.reset()
	go m.acct.listen(d)
	logger.Info("ipsec: charon started")
	return nil
}

// applyLocked loads what changed and returns the EAP identities whose secret
// was replaced or removed, so their live sessions can be dropped.
func (m *Manager) applyLocked(want desired) (map[string]bool, error) {
	changed := map[string]bool{}
	if keys := strings.Join(want.keys, "\n"); keys != m.loadedKeys {
		// clear-creds also drops the shared secrets, so they are all reloaded below.
		if _, err := m.d.call("clear-creds", nil); err != nil {
			return nil, err
		}
		for _, k := range want.keys {
			msg := vici.NewMessage()
			_ = msg.Set("type", "any")
			_ = msg.Set("data", k)
			if _, err := m.d.call("load-key", msg); err != nil {
				return nil, err
			}
		}
		m.loadedKeys = keys
		m.loadedShared = map[string]string{}
	}
	for id, s := range want.shared {
		fp := s.fingerprint()
		prev, had := m.loadedShared[id]
		if prev == fp {
			continue
		}
		msg := vici.NewMessage()
		_ = msg.Set("id", id)
		_ = msg.Set("type", s.Type)
		_ = msg.Set("data", s.Data)
		_ = msg.Set("owners", s.Owners)
		if _, err := m.d.call("load-shared", msg); err != nil {
			return nil, err
		}
		if had && strings.HasPrefix(id, "eap-") {
			changed[strings.TrimPrefix(id, "eap-")] = true
		}
		m.loadedShared[id] = fp
	}
	for id := range m.loadedShared {
		if _, ok := want.shared[id]; ok {
			continue
		}
		msg := vici.NewMessage()
		_ = msg.Set("id", id)
		if _, err := m.d.call("unload-shared", msg); err != nil {
			logger.Debug("ipsec: unload secret", id, err)
		}
		delete(m.loadedShared, id)
		if strings.HasPrefix(id, "eap-") {
			changed[strings.TrimPrefix(id, "eap-")] = true
		}
	}
	for name, pool := range want.pools {
		if m.loadedPools[name] {
			continue
		}
		msg := vici.NewMessage()
		_ = msg.Set(name, pool)
		if _, err := m.d.call("load-pool", msg); err != nil {
			return nil, err
		}
		m.loadedPools[name] = true
	}
	for name, conn := range want.conns {
		fp := conn.String()
		if m.loadedConns[name] == fp {
			continue
		}
		msg := vici.NewMessage()
		_ = msg.Set(name, conn)
		if _, err := m.d.call("load-conn", msg); err != nil {
			return nil, err
		}
		m.loadedConns[name] = fp
	}
	for name := range m.loadedConns {
		if _, ok := want.conns[name]; ok {
			continue
		}
		msg := vici.NewMessage()
		_ = msg.Set("name", name)
		if _, err := m.d.call("unload-conn", msg); err != nil {
			logger.Debug("ipsec: unload connection", name, err)
		}
		delete(m.loadedConns, name)
	}
	for name := range m.loadedPools {
		if _, ok := want.pools[name]; ok {
			continue
		}
		msg := vici.NewMessage()
		_ = msg.Set("name", name)
		// A pool with leases out stays until its clients are gone; retried next time.
		if _, err := m.d.call("unload-pool", msg); err == nil {
			delete(m.loadedPools, name)
		}
	}
	return changed, nil
}

func (m *Manager) routeLocked(insts []Instance) {
	want := map[netip.Prefix]bool{}
	for _, inst := range insts {
		p := inst.Pool.Masked()
		want[p] = true
		if !m.routed[p] {
			if err := vpnnat.Default.Up(p); err != nil {
				logger.Warning("ipsec: routing for", p, "not set up:", err)
			}
			m.routed[p] = true
		}
	}
	for p := range m.routed {
		if !want[p] {
			vpnnat.Default.Down(p)
			delete(m.routed, p)
		}
	}
}

// revokedLocked lists the sessions whose login is no longer accepted as it was.
func (m *Manager) revokedLocked(sas []ikeSA, changed map[string]bool) []uint64 {
	var ids []uint64
	for _, sa := range sas {
		if !strings.HasPrefix(sa.conn, "ikev2-") {
			continue
		}
		inst, ok := m.insts[sa.conn]
		if !ok {
			ids = append(ids, sa.id)
			continue
		}
		if _, ok := inst.Users[sa.user]; !ok || changed[sa.user] {
			ids = append(ids, sa.id)
		}
	}
	return ids
}

// overLimit returns the sessions to close so no user exceeds MaxConns; the
// newest go first, so a second device cannot push the first one off.
func overLimit(sas []ikeSA, insts map[string]Instance) []uint64 {
	byUser := map[string][]ikeSA{}
	for _, sa := range sas {
		inst, ok := insts[sa.conn]
		if !ok || inst.Users[sa.user].MaxConns <= 0 {
			continue
		}
		key := sa.conn + "\x00" + sa.user
		byUser[key] = append(byUser[key], sa)
	}
	var ids []uint64
	for _, group := range byUser {
		limit := insts[group[0].conn].Users[group[0].user].MaxConns
		if len(group) <= limit {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].established > group[j].established })
		for _, sa := range group[limit:] {
			ids = append(ids, sa.id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (m *Manager) terminateLocked(id uint64) {
	msg := vici.NewMessage()
	_ = msg.Set("ike-id", strconv.FormatUint(id, 10))
	// A proper DELETE tells the client at once; force would leave it waiting for DPD.
	_ = msg.Set("timeout", "-1")
	if _, err := m.d.call("terminate", msg); err != nil {
		logger.Debug("ipsec: terminate", id, err)
	}
}

func (m *Manager) listSAsLocked() ([]ikeSA, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var sas []ikeSA
	for msg, err := range m.d.sess.CallStreaming(ctx, "list-sas", "list-sa", vici.NewMessage()) {
		if err != nil {
			return nil, err
		}
		sas = append(sas, parseIKESAs(msg, true)...)
	}
	return sas, nil
}

// parseIKESAs reads list-sas replies and events, which share one layout.
func parseIKESAs(msg *vici.Message, establishedOnly bool) []ikeSA {
	var out []ikeSA
	for _, conn := range msg.Keys() {
		ike, ok := msg.Get(conn).(*vici.Message)
		if !ok || (establishedOnly && str(ike, "state") != "ESTABLISHED") {
			continue
		}
		sa := ikeSA{
			conn:        conn,
			id:          num[uint64](ike, "uniqueid"),
			established: num[int64](ike, "established"),
			user:        str(ike, "remote-eap-id"),
		}
		sa.children = parseChildren(ike)
		out = append(out, sa)
	}
	return out
}

func parseChildren(ike *vici.Message) []childSA {
	children, ok := ike.Get("child-sas").(*vici.Message)
	if !ok {
		return nil
	}
	var out []childSA
	for _, k := range children.Keys() {
		c, ok := children.Get(k).(*vici.Message)
		if !ok {
			continue
		}
		if old, ok := c.Get("old").(*vici.Message); ok {
			c = old // child-rekey events carry the ending SA under "old"
		}
		out = append(out, childSA{id: num[uint64](c, "uniqueid"), in: num[int64](c, "bytes-in"), out: num[int64](c, "bytes-out")})
	}
	return out
}

func str(m *vici.Message, key string) string {
	s, _ := m.Get(key).(string)
	return s
}

func num[T int64 | uint64](m *vici.Message, key string) T {
	n, _ := strconv.ParseUint(str(m, key), 10, 64)
	return T(n)
}

// CollectTraffic returns each user's bytes since the last call and who is online.
func (m *Manager) CollectTraffic() ([]Traffic, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.d == nil || !m.d.running() {
		return m.acct.drain(nil, nil), nil
	}
	sas, err := m.listSAsLocked()
	if err != nil {
		logger.Debug("ipsec: list sessions:", err)
		return m.acct.drain(nil, nil), nil
	}
	var online []string
	seen := map[string]bool{}
	for _, sa := range sas {
		if _, ok := m.insts[sa.conn]; !ok || sa.user == "" {
			continue
		}
		if !seen[sa.user] {
			seen[sa.user] = true
			online = append(online, sa.user)
		}
	}
	return m.acct.drain(sas, m.insts), online
}

func (m *Manager) stopLocked() {
	if m.d != nil {
		m.d.stop()
		m.d = nil
	}
	for p := range m.routed {
		vpnnat.Default.Down(p)
		delete(m.routed, p)
	}
	m.resetLoaded()
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *Manager) HasRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.d != nil && m.d.running()
}

// Status is whether charon runs and, if not, why it last failed.
func (m *Manager) Status() (running bool, lastLine string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.d == nil {
		return false, m.startErr
	}
	return m.d.running(), m.d.last()
}
