// Package openvpn runs OpenVPN inbounds: one openvpn process per inbound, with
// every login decided and every byte metered by the panel over its management
// socket, so accounts, quotas and kicks apply without a restart.
package openvpn

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/vpnnat"
)

const (
	mgmtDialTimeout = 10 * time.Second
	stopTimeout     = 5 * time.Second
)

// Traffic is one user's bytes on one inbound since the last collection.
type Traffic struct {
	Tag   string
	Email string
	Up    int64
	Down  int64
}

// BinaryPath prefers a binary shipped in the panel's bin folder over the system one.
func BinaryPath() (string, error) {
	local := filepath.Join(config.GetBinFolderPath(), "openvpn")
	if fi, err := os.Stat(local); err == nil && !fi.IsDir() {
		return local, nil
	}
	for _, p := range []string{"/usr/sbin/openvpn", "/usr/local/sbin/openvpn"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return exec.LookPath("openvpn")
}

// runDir must not be bin/openvpn: that name is the shipped binary.
func runDir() string {
	return filepath.Join(config.GetBinFolderPath(), "openvpn.d")
}

type managed struct {
	inst     Instance
	structFP string
	usersFP  string
	cmd      *exec.Cmd
	done     chan struct{}
	sess     *session
	conn     net.Conn
	lastLine atomic.Pointer[string]
}

func (m *managed) setLast(line string) { m.lastLine.Store(&line) }

func (m *managed) last() string {
	if p := m.lastLine.Load(); p != nil {
		return *p
	}
	return ""
}

func (m *managed) running() bool {
	select {
	case <-m.done:
		return false
	default:
		return true
	}
}

// Manager owns the running openvpn processes, keyed by inbound id.
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
	managerOnce.Do(func() { manager = &Manager{procs: map[int]*managed{}} })
	return manager
}

// KickUser ends the live connections of one account on every running inbound.
func (m *Manager) KickUser(user string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	kicked := 0
	for _, p := range m.procs {
		if p.running() && p.sess != nil {
			kicked += p.sess.KickUser(user)
		}
	}
	return kicked
}

// Ensure starts the inbound, restarts it on a structural change, and otherwise
// applies a user change in place.
func (m *Manager) Ensure(inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureLocked(inst)
}

func (m *Manager) ensureLocked(inst Instance) error {
	structFP, usersFP := inst.structuralFingerprint(), usersFingerprint(inst.Users)
	if cur, ok := m.procs[inst.Id]; ok {
		if cur.running() && cur.structFP == structFP {
			cur.inst.Tag = inst.Tag
			if cur.usersFP != usersFP {
				cur.sess.SetUsers(inst.Users)
				cur.usersFP = usersFP
				cur.inst.Users = inst.Users
			}
			return nil
		}
		m.stopLocked(inst.Id)
	}
	cur, err := start(inst)
	if err != nil {
		if m.startErrs == nil {
			m.startErrs = map[int]string{}
		}
		m.startErrs[inst.Id] = err.Error()
		return err
	}
	delete(m.startErrs, inst.Id)
	cur.structFP, cur.usersFP = structFP, usersFP
	m.procs[inst.Id] = cur
	logger.Infof("openvpn: started inbound %d on %s/%d", inst.Id, inst.Proto, inst.Port)
	return nil
}

func start(inst Instance) (*managed, error) {
	bin, err := BinaryPath()
	if err != nil {
		return nil, fmt.Errorf("openvpn binary not found: %w", err)
	}
	if !inst.PKI.Complete() || !inst.Network.IsValid() || !inst.Network.Addr().Is4() {
		return nil, errors.New("openvpn: inbound has no certificates or client network")
	}
	dir := runDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	sock := filepath.Join(dir, fmt.Sprintf("mgmt-%d.sock", inst.Id))
	_ = os.Remove(sock)
	cfgPath := filepath.Join(dir, fmt.Sprintf("server-%d.conf", inst.Id))
	if err := os.WriteFile(cfgPath, []byte(renderServerConfig(inst, sock)), 0o600); err != nil {
		return nil, err
	}
	if err := vpnnat.Default.Up(inst.Network); err != nil {
		logger.Warning("openvpn: routing for inbound", inst.Id, "not set up:", err)
	}
	cmd := exec.CommandContext(context.Background(), bin, "--config", cfgPath)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	cur := &managed{inst: inst, cmd: cmd, done: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		vpnnat.Default.Down(inst.Network)
		return nil, err
	}
	go cur.pump(out)
	go func() {
		err := cmd.Wait()
		if err != nil {
			cur.setLast(err.Error())
		}
		close(cur.done)
	}()
	nc, err := dialMgmt(sock, cur.done)
	if err != nil {
		_ = cmd.Process.Kill()
		<-cur.done
		vpnnat.Default.Down(inst.Network)
		return nil, fmt.Errorf("openvpn inbound %d: %w (%s)", inst.Id, err, cur.last())
	}
	cur.conn = nc
	cur.sess = newSession(nc)
	cur.sess.SetUsers(inst.Users)
	go cur.sess.serve()
	return cur, nil
}

func (m *managed) pump(r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		m.setLast(sc.Text())
		logger.Debugf("openvpn %d | %s", m.inst.Id, sc.Text())
	}
}

func dialMgmt(sock string, done <-chan struct{}) (net.Conn, error) {
	deadline := time.Now().Add(mgmtDialTimeout)
	for {
		nc, err := net.Dial("unix", sock)
		if err == nil {
			return nc, nil
		}
		select {
		case <-done:
			return nil, errors.New("openvpn exited before its management socket opened")
		default:
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (m *Manager) stopLocked(id int) {
	delete(m.startErrs, id)
	cur, ok := m.procs[id]
	if !ok {
		return
	}
	delete(m.procs, id)
	if cur.conn != nil {
		_ = cur.conn.Close()
	}
	if cur.running() {
		_ = cur.cmd.Process.Signal(os.Interrupt)
		select {
		case <-cur.done:
		case <-time.After(stopTimeout):
			_ = cur.cmd.Process.Kill()
			<-cur.done
		}
	}
	vpnnat.Default.Down(cur.inst.Network)
	_ = os.Remove(filepath.Join(runDir(), fmt.Sprintf("server-%d.conf", id)))
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(id)
}

// Reconcile stops unwanted inbounds and (re)starts the rest, recovering crashes.
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
			logger.Warning("openvpn: reconcile inbound", inst.Id, err)
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

// CollectTraffic returns each user's bytes since the last call and who is online.
func (m *Manager) CollectTraffic() ([]Traffic, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var traffic []Traffic
	var online []string
	for _, cur := range m.procs {
		deltas, on := cur.sess.Collect()
		for email, c := range deltas {
			traffic = append(traffic, Traffic{Tag: cur.inst.Tag, Email: email, Up: c.Up, Down: c.Down})
		}
		online = append(online, on...)
	}
	return traffic, online
}

func (m *Manager) HasRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cur := range m.procs {
		if cur.running() {
			return true
		}
	}
	return false
}

// Status is the last openvpn output line for an inbound, for the panel to show.
func (m *Manager) Status(id int) (running bool, lastLine string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.procs[id]
	if !ok {
		return false, m.startErrs[id]
	}
	return cur.running(), cur.last()
}
