// Package ocserv runs OpenConnect/AnyConnect inbounds: one ocserv process per
// inbound, logins checked against a password file the panel writes, byte
// counts read through occtl, and revoked or over-limit sessions ended with it.
package ocserv

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/vpnnat"
)

const (
	startTimeout = 10 * time.Second
	stopTimeout  = 5 * time.Second
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
	local := filepath.Join(config.GetBinFolderPath(), "ocserv")
	if fi, err := os.Stat(local); err == nil && !fi.IsDir() {
		return local, nil
	}
	for _, p := range []string{"/usr/sbin/ocserv", "/usr/local/sbin/ocserv"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("ocserv"); err == nil {
		return p, nil
	}
	return "", errors.New("ocserv not found; install the ocserv package")
}

// runDir must not be bin/ocserv: that name is the shipped binary.
func runDir() string {
	return filepath.Join(config.GetBinFolderPath(), "ocserv.d")
}

type managed struct {
	inst     Instance
	structFP string
	paths    paths
	cmd      *exec.Cmd
	done     chan struct{}
	run      occtl
	tracker  *tracker
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

func (m *managed) pump(r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		m.setLast(sc.Text())
		logger.Debugf("ocserv %d | %s", m.inst.Id, sc.Text())
	}
}

// Manager owns the running ocserv processes, keyed by inbound id.
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
		if cur.running() && cur.structFP == fp {
			cur.inst.Tag = inst.Tag
			return m.applyUsersLocked(cur, inst.Users)
		}
		m.stopLocked(inst.Id)
	}
	cur, err := start(inst)
	if err != nil {
		m.startErrs[inst.Id] = err.Error()
		return err
	}
	delete(m.startErrs, inst.Id)
	cur.structFP = fp
	cur.inst.Users = maps.Clone(inst.Users)
	m.procs[inst.Id] = cur
	logger.Infof("ocserv: started inbound %d on port %d", inst.Id, inst.Port)
	return nil
}

func writeFile(path, body string, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func start(inst Instance) (*managed, error) {
	bin, err := BinaryPath()
	if err != nil {
		return nil, err
	}
	if _, err := occtlPath(); err != nil {
		return nil, err
	}
	sockDir, err := socketDir()
	if err != nil {
		return nil, err
	}
	p := pathsFor(inst.Id, runDir(), sockDir)
	if err := os.MkdirAll(p.ended, 0o700); err != nil {
		return nil, err
	}
	passwd, err := passwdFile(inst.Users)
	if err != nil {
		return nil, err
	}
	usr, grp := dropTo()
	files := []struct {
		path, body string
		mode       os.FileMode
	}{
		{p.cert, inst.PKI.ServerCert + inst.PKI.CA, 0o600},
		{p.key, inst.PKI.ServerKey, 0o600},
		{p.passwd, passwd, 0o600},
		{p.discon, renderDisconnectScript(p), 0o700},
		{p.conf, renderConfig(inst, p, usr, grp), 0o600},
	}
	for _, f := range files {
		if err := writeFile(f.path, f.body, f.mode); err != nil {
			return nil, err
		}
	}
	_ = os.Remove(p.occtl)
	if err := vpnnat.Default.Up(inst.Pool); err != nil {
		logger.Warning("ocserv: routing for inbound", inst.Id, "not set up:", err)
	}
	cmd := exec.CommandContext(context.Background(), bin, "-f", "-c", p.conf)
	// An os.Pipe, not StdoutPipe: Wait would close the latter and could drop
	// the lines that say why ocserv exited.
	out, w, err := os.Pipe()
	if err != nil {
		vpnnat.Default.Down(inst.Pool)
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = w, w
	cur := &managed{inst: inst, paths: p, cmd: cmd, done: make(chan struct{}), run: runOcctl(p.occtl), tracker: newTracker(p.ended)}
	err = cmd.Start()
	_ = w.Close()
	if err != nil {
		_ = out.Close()
		vpnnat.Default.Down(inst.Pool)
		return nil, err
	}
	go func() {
		cur.pump(out)
		_ = out.Close()
	}()
	go func() {
		_ = cmd.Wait()
		close(cur.done)
	}()
	if err := cur.waitReady(); err != nil {
		_ = cmd.Process.Kill()
		<-cur.done
		vpnnat.Default.Down(inst.Pool)
		return nil, fmt.Errorf("ocserv inbound %d: %w (%s)", inst.Id, err, cur.last())
	}
	return cur, nil
}

// waitReady waits for the control socket, which opens once the server is up.
func (m *managed) waitReady() error {
	deadline := time.Now().Add(startTimeout)
	for {
		if _, err := os.Stat(m.paths.occtl); err == nil {
			return nil
		}
		select {
		case <-m.done:
			return errors.New("ocserv exited before it was ready")
		default:
		}
		if time.Now().After(deadline) {
			return errors.New("ocserv did not become ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// applyUsersLocked swaps the password file when an account changed and ends
// the sessions of removed or re-passworded accounts and the newest ones of an
// account over its connection limit.
func (m *Manager) applyUsersLocked(cur *managed, users map[string]User) error {
	// A copy, so a caller reusing its map cannot make an account change look like none.
	prev := cur.inst.Users
	cur.inst.Users = maps.Clone(users)
	if !sameLogins(prev, users) {
		passwd, err := passwdFile(users)
		if err != nil {
			return err
		}
		if err := writeFile(cur.paths.passwd, passwd, 0o600); err != nil {
			return err
		}
	}
	sessions, err := liveSessions(cur.run)
	if err != nil {
		return nil
	}
	perUser := map[string][]session{}
	for _, s := range sessions {
		u, ok := users[s.user]
		if !ok || (prev[s.user].Password != "" && prev[s.user].Password != u.Password) {
			cur.disconnect(s)
			continue
		}
		perUser[s.user] = append(perUser[s.user], s)
	}
	for user, live := range perUser {
		limit := users[user].MaxConns
		if limit <= 0 || len(live) <= limit {
			continue
		}
		slices.SortFunc(live, func(a, b session) int { return int(a.connectedAt - b.connectedAt) })
		for _, s := range live[limit:] {
			cur.disconnect(s)
		}
	}
	return nil
}

func sameLogins(a, b map[string]User) bool {
	if len(a) != len(b) {
		return false
	}
	for name, u := range a {
		if o, ok := b[name]; !ok || o.Password != u.Password {
			return false
		}
	}
	return true
}

func (m *managed) disconnect(s session) {
	if _, err := m.run("disconnect", "id", s.id); err != nil {
		logger.Debug("ocserv: end session", s.id, err)
	}
}

// KickUser ends the live sessions of one account on every inbound.
func (m *Manager) KickUser(user string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	kicked := 0
	for _, cur := range m.procs {
		sessions, err := liveSessions(cur.run)
		if err != nil {
			continue
		}
		for _, s := range sessions {
			if s.user == user {
				cur.disconnect(s)
				kicked++
			}
		}
	}
	return kicked
}

func (m *Manager) stopLocked(id int) {
	delete(m.startErrs, id)
	cur, ok := m.procs[id]
	if !ok {
		return
	}
	delete(m.procs, id)
	if cur.running() {
		_ = cur.cmd.Process.Signal(os.Interrupt)
		select {
		case <-cur.done:
		case <-time.After(stopTimeout):
			_ = cur.cmd.Process.Kill()
			<-cur.done
		}
	}
	vpnnat.Default.Down(cur.inst.Pool)
	for _, f := range []string{cur.paths.passwd, cur.paths.key, cur.paths.cert} {
		_ = os.Remove(f)
	}
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(id)
}

// Reconcile stops unwanted inbounds and (re)starts the rest, recovering crashes.
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

// CollectTraffic returns each user's bytes since the last call and who is online.
func (m *Manager) CollectTraffic() ([]Traffic, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var traffic []Traffic
	seen := map[string]bool{}
	var online []string
	for _, cur := range m.procs {
		sessions, err := liveSessions(cur.run)
		if err != nil {
			sessions = nil
		}
		for user, d := range cur.tracker.collect(sessions) {
			traffic = append(traffic, Traffic{Tag: cur.inst.Tag, Email: user, Up: d[0], Down: d[1]})
		}
		for _, s := range sessions {
			if !seen[s.user] {
				seen[s.user] = true
				online = append(online, s.user)
			}
		}
	}
	slices.Sort(online)
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
	if !cur.running() {
		return false, strings.TrimSpace(cur.last())
	}
	return true, ""
}
