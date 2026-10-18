// Package sstp runs SSTP inbounds (the VPN Windows sets up from its own
// settings): a TLS listener per inbound speaks SSTP and hands the PPP frames
// it carries to one pppd per client, which checks the logins the panel keeps in
// pppd's chap-secrets. The panel sees every frame, so it also meters and kicks.
package sstp

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/pppsecrets"
	"github.com/mhsanaei/3x-ui/v3/internal/vpnnat"
)

// Traffic is one user's bytes on one inbound since the last collection.
type Traffic struct {
	Tag   string
	Email string
	Up    int64
	Down  int64
}

type managed struct {
	srv      *server
	inst     Instance
	structFP string
}

// Manager owns the running listeners, keyed by inbound id.
type Manager struct {
	mu    sync.Mutex
	procs map[int]*managed
	// startErrs keeps why an inbound is not running; it is in no process table.
	startErrs map[int]string

	spawn       spawner
	preflight   func() error
	secretsPath string
}

var (
	managerOnce sync.Once
	manager     *Manager
)

func GetManager() *Manager {
	managerOnce.Do(func() { manager = newManager(spawnPppd, hostCanRunPPP, pppsecrets.Path) })
	return manager
}

func newManager(spawn spawner, preflight func() error, secretsPath string) *Manager {
	return &Manager{
		procs: map[int]*managed{}, startErrs: map[int]string{},
		spawn: spawn, preflight: preflight, secretsPath: secretsPath,
	}
}

// Ensure starts the inbound, rebuilds it on a structural change, and otherwise
// applies an account change in place.
func (m *Manager) Ensure(inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureLocked(inst)
}

func (m *Manager) ensureLocked(inst Instance) error {
	fp := inst.structuralFingerprint()
	if cur, ok := m.procs[inst.Id]; ok {
		if cur.srv.isRunning() && cur.structFP == fp {
			cur.inst.Tag = inst.Tag
			cur.inst.Users = inst.Users
			if err := m.writeSecretsLocked(); err != nil {
				return err
			}
			cur.srv.setUsers(inst.Users)
			return nil
		}
		m.stopLocked(inst.Id)
	}
	if err := m.preflight(); err != nil {
		m.startErrs[inst.Id] = err.Error()
		return err
	}
	srv, err := startServer(inst, m.spawn)
	if err != nil {
		m.startErrs[inst.Id] = err.Error()
		return err
	}
	delete(m.startErrs, inst.Id)
	m.procs[inst.Id] = &managed{srv: srv, inst: inst, structFP: fp}
	if err := m.writeSecretsLocked(); err != nil {
		m.stopLocked(inst.Id)
		m.startErrs[inst.Id] = err.Error()
		return err
	}
	if err := vpnnat.Default.Up(inst.Pool); err != nil {
		logger.Warning("sstp: routing for", inst.Pool, "not set up:", err)
	}
	logger.Infof("sstp: started inbound %d on %s:%d", inst.Id, inst.Listen, inst.Port)
	return nil
}

// writeSecretsLocked publishes every running inbound's logins to pppd, each
// valid only for that inbound's own pppd.
func (m *Manager) writeSecretsLocked() error {
	var entries []pppsecrets.Entry
	for _, cur := range m.procs {
		for email, u := range cur.inst.Users {
			entries = append(entries, pppsecrets.Entry{User: email, Server: cur.inst.pppName(), Secret: u.Password})
		}
	}
	if err := pppsecrets.Write(m.secretsPath, "sstp", entries); err != nil {
		return fmt.Errorf("sstp: write %s: %w", m.secretsPath, err)
	}
	return nil
}

func (m *Manager) stopLocked(id int) {
	delete(m.startErrs, id)
	cur, ok := m.procs[id]
	if !ok {
		return
	}
	delete(m.procs, id)
	cur.srv.stop()
	vpnnat.Default.Down(cur.inst.Pool)
	if err := m.writeSecretsLocked(); err != nil {
		logger.Warning("sstp:", err)
	}
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(id)
}

// Reconcile stops unwanted inbounds and (re)starts the rest, recovering a
// listener that died.
func (m *Manager) Reconcile(desired []Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[int]bool{}
	for _, inst := range desired {
		want[inst.Id] = true
	}
	for id := range m.procs {
		if !want[id] {
			m.stopLocked(id)
		}
	}
	var errs []error
	for _, inst := range desired {
		if err := m.ensureLocked(inst); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.procs {
		m.stopLocked(id)
	}
}

// KickUser ends the live sessions of one account on every inbound.
func (m *Manager) KickUser(user string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	kicked := 0
	for _, cur := range m.procs {
		kicked += cur.srv.kick(user)
	}
	return kicked
}

// CollectTraffic returns each user's bytes since the last call and who is online.
func (m *Manager) CollectTraffic() ([]Traffic, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var traffic []Traffic
	var online []string
	for _, cur := range m.procs {
		deltas, on := cur.srv.collect()
		for email, d := range deltas {
			traffic = append(traffic, Traffic{Tag: cur.inst.Tag, Email: email, Up: d[0], Down: d[1]})
		}
		online = append(online, on...)
	}
	return traffic, online
}

// Status is whether the inbound's listener runs and, if not, why.
func (m *Manager) Status(id int) (running bool, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.procs[id]
	if !ok {
		return false, m.startErrs[id]
	}
	if !cur.srv.isRunning() {
		return false, "the SSTP listener stopped"
	}
	return true, ""
}
