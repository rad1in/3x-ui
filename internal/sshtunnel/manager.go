// Package sshtunnel runs SSH tunnel inbounds: an in-process SSH server per
// inbound whose accounts are the panel's clients. It serves port forwarding
// only (no shell), and meters, limits and kicks accounts itself.
package sshtunnel

import (
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
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

// Manager owns the running servers, keyed by inbound id.
type Manager struct {
	mu    sync.Mutex
	procs map[int]*managed
	// startErrs keeps why an inbound is not running; it is in no process table.
	startErrs map[int]string
}

var (
	managerOnce sync.Once
	manager     *Manager
)

func GetManager() *Manager {
	managerOnce.Do(func() { manager = &Manager{procs: map[int]*managed{}, startErrs: map[int]string{}} })
	return manager
}

// Ensure starts the inbound, restarts it on a structural change, and otherwise
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
			cur.srv.setUsers(inst.Users)
			return nil
		}
		m.stopLocked(inst.Id)
	}
	srv, err := startServer(inst)
	if err != nil {
		m.startErrs[inst.Id] = err.Error()
		return err
	}
	delete(m.startErrs, inst.Id)
	m.procs[inst.Id] = &managed{srv: srv, inst: inst, structFP: fp}
	logStart(inst)
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
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(id)
}

// Reconcile stops unwanted inbounds and (re)starts the rest, recovering a
// listener that died.
func (m *Manager) Reconcile(desired []Instance) {
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
	for _, inst := range desired {
		if err := m.ensureLocked(inst); err != nil {
			logger.Warning("ssh: reconcile inbound", inst.Id, err)
		}
	}
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.procs {
		m.stopLocked(id)
	}
}

// KickUser ends the live connections of one account on every inbound.
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

// Status is whether the inbound's server runs and, if not, why.
func (m *Manager) Status(id int) (running bool, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.procs[id]
	if !ok {
		return false, m.startErrs[id]
	}
	if !cur.srv.isRunning() {
		return false, "the SSH listener stopped"
	}
	return true, ""
}
