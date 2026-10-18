// Package l2tp runs the L2TP/IPsec inbound: xl2tpd hands each connection to
// pppd, which checks the panel's logins in /etc/ppp/chap-secrets; the IPsec
// layer is a pre-shared-key connection on the shared charon (internal/ipsec).
package l2tp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/vpnnat"
)

const stopTimeout = 5 * time.Second

// Traffic is one user's bytes on the inbound since the last collection.
type Traffic struct {
	Tag   string
	Email string
	Up    int64
	Down  int64
}

func BinaryPath() (string, error) {
	local := filepath.Join(config.GetBinFolderPath(), "xl2tpd")
	for _, p := range []string{local, "/usr/sbin/xl2tpd", "/usr/local/sbin/xl2tpd"} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	if p, err := exec.LookPath("xl2tpd"); err == nil {
		return p, nil
	}
	return "", errors.New("xl2tpd not found; install the xl2tpd and ppp packages")
}

func runDir() string {
	return filepath.Join(config.GetBinFolderPath(), "l2tp.d")
}

func pppSupport() error {
	if _, err := os.Stat("/dev/ppp"); err != nil {
		return errors.New("this kernel has no PPP support (/dev/ppp is missing), which L2TP needs")
	}
	return nil
}

// Manager owns xl2tpd and the panel's chap-secrets block. UDP 1701 is
// host-wide, so it serves one inbound: the lowest id it is given.
type Manager struct {
	mu       sync.Mutex
	want     map[int]Instance
	cur      *Instance
	passwds  map[string]string
	cmd      *exec.Cmd
	done     chan struct{}
	structFP string
	tracker  *tracker
	startErr string
	lastLine atomic.Pointer[string]
}

var (
	managerOnce sync.Once
	manager     *Manager
)

func GetManager() *Manager {
	managerOnce.Do(func() { manager = &Manager{want: map[int]Instance{}} })
	return manager
}

func (m *Manager) Ensure(inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.want[inst.Id] = inst
	return m.applyLocked()
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.want, id)
	if err := m.applyLocked(); err != nil {
		logger.Warning("l2tp: remove inbound", id, err)
	}
}

func (m *Manager) Reconcile(desired []Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.want = map[int]Instance{}
	for _, inst := range desired {
		m.want[inst.Id] = inst
	}
	return m.applyLocked()
}

func (m *Manager) running() bool {
	if m.done == nil {
		return false
	}
	select {
	case <-m.done:
		return false
	default:
		return true
	}
}

func (m *Manager) applyLocked() error {
	var inst *Instance
	for id := range m.want {
		if inst == nil || id < inst.Id {
			v := m.want[id]
			inst = &v
		}
	}
	if inst == nil {
		m.stopLocked()
		return nil
	}
	if m.cur != nil && m.cur.Id != inst.Id {
		m.stopLocked()
	}

	users := make(map[string]string, len(inst.Users))
	for email, u := range inst.Users {
		users[email] = u.Password
	}
	if err := writeChapSecrets(chapSecretsPath, users); err != nil {
		return fmt.Errorf("l2tp: write %s: %w", chapSecretsPath, err)
	}
	if err := ipsec.GetManager().EnsureL2TP(ipsec.L2TPConn{Id: inst.Id, PSK: inst.PSK}); err != nil {
		logger.Warning("l2tp: ipsec side:", err)
	}
	if m.cur == nil || m.cur.Pool != inst.Pool {
		if m.cur != nil {
			vpnnat.Default.Down(m.cur.Pool)
		}
		if err := vpnnat.Default.Up(inst.Pool); err != nil {
			logger.Warning("l2tp: routing for", inst.Pool, "not set up:", err)
		}
	}
	prevPasswds := m.passwds
	m.cur, m.passwds = inst, users

	if fp := inst.structuralFingerprint(); !m.running() || fp != m.structFP {
		m.stopProcessLocked()
		if err := m.startLocked(*inst); err != nil {
			m.startErr = err.Error()
			return err
		}
		m.startErr = ""
		m.structFP = fp
	}
	m.kickLocked(*inst, prevPasswds)
	return nil
}

func (m *Manager) startLocked(inst Instance) error {
	if err := pppSupport(); err != nil {
		return err
	}
	bin, err := BinaryPath()
	if err != nil {
		return err
	}
	dir := runDir()
	for _, sub := range []string{"", "sessions", "ended"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return err
		}
	}
	files := map[string]string{
		"xl2tpd.conf":    renderXL2TPD(inst, dir),
		"options.xl2tpd": renderPPPOptions(inst, dir),
		"ip-up":          renderIPUp(dir),
		"ip-down":        renderIPDown(dir),
	}
	for name, body := range files {
		mode := os.FileMode(0o600)
		if strings.HasPrefix(name, "ip-") {
			mode = 0o700
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), mode); err != nil {
			return err
		}
	}
	cmd := exec.CommandContext(context.Background(), bin, "-D",
		"-c", filepath.Join(dir, "xl2tpd.conf"),
		"-p", filepath.Join(dir, "xl2tpd.pid"),
		"-C", filepath.Join(dir, "l2tp-control"))
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go m.pump(out)
	go func() {
		if err := cmd.Wait(); err != nil {
			m.setLast(err.Error())
		}
		close(done)
	}()
	m.cmd, m.done = cmd, done
	if m.tracker == nil {
		m.tracker = newTracker(dir, sysfsStats)
	}
	logger.Infof("l2tp: xl2tpd started for inbound %d", inst.Id)
	return nil
}

func (m *Manager) setLast(line string) { m.lastLine.Store(&line) }

func (m *Manager) pump(r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		m.setLast(sc.Text())
		logger.Debugf("xl2tpd | %s", sc.Text())
	}
}

// isPPPD guards the kill: a pid in a stale session file may have been reused.
func isPPPD(pid int) bool {
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	return err == nil && strings.TrimSpace(string(comm)) == "pppd"
}

// kickLocked ends the links of users who lost access or changed password, and
// the newest links of users over their connection limit.
func (m *Manager) kickLocked(inst Instance, prevPasswds map[string]string) {
	perUser := map[string][]session{}
	for _, s := range liveSessions(runDir()) {
		u, ok := inst.Users[s.user]
		changed := prevPasswds != nil && prevPasswds[s.user] != "" && prevPasswds[s.user] != u.Password
		if !ok || changed {
			m.kill(s)
			continue
		}
		perUser[s.user] = append(perUser[s.user], s)
	}
	for user, links := range perUser {
		limit := inst.Users[user].MaxConns
		if limit <= 0 || len(links) <= limit {
			continue
		}
		for _, s := range links[limit:] {
			m.kill(s)
		}
	}
}

// KickUser ends every live link of one user and returns how many; the login
// stays valid, so the client may dial again at once.
func (m *Manager) KickUser(user string) int {
	kicked := 0
	for _, s := range liveSessions(runDir()) {
		if s.user == user && isPPPD(s.pid) {
			m.kill(s)
			kicked++
		}
	}
	return kicked
}

func (m *Manager) kill(s session) {
	if !isPPPD(s.pid) {
		return
	}
	if err := syscall.Kill(s.pid, syscall.SIGTERM); err != nil {
		logger.Debug("l2tp: end link", s.ifname, err)
	}
}

func (m *Manager) stopProcessLocked() {
	if m.cmd == nil {
		return
	}
	if m.running() {
		_ = m.cmd.Process.Signal(os.Interrupt)
		select {
		case <-m.done:
		case <-time.After(stopTimeout):
			_ = m.cmd.Process.Kill()
			<-m.done
		}
	}
	m.cmd, m.done = nil, nil
}

func (m *Manager) stopLocked() {
	m.stopProcessLocked()
	if m.cur != nil {
		ipsec.GetManager().RemoveL2TP(m.cur.Id)
		vpnnat.Default.Down(m.cur.Pool)
		if err := writeChapSecrets(chapSecretsPath, nil); err != nil {
			logger.Warning("l2tp: clear", chapSecretsPath, err)
		}
	}
	m.cur, m.passwds, m.structFP = nil, nil, ""
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.want = map[int]Instance{}
	m.stopLocked()
}

// CollectTraffic returns each user's bytes since the last call and who is online.
func (m *Manager) CollectTraffic() ([]Traffic, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur == nil || m.tracker == nil {
		return nil, nil
	}
	deltas, sessions := m.tracker.collect()
	traffic := make([]Traffic, 0, len(deltas))
	for user, d := range deltas {
		traffic = append(traffic, Traffic{Tag: m.cur.Tag, Email: user, Up: d[0], Down: d[1]})
	}
	seen := map[string]bool{}
	var online []string
	for _, s := range sessions {
		if !seen[s.user] {
			seen[s.user] = true
			online = append(online, s.user)
		}
	}
	sort.Strings(online)
	return traffic, online
}

// Status is whether xl2tpd runs and, if not, why it last failed.
func (m *Manager) Status() (running bool, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startErr != "" {
		return false, m.startErr
	}
	if why := ipsec.GetManager().L2TPUnavailable(); why != "" {
		return m.running(), why
	}
	if p := m.lastLine.Load(); p != nil && !m.running() {
		return false, *p
	}
	return m.running(), ""
}
